package cmd

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/99designs/keyring"
	ggoogleapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/config"
	gogapi "github.com/steipete/gogcli/internal/googleapi"
)

const (
	// Exit code 0 is success.
	// Exit code 1 is generic failure.
	// Exit code 2 is usage/parse error (see usage.go).
	// Exit code 3 is empty results (see paging.go).

	exitCodeAuthRequired     = 4
	exitCodeNotFound         = 5
	exitCodePermissionDenied = 6
	exitCodeRateLimited      = 7
	exitCodeRetryable        = 8
	exitCodeConfig           = 10

	// 130 is the conventional "interrupted" exit code (SIGINT / Ctrl-C).
	exitCodeCancelled = 130
)

// stableExitCode wraps common/expected failure modes in ExitError so callers can
// branch on exit status without needing to parse human-oriented stderr.
func stableExitCode(err error) error {
	if err == nil {
		return nil
	}

	var ee *ExitError
	if errors.As(err, &ee) {
		return err
	}

	if errors.Is(err, context.Canceled) {
		return &ExitError{Code: exitCodeCancelled, Err: err}
	}

	var authErr *gogapi.AuthRequiredError
	if errors.As(err, &authErr) {
		return &ExitError{Code: exitCodeAuthRequired, Err: err}
	}

	var credErr *config.CredentialsMissingError
	if errors.As(err, &credErr) {
		return &ExitError{Code: exitCodeConfig, Err: err}
	}

	if errors.Is(err, keyring.ErrKeyNotFound) {
		return &ExitError{Code: exitCodeAuthRequired, Err: err}
	}

	var gerr *ggoogleapi.Error
	if errors.As(err, &gerr) {
		if code := googleAPIExitCode(gerr); code != 1 {
			return &ExitError{Code: code, Err: err}
		}
	}

	var cbErr *gogapi.CircuitBreakerError
	if errors.As(err, &cbErr) {
		return &ExitError{Code: exitCodeRetryable, Err: err}
	}

	var ne net.Error
	if errors.As(err, &ne) {
		if ne.Timeout() {
			return &ExitError{Code: exitCodeRetryable, Err: err}
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return &ExitError{Code: exitCodeRetryable, Err: err}
	}

	return err
}

func googleAPIExitCode(err *ggoogleapi.Error) int {
	if err == nil {
		return 1
	}

	// google.golang.org/api/googleapi.Error includes Code and a list of structured
	// "reason" values; we map the common ones to stable exit codes.
	reason := ""
	if len(err.Errors) > 0 {
		reason = strings.TrimSpace(strings.ToLower(err.Errors[0].Reason))
	}

	switch err.Code {
	case 401:
		return exitCodeAuthRequired
	case 403:
		if isQuotaOrRateLimitReason(reason) {
			return exitCodeRateLimited
		}
		return exitCodePermissionDenied
	case 404:
		return exitCodeNotFound
	case 429:
		return exitCodeRateLimited
	default:
		if err.Code >= 500 {
			return exitCodeRetryable
		}
	}

	return 1
}

func isQuotaOrRateLimitReason(reason string) bool {
	switch strings.TrimSpace(strings.ToLower(reason)) {
	case "ratelimitexceeded",
		"userratelimitexceeded",
		"quotaexceeded",
		"dailylimitexceeded",
		"resourceexhausted":
		return true
	default:
		return false
	}
}
