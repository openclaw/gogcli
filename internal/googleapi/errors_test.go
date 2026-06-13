package googleapi

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var errBase = errors.New("base")

func TestErrors_IsHelpers(t *testing.T) {
	if !IsAuthRequiredError(&AuthRequiredError{Service: "gmail", Email: "a@b.com", Cause: errBase}) {
		t.Fatalf("expected IsAuthRequiredError")
	}

	if !IsInsufficientScopeError(&InsufficientScopeError{Service: "gmail", Email: "a@b.com"}) {
		t.Fatalf("expected IsInsufficientScopeError")
	}

	if !IsRateLimitError(&RateLimitError{RetryAfter: time.Second, Retries: 2}) {
		t.Fatalf("expected IsRateLimitError")
	}

	if !IsCircuitBreakerError(&CircuitBreakerError{}) {
		t.Fatalf("expected IsCircuitBreakerError")
	}

	if !IsQuotaExceededError(&QuotaExceededError{Resource: "gmail"}) {
		t.Fatalf("expected IsQuotaExceededError")
	}

	if !IsNotFoundError(&NotFoundError{Resource: "msg", ID: "id"}) {
		t.Fatalf("expected IsNotFoundError")
	}

	if !IsPermissionDeniedError(&PermissionDeniedError{Resource: "file", Action: "read"}) {
		t.Fatalf("expected IsPermissionDeniedError")
	}
}

func TestErrors_Messages(t *testing.T) {
	authErr := &AuthRequiredError{Service: "gmail", Email: "a@b.com", Cause: errBase}
	if got := authErr.Error(); got != "auth required for gmail a@b.com" {
		t.Fatalf("unexpected: %q", got)
	}

	if !errors.Is(authErr, errBase) {
		t.Fatalf("expected unwrap to match base")
	}

	scopeErr := &InsufficientScopeError{
		Service:            "gmail",
		Email:              "a@b.com",
		RequiredScopes:     []string{"https://mail.google.com/"},
		ReauthorizeCommand: "gog auth add a@b.com --services gmail --extra-scopes https://mail.google.com/ --force-consent",
	}
	if got := scopeErr.Error(); !strings.Contains(got, scopeErr.ReauthorizeCommand) {
		t.Fatalf("unexpected: %q", got)
	}

	if got := (&RateLimitError{RetryAfter: 2 * time.Second, Retries: 3}).Error(); !strings.Contains(got, "retry after 2s") {
		t.Fatalf("unexpected: %q", got)
	}

	if got := (&RateLimitError{Retries: 1}).Error(); !strings.Contains(got, "after 1 retries") {
		t.Fatalf("unexpected: %q", got)
	}

	if got := (&NotFoundError{Resource: "file"}).Error(); got != "file not found" {
		t.Fatalf("unexpected: %q", got)
	}

	if got := (&NotFoundError{Resource: "file", ID: "id1"}).Error(); got != "file not found: id1" {
		t.Fatalf("unexpected: %q", got)
	}

	if got := (&PermissionDeniedError{Resource: "file"}).Error(); got != "permission denied for file" {
		t.Fatalf("unexpected: %q", got)
	}

	if got := (&PermissionDeniedError{Resource: "file", Action: "delete"}).Error(); got != "permission denied: cannot delete file" {
		t.Fatalf("unexpected: %q", got)
	}

	if got := (&CircuitBreakerError{}).Error(); got == "" {
		t.Fatalf("expected circuit breaker message")
	}

	if got := (&QuotaExceededError{Resource: "drive"}).Error(); got != "API quota exceeded for drive" {
		t.Fatalf("unexpected: %q", got)
	}

	if got := (&QuotaExceededError{}).Error(); got != "API quota exceeded" {
		t.Fatalf("unexpected: %q", got)
	}
}
