package googleapi

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxBufferedReplayBodyBytes    = int64(16 << 20)
	maxInspectedResponseBodyBytes = int64(1 << 20)
	// maxRetryAfter caps Retry-After waits. Google may send delta-seconds
	// or an HTTP-date far in the future; CLI contexts often have no deadline.
	maxRetryAfter = 60 * time.Second
)

var errRequestBodyTooLarge = errors.New("request body too large to buffer for retry")

type retriesDisabledContextKey struct{}

// WithoutRetries marks requests made with ctx as single-attempt operations.
// The configured base transport still handles authentication.
func WithoutRetries(ctx context.Context) context.Context {
	return context.WithValue(ctx, retriesDisabledContextKey{}, true)
}

func retriesDisabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}

	disabled, _ := ctx.Value(retriesDisabledContextKey{}).(bool)

	return disabled
}

// RetryTransport wraps an http.RoundTripper with retry logic for
// rate limits (429, or 403 with a rate-limit reason) and server errors (5xx).
type RetryTransport struct {
	Base           http.RoundTripper
	MaxRetries429  int
	MaxRetries5xx  int
	BaseDelay      time.Duration
	CircuitBreaker *CircuitBreaker
	RefreshAuth    func(context.Context) error
	// Reauth is called when the OAuth refresh token is expired or revoked
	// (invalid_grant). It should launch a browser-based re-authorization
	// flow, persist the new refresh token, and return nil on success.
	// If Reauth is nil, invalid_grant errors are surfaced without retry.
	Reauth func(context.Context) error
}

// NewRetryTransport creates a RetryTransport with sensible defaults.
func NewRetryTransport(base http.RoundTripper) *RetryTransport {
	if base == nil {
		base = http.DefaultTransport
	}

	return &RetryTransport{
		Base:           base,
		MaxRetries429:  MaxRateLimitRetries,
		MaxRetries5xx:  Max5xxRetries,
		BaseDelay:      RateLimitBaseDelay,
		CircuitBreaker: NewCircuitBreaker(),
	}
}

// RoundTrip implements http.RoundTripper with retry logic.
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.CircuitBreaker != nil && t.CircuitBreaker.IsOpen() {
		if req.Body != nil {
			_ = req.Body.Close()
		}

		return nil, &CircuitBreakerError{}
	}
	retryDisabled := retriesDisabled(req.Context())

	replayable, err := ensureReplayableBody(req)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	retries429 := 0
	retries5xx := 0
	retriedAuth := false
	retriedReauth := false

	for {
		// Reset body for retry
		if req.GetBody != nil {
			if req.Body != nil {
				_ = req.Body.Close()
			}

			if body, getErr := req.GetBody(); getErr != nil {
				return nil, fmt.Errorf("reset request body: %w", getErr)
			} else {
				req.Body = body
			}
		}

		resp, err = t.Base.RoundTrip(req)
		if err != nil {
			// Detect invalid_grant from the OAuth2 transport layer.
			// This happens when the stored refresh token is expired or
			// revoked. If a Reauth function is available and we haven't
			// already retried, attempt auto-reauth and retry the request.
			if isInvalidGrantError(err) {
				if t.Reauth != nil && !retriedReauth && !retryDisabled && replayable {
					slog.Debug("refresh token expired or revoked, attempting auto-reauth")

					if reauthErr := t.Reauth(req.Context()); reauthErr != nil {
						slog.Debug("auto-reauth failed", "err", reauthErr)

						return nil, fmt.Errorf("refresh token expired or revoked: %w; re-authentication failed: %w; run 'gog auth add' to re-authorize manually", err, reauthErr)
					}

					retriedReauth = true

					continue
				}

				// Reauth is not available (suppressed by --no-input, not
				// stored OAuth, or already retried). Surface a clear,
				// actionable error instead of the raw OAuth2 message.
				return nil, fmt.Errorf("refresh token expired or revoked: %w; run 'gog auth add' to re-authorize", err)
			}

			return nil, fmt.Errorf("round trip: %w", err)
		}

		// Success
		if resp.StatusCode < 400 {
			if t.CircuitBreaker != nil {
				t.CircuitBreaker.RecordSuccess()
			}

			return resp, nil
		}

		// Rate limit (429, or 403 with a rate-limit reason)
		if resp.StatusCode == http.StatusTooManyRequests || responseIndicatesRateLimit(resp) {
			if retryDisabled || retries429 >= t.MaxRetries429 || !replayable {
				return resp, nil // Return the rate-limit response after max retries
			}

			delay := t.calculateBackoff(retries429, resp)
			slog.Debug("rate limited, retrying",
				"status", resp.StatusCode,
				"delay", delay,
				"attempt", retries429+1,
				"max_retries", t.MaxRetries429)

			drainAndClose(resp.Body)

			if err := t.sleep(req.Context(), delay); err != nil {
				return nil, err
			}

			retries429++

			continue
		}

		// Server error (5xx)
		if resp.StatusCode >= 500 {
			if t.CircuitBreaker != nil {
				t.CircuitBreaker.RecordFailure()
			}

			if retryDisabled || retries5xx >= t.MaxRetries5xx || !replayable {
				return resp, nil
			}

			slog.Debug("server error, retrying",
				"status", resp.StatusCode,
				"attempt", retries5xx+1)

			drainAndClose(resp.Body)

			if err := t.sleep(req.Context(), ServerErrorRetryDelay); err != nil {
				return nil, err
			}

			retries5xx++

			continue
		}

		if resp.StatusCode == http.StatusForbidden && t.RefreshAuth != nil && !retriedAuth && replayable && !retryDisabled {
			insufficientScopes, detectErr := responseIndicatesInsufficientScopes(resp)
			if detectErr != nil {
				slog.Debug("could not inspect auth failure response for retry", "err", detectErr)
				return resp, nil
			}

			if insufficientScopes {
				slog.Debug("insufficient scopes response, refreshing auth token and retrying")

				if err := t.RefreshAuth(req.Context()); err != nil {
					slog.Debug("could not refresh auth after insufficient scopes response", "err", err)

					return resp, nil
				}

				drainAndClose(resp.Body)

				retriedAuth = true

				continue
			}
		}

		// Other errors (4xx except 429): don't retry
		return resp, nil
	}
}

func capRetryAfter(wait time.Duration) time.Duration {
	if wait > maxRetryAfter {
		return maxRetryAfter
	}

	return wait
}

func (t *RetryTransport) calculateBackoff(attempt int, resp *http.Response) time.Duration {
	// Check Retry-After header
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil || errors.Is(err, strconv.ErrRange) {
			if seconds < 0 {
				return 0
			}

			// Bound seconds before converting to a duration to avoid overflow.
			if seconds >= int64(maxRetryAfter/time.Second) {
				return maxRetryAfter
			}

			return time.Duration(seconds) * time.Second
		}

		if t, err := http.ParseTime(retryAfter); err == nil {
			d := time.Until(t)
			if d < 0 {
				return 0
			}

			return capRetryAfter(d)
		}
	}

	// Exponential backoff with jitter: 1s, 2s, 4s...
	if t.BaseDelay <= 0 {
		return 0
	}

	var baseDelay time.Duration

	if bd := t.BaseDelay * time.Duration(1<<attempt); bd <= 0 {
		return 0
	} else {
		baseDelay = bd
	}

	jitterRange := baseDelay / 2
	if jitterRange <= 0 {
		return baseDelay
	}

	jitter, err := randomDurationBelow(jitterRange)
	if err != nil {
		return baseDelay
	}

	return baseDelay + jitter
}

func randomDurationBelow(upperBound time.Duration) (time.Duration, error) {
	if upperBound <= 0 {
		return 0, nil
	}

	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(upperBound)))
	if err != nil {
		return 0, fmt.Errorf("generate retry jitter: %w", err)
	}

	return time.Duration(n.Int64()), nil
}

func (t *RetryTransport) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)

	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("sleep interrupted: %w", ctx.Err())
	}
}

func ensureReplayableBody(req *http.Request) (bool, error) {
	if req == nil || req.Body == nil || req.GetBody != nil {
		return true, nil
	}

	if req.ContentLength <= 0 || req.ContentLength > maxBufferedReplayBodyBytes {
		return false, nil
	}

	// Once buffering starts, this transport owns closing the original body,
	// including when reading or enforcing the replay limit fails.
	defer req.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, maxBufferedReplayBodyBytes+1))
	if err != nil {
		return false, fmt.Errorf("read request body: %w", err)
	}

	if int64(len(bodyBytes)) > maxBufferedReplayBodyBytes {
		return false, fmt.Errorf("%w: %d bytes exceeds %d bytes", errRequestBodyTooLarge, len(bodyBytes), maxBufferedReplayBodyBytes)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	return true, nil
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 1<<20))
	_ = body.Close()
}

// peekResponseBody reads a small response body and puts an identical body
// back on resp, so the caller still receives the original response. It
// returns ok=false for bodies too large to inspect.
func peekResponseBody(resp *http.Response) (body []byte, ok bool, err error) {
	if resp == nil || resp.Body == nil {
		return nil, false, nil
	}

	if resp.ContentLength > maxInspectedResponseBodyBytes {
		return nil, false, nil
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxInspectedResponseBodyBytes+1))
	if err != nil {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{Reader: io.MultiReader(bytes.NewReader(bodyBytes), resp.Body), Closer: resp.Body}

		return nil, false, fmt.Errorf("read error response: %w", err)
	}

	if int64(len(bodyBytes)) > maxInspectedResponseBodyBytes {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{Reader: io.MultiReader(bytes.NewReader(bodyBytes), resp.Body), Closer: resp.Body}

		return nil, false, nil
	}

	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	return bodyBytes, true, nil
}

// responseIndicatesRateLimit reports whether resp is a 403 whose Google error
// reason is a rate limit. Google documents these as retryable with
// exponential backoff, like 429.
func responseIndicatesRateLimit(resp *http.Response) bool {
	if resp.StatusCode != http.StatusForbidden {
		return false
	}

	bodyBytes, ok, err := peekResponseBody(resp)
	if err != nil {
		slog.Debug("could not inspect forbidden response for rate limit", "err", err)
		return false
	}

	if !ok {
		return false
	}

	var payload struct {
		Error struct {
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	if json.Unmarshal(bodyBytes, &payload) != nil {
		return false
	}

	for _, item := range payload.Error.Errors {
		if IsRateLimitReason(item.Reason) {
			return true
		}
	}

	return false
}

func responseIndicatesInsufficientScopes(resp *http.Response) (bool, error) {
	bodyBytes, ok, err := peekResponseBody(resp)
	if err != nil || !ok {
		return false, err
	}

	text := strings.ToLower(string(bodyBytes))
	normalized := strings.NewReplacer("_", " ", "-", " ").Replace(text)

	return strings.Contains(normalized, "insufficient authentication scopes") ||
		strings.Contains(normalized, "insufficient scopes") ||
		strings.Contains(normalized, "insufficientpermissions") ||
		strings.Contains(normalized, "insufficient permission") ||
		strings.Contains(normalized, "access token scope insufficient"), nil
}
