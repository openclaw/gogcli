package googleapi

import (
	"errors"
	"fmt"
	"strings"
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

type InsufficientScopeError struct {
	Service            string
	Email              string
	RequiredScopes     []string
	GrantedScopes      []string
	ReauthorizeCommand string
}

func (e *InsufficientScopeError) Error() string {
	message := fmt.Sprintf(
		"OAuth grant for %s is missing required %s scope: %s",
		e.Email,
		e.Service,
		strings.Join(e.RequiredScopes, ", "),
	)
	if e.ReauthorizeCommand != "" {
		message += "; re-authenticate with: " + e.ReauthorizeCommand
	}

	return message
}

// HTTPStatusError preserves status metadata from Google APIs implemented with
// direct HTTP clients while retaining the service-specific error text.
type HTTPStatusError struct {
	Code   int
	Status string
	Err    error
}

func (e *HTTPStatusError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}

	return fmt.Sprintf("HTTP %d", e.Code)
}

func (e *HTTPStatusError) Unwrap() error {
	return e.Err
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

// IsInsufficientScopeError checks if the stored OAuth grant lacks a required scope.
func IsInsufficientScopeError(err error) bool {
	var e *InsufficientScopeError
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
