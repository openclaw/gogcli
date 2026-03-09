package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/99designs/keyring"
	ggoogleapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/config"
	gogapi "github.com/steipete/gogcli/internal/googleapi"
)

func TestStableExitCode_PreservesExistingExitError(t *testing.T) {
	in := &ExitError{Code: 3, Err: errors.New("no results")}
	out := stableExitCode(in)
	if !errors.Is(out, in) {
		t.Fatalf("expected same error instance")
	}
	if got := ExitCode(out); got != 3 {
		t.Fatalf("expected exit code 3, got %d", got)
	}
}

func TestStableExitCode_AuthRequired(t *testing.T) {
	in := &gogapi.AuthRequiredError{Service: "gmail", Email: "a@b.com", Cause: keyring.ErrKeyNotFound}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeAuthRequired {
		t.Fatalf("expected exit code %d, got %d", exitCodeAuthRequired, got)
	}
}

func TestStableExitCode_CredentialsMissing(t *testing.T) {
	in := &config.CredentialsMissingError{Path: "/tmp/credentials.json", Cause: errors.New("missing")}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeConfig {
		t.Fatalf("expected exit code %d, got %d", exitCodeConfig, got)
	}
}

func TestStableExitCode_GoogleAPINotFound(t *testing.T) {
	in := &ggoogleapi.Error{Code: 404, Message: "not found"}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeNotFound {
		t.Fatalf("expected exit code %d, got %d", exitCodeNotFound, got)
	}
}

func TestStableExitCode_GoogleAPIRateLimited(t *testing.T) {
	in := &ggoogleapi.Error{Code: 429, Message: "too many requests"}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeRateLimited {
		t.Fatalf("expected exit code %d, got %d", exitCodeRateLimited, got)
	}
}

func TestStableExitCode_GoogleAPIQuotaExceeded(t *testing.T) {
	in := &ggoogleapi.Error{
		Code:    403,
		Message: "quota exceeded",
		Errors:  []ggoogleapi.ErrorItem{{Reason: "quotaExceeded"}},
	}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeRateLimited {
		t.Fatalf("expected exit code %d, got %d", exitCodeRateLimited, got)
	}
}

func TestStableExitCode_GoogleAPIRetryable(t *testing.T) {
	in := &ggoogleapi.Error{Code: 503, Message: "backend error"}
	out := stableExitCode(in)
	if got := ExitCode(out); got != exitCodeRetryable {
		t.Fatalf("expected exit code %d, got %d", exitCodeRetryable, got)
	}
}

func TestStableExitCode_Cancelled(t *testing.T) {
	out := stableExitCode(context.Canceled)
	if got := ExitCode(out); got != exitCodeCancelled {
		t.Fatalf("expected exit code %d, got %d", exitCodeCancelled, got)
	}
}

func TestStableExitCode_DeadlineExceeded(t *testing.T) {
	out := stableExitCode(context.DeadlineExceeded)
	if got := ExitCode(out); got != exitCodeRetryable {
		t.Fatalf("expected exit code %d, got %d", exitCodeRetryable, got)
	}
}

func TestStableExitCode_GenericErrorUnchanged(t *testing.T) {
	in := errors.New("boom")
	out := stableExitCode(in)
	if !errors.Is(out, in) {
		t.Fatalf("expected stableExitCode to return original error for generic errors")
	}
	if got := ExitCode(out); got != 1 {
		t.Fatalf("expected exit code 1, got %d", got)
	}
}
