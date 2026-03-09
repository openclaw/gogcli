package cmd

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	gapi "google.golang.org/api/googleapi"
)

const (
	maxRetries = 5
	baseDelay  = 1 * time.Second
	maxDelay   = 30 * time.Second
)

// retryOnQuota retries fn on 429 (rate limit) and 500/503 (transient server) errors
// with exponential backoff + jitter.
func retryOnQuota(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		// Check if retryable
		if !isRetryableError(err) {
			return err
		}

		// Don't retry if we've exhausted attempts
		if attempt == maxRetries {
			return fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
		}

		// Exponential backoff with jitter
		delay := baseDelay * time.Duration(1<<uint(attempt))
		if delay > maxDelay {
			delay = maxDelay
		}
		// Add jitter: 50-100% of delay (crypto/rand for linter compliance)
		var randBuf [8]byte
		_, _ = rand.Read(randBuf[:])
		halfDelay := int64(delay / 2)
		var jitter time.Duration
		if halfDelay > 0 {
			jitter = time.Duration(binary.LittleEndian.Uint64(randBuf[:]) % uint64(halfDelay)) //nolint:gosec // jitter value is bounded
		}
		delay = delay/2 + jitter

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

// isRetryableError returns true for transient Google API errors (429, 500, 502, 503)
// that are safe to retry with exponential backoff.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *gapi.Error
	if ok := errors.As(err, &apiErr); ok {
		switch apiErr.Code {
		case 429: // rate limit
			return true
		case 500, 502, 503: // transient server errors
			return true
		}
	}
	// Also check for string match as fallback (some errors don't use googleapi.Error)
	errStr := err.Error()
	return strings.Contains(errStr, "rateLimitExceeded") || strings.Contains(errStr, "429")
}
