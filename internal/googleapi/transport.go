package googleapi

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
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
	maxAuthRetryResponseBodyBytes = int64(1 << 20)
)

var errRequestBodyTooLarge = errors.New("request body too large to buffer for retry")

// RetryTransport wraps an http.RoundTripper with retry logic for
// rate limits (429) and server errors (5xx).
type RetryTransport struct {
	Base           http.RoundTripper
	MaxRetries429  int
	MaxRetries5xx  int
	BaseDelay      time.Duration
	CircuitBreaker *CircuitBreaker
	RefreshAuth    func(context.Context) error
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
		return nil, &CircuitBreakerError{}
	}

	replayable, err := ensureReplayableBody(req)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	retries429 := 0
	retries5xx := 0
	retriedAuth := false

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
			return nil, fmt.Errorf("round trip: %w", err)
		}

		// Success
		if resp.StatusCode < 400 {
			if t.CircuitBreaker != nil {
				t.CircuitBreaker.RecordSuccess()
			}

			return resp, nil
		}

		// Rate limit (429)
		if resp.StatusCode == http.StatusTooManyRequests {
			if retries429 >= t.MaxRetries429 || !replayable {
				return resp, nil // Return the 429 response after max retries
			}

			delay := t.calculateBackoff(retries429, resp)
			slog.Debug("rate limited, retrying", //nolint:gosec // logged values are internal retry metadata
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

			if retries5xx >= t.MaxRetries5xx || !replayable {
				return resp, nil
			}

			slog.Debug("server error, retrying", //nolint:gosec // logged values are internal retry metadata
				"status", resp.StatusCode,
				"attempt", retries5xx+1)

			drainAndClose(resp.Body)

			if err := t.sleep(req.Context(), ServerErrorRetryDelay); err != nil {
				return nil, err
			}

			retries5xx++

			continue
		}

		if resp.StatusCode == http.StatusForbidden && t.RefreshAuth != nil && !retriedAuth && replayable {
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

func (t *RetryTransport) calculateBackoff(attempt int, resp *http.Response) time.Duration {
	// Check Retry-After header
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			if seconds < 0 {
				return 0
			}

			return time.Duration(seconds) * time.Second
		}

		if t, err := http.ParseTime(retryAfter); err == nil {
			d := time.Until(t)
			if d < 0 {
				return 0
			}

			return d
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

	bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, maxBufferedReplayBodyBytes+1))
	if err != nil {
		return false, fmt.Errorf("read request body: %w", err)
	}

	if int64(len(bodyBytes)) > maxBufferedReplayBodyBytes {
		return false, fmt.Errorf("%w: %d bytes exceeds %d bytes", errRequestBodyTooLarge, len(bodyBytes), maxBufferedReplayBodyBytes)
	}
	_ = req.Body.Close()

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

func responseIndicatesInsufficientScopes(resp *http.Response) (bool, error) {
	if resp == nil || resp.Body == nil {
		return false, nil
	}

	if resp.ContentLength > maxAuthRetryResponseBodyBytes {
		return false, nil
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxAuthRetryResponseBodyBytes+1))
	if err != nil {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{Reader: io.MultiReader(bytes.NewReader(bodyBytes), resp.Body), Closer: resp.Body}

		return false, fmt.Errorf("read auth failure response: %w", err)
	}

	if int64(len(bodyBytes)) > maxAuthRetryResponseBodyBytes {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{Reader: io.MultiReader(bytes.NewReader(bodyBytes), resp.Body), Closer: resp.Body}

		return false, nil
	}

	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	text := strings.ToLower(string(bodyBytes))
	normalized := strings.NewReplacer("_", " ", "-", " ").Replace(text)

	return strings.Contains(normalized, "insufficient authentication scopes") ||
		strings.Contains(normalized, "insufficient scopes") ||
		strings.Contains(normalized, "insufficientpermissions") ||
		strings.Contains(normalized, "insufficient permission") ||
		strings.Contains(normalized, "access token scope insufficient"), nil
}
