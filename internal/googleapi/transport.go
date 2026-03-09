package googleapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// RetryTransport wraps an http.RoundTripper with retry logic for
// rate limits (429) and server errors (5xx).
type RetryTransport struct {
	Base           http.RoundTripper
	MaxRetries429  int
	MaxRetries5xx  int
	BaseDelay      time.Duration
	CircuitBreaker *CircuitBreaker
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

	if err := ensureReplayableBody(req); err != nil {
		return nil, err
	}

	var resp *http.Response
	var err error
	retries429 := 0
	retries5xx := 0

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
			if retries429 >= t.MaxRetries429 {
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

			if retries5xx >= t.MaxRetries5xx {
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
	jitter := time.Duration(rand.Int64N(int64(jitterRange))) //nolint:gosec // non-crypto jitter

	return baseDelay + jitter
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

// bytesReader is a simple bytes.Reader replacement to avoid import
type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n

	return n, nil
}

func ensureReplayableBody(req *http.Request) error {
	if req == nil || req.Body == nil || req.GetBody != nil {
		return nil
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	_ = req.Body.Close()

	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(newBytesReader(bodyBytes)), nil
	}
	req.Body = io.NopCloser(newBytesReader(bodyBytes))

	return nil
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 1<<20))
	_ = body.Close()
}
