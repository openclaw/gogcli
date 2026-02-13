package googleapi

import (
	"errors"
	"fmt"
	"time"
)

type AuthRequiredError struct {
	Service string
	Email   string
	Client  string
	Cause   error
}

func (e *AuthRequiredError) Error() string {
	if e.Client != "" {
		return fmt.Sprintf("auth required for %s %s (client %s)", e.Service, e.Email, e.Client)
	}

	return fmt.Sprintf("auth required for %s %s", e.Service, e.Email)
}

func (e *AuthRequiredError) Unwrap() error {
	return e.Cause
}

// RateLimitError indicates rate limit was exceeded
type RateLimitError struct {
	RetryAfter time.Duration
	Retries    int
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limit exceeded, retry after %s (attempted %d retries)", e.RetryAfter, e.Retries)
	}

	return fmt.Sprintf("rate limit exceeded after %d retries", e.Retries)
}

// CircuitBreakerError indicates the circuit breaker is open
type CircuitBreakerError struct{}

func (e *CircuitBreakerError) Error() string {
	return "circuit breaker is open, too many recent failures - try again later"
}

// QuotaExceededError indicates API quota was exceeded
type QuotaExceededError struct {
	Resource string
}

func (e *QuotaExceededError) Error() string {
	if e.Resource != "" {
		return fmt.Sprintf("API quota exceeded for %s", e.Resource)
	}

	return "API quota exceeded"
}

// NotFoundError indicates the requested resource was not found
type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("%s not found: %s", e.Resource, e.ID)
	}

	return fmt.Sprintf("%s not found", e.Resource)
}

// PermissionDeniedError indicates insufficient permissions
type PermissionDeniedError struct {
	Resource string
	Action   string
}

func (e *PermissionDeniedError) Error() string {
	if e.Action != "" {
		return fmt.Sprintf("permission denied: cannot %s %s", e.Action, e.Resource)
	}

	return fmt.Sprintf("permission denied for %s", e.Resource)
}

// IsAuthRequiredError checks if the error is an auth required error
func IsAuthRequiredError(err error) bool {
	var e *AuthRequiredError
	return errors.As(err, &e)
}

// IsRateLimitError checks if the error is a rate limit error
func IsRateLimitError(err error) bool {
	var e *RateLimitError
	return errors.As(err, &e)
}

// IsCircuitBreakerError checks if the error is a circuit breaker error
func IsCircuitBreakerError(err error) bool {
	var e *CircuitBreakerError
	return errors.As(err, &e)
}

// IsQuotaExceededError checks if the error is a quota exceeded error
func IsQuotaExceededError(err error) bool {
	var e *QuotaExceededError
	return errors.As(err, &e)
}

// IsNotFoundError checks if the error is a not found error
func IsNotFoundError(err error) bool {
	var e *NotFoundError
	return errors.As(err, &e)
}

// IsPermissionDeniedError checks if the error is a permission denied error
func IsPermissionDeniedError(err error) bool {
	var e *PermissionDeniedError
	return errors.As(err, &e)
}

// InvalidGrantError indicates the OAuth refresh token has been expired or
// revoked. The user must re-authorize to obtain a new refresh token.
type InvalidGrantError struct {
	Cause error
}

func (e *InvalidGrantError) Error() string {
	return fmt.Sprintf("oauth2 refresh token expired or revoked (re-authorize with: gog auth add --force-consent): %v", e.Cause)
}

func (e *InvalidGrantError) Unwrap() error {
	return e.Cause
}

// IsInvalidGrantError checks if the error is an invalid grant error
func IsInvalidGrantError(err error) bool {
	var e *InvalidGrantError
	return errors.As(err, &e)
}
