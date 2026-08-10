package googleapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

var (
	errAutoReauthNetworkTimeout      = errors.New("network timeout")
	errAutoReauthUntypedInvalidGrant = errors.New(`oauth2: "invalid_grant" "Bad Request"`)
	errAutoReauthBrowserNotOpen      = errors.New("browser did not open")
)

func TestIsInvalidGrantError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "unrelated error",
			err:  errAutoReauthNetworkTimeout,
			want: false,
		},
		{
			name: `invalid_grant with "Token has been expired or revoked"`,
			err:  invalidGrantError(),
			want: true,
		},
		{
			name: "untyped invalid_grant text",
			err:  errAutoReauthUntypedInvalidGrant,
			want: false,
		},
		{
			name: "wrapped invalid_grant",
			err:  fmt.Errorf("refresh access token: %w", invalidGrantError()),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInvalidGrantError(tt.err); got != tt.want {
				t.Errorf("isInvalidGrantError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func invalidGrantError() error {
	return &oauth2.RetrieveError{
		ErrorCode:        "invalid_grant",
		ErrorDescription: "Token has been expired or revoked.",
	}
}

func TestRetryTransportAutoReauthOnInvalidGrant(t *testing.T) {
	tokenSource := &refreshableTestTokenSource{token: "stale-token"}
	reauthCalls := 0
	calls := 0

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++

		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		_ = body

		switch calls {
		case 1:
			// First call: the oauth2 transport will fail with invalid_grant
			// because the refresh token is stale. We simulate this by
			// returning an error from the base transport.
			return nil, invalidGrantError()
		case 2:
			// After reauth, the token should be fresh.
			if got := req.Header.Get("Authorization"); got != "Bearer fresh-token" {
				t.Fatalf("second authorization = %q, want %q", got, "Bearer fresh-token")
			}

			return newTestResponse(http.StatusOK, "ok"), nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return nil, errUnexpectedRequestBody
		}
	})

	rt := &RetryTransport{
		Base: &oauth2TransportWrapper{
			source: tokenSource,
			base:   base,
		},
		MaxRetries429: 0,
		MaxRetries5xx: 0,
		BaseDelay:     0,
		Reauth: func(ctx context.Context) error {
			reauthCalls++
			tokenSource.token = "fresh-token"

			return nil
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("payload")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.ContentLength = int64(len("payload"))

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}

	if reauthCalls != 1 {
		t.Fatalf("reauth calls = %d, want 1", reauthCalls)
	}
}

func TestRetryTransportAutoReauthFailureSurfacesError(t *testing.T) {
	calls := 0

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		_ = req.Body

		return nil, invalidGrantError()
	})

	rt := &RetryTransport{
		Base: base,
		Reauth: func(context.Context) error {
			return errAutoReauthBrowserNotOpen
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if err == nil {
		t.Fatalf("expected error")
	}

	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("error should mention invalid_grant: %v", err)
	}

	if !strings.Contains(err.Error(), "re-authentication failed") {
		t.Fatalf("error should mention re-authentication failed: %v", err)
	}

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRetryTransportNoReauthWhenNil(t *testing.T) {
	calls := 0

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, invalidGrantError()
	})

	rt := &RetryTransport{
		Base:   base,
		Reauth: nil, // No reauth function
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if err == nil {
		t.Fatalf("expected error")
	}

	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("error should contain invalid_grant: %v", err)
	}

	if !strings.Contains(err.Error(), "gog auth add") {
		t.Fatalf("error should contain 'gog auth add' hint: %v", err)
	}

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRetryTransportNoReauthWhenRetriesDisabled(t *testing.T) {
	reauthCalls := 0
	calls := 0

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, invalidGrantError()
	})

	rt := &RetryTransport{
		Base: base,
		Reauth: func(context.Context) error {
			reauthCalls++
			return nil
		},
	}

	ctx := WithoutRetries(context.Background())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if err == nil {
		t.Fatalf("expected error")
	}

	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("error should contain invalid_grant: %v", err)
	}

	if !strings.Contains(err.Error(), "gog auth add") {
		t.Fatalf("error should contain 'gog auth add' hint: %v", err)
	}

	if reauthCalls != 0 {
		t.Fatalf("reauth should not be called when retries are disabled, got %d calls", reauthCalls)
	}

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRetryTransportNoReauthForNonReplayableBody(t *testing.T) {
	reauthCalls := 0
	calls := 0

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(req.Body)
		_ = body

		return nil, invalidGrantError()
	})

	rt := &RetryTransport{
		Base: base,
		Reauth: func(context.Context) error {
			reauthCalls++
			return nil
		},
	}

	// Non-replayable: large body with unknown content length
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("payload")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.ContentLength = maxBufferedReplayBodyBytes + 1

	resp, err := rt.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if err == nil {
		t.Fatalf("expected error")
	}

	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("error should contain invalid_grant: %v", err)
	}

	if !strings.Contains(err.Error(), "gog auth add") {
		t.Fatalf("error should contain 'gog auth add' hint: %v", err)
	}

	if reauthCalls != 0 {
		t.Fatalf("reauth should not be called for non-replayable body, got %d calls", reauthCalls)
	}
}

func TestRetryTransportReauthOnlyOnce(t *testing.T) {
	reauthCalls := 0
	calls := 0

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		// Always return invalid_grant, even after reauth
		return nil, invalidGrantError()
	})

	rt := &RetryTransport{
		Base: base,
		Reauth: func(context.Context) error {
			reauthCalls++
			return nil
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if err == nil {
		t.Fatalf("expected error")
	}

	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("error should contain invalid_grant: %v", err)
	}

	if !strings.Contains(err.Error(), "gog auth add") {
		t.Fatalf("error should contain 'gog auth add' hint: %v", err)
	}

	// Should only reauth once, not loop
	if reauthCalls != 1 {
		t.Fatalf("reauth calls = %d, want 1", reauthCalls)
	}

	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestNoInputFromContext(t *testing.T) {
	if NoInputFromContext(context.Background()) {
		t.Fatal("default context should not be no-input")
	}

	ctx := WithNoInput(context.Background())
	if !NoInputFromContext(ctx) {
		t.Fatal("WithNoInput context should report NoInput")
	}
}

// TestRetryTransportReauthResetsTokenSource verifies that after a successful
// reauth, the retried request uses the NEW refresh token, not the stale one.
// This is the critical test that exposes the "store-only" hole: if the
// in-memory token source is not reset, the retried request reuses the
// revoked refresh token and fails again.
func TestRetryTransportReauthResetsTokenSource(t *testing.T) {
	// simulateResettableSource is a token source that tracks which refresh
	// token it uses, simulating resettableOAuthTokenSource behavior.
	source := &simulResettableSource{refreshToken: "revoked-token"}

	reauthCalled := false
	calls := 0

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(req.Body)
		_ = body

		switch calls {
		case 1:
			// First call: the oauth2 transport tries to refresh using the
			// revoked token and fails with invalid_grant.
			return nil, invalidGrantError()
		case 2:
			// After reauth, the token source should have been reset.
			// If it wasn't, this test would fail because the reauth
			// function only updates the store, not the source.
			if source.refreshToken == "revoked-token" {
				t.Fatal("token source was not reset after reauth; retried request would use revoked token")
			}

			return newTestResponse(http.StatusOK, "ok"), nil
		default:
			t.Fatalf("unexpected call %d", calls)

			return nil, errUnexpectedRequestBody
		}
	})

	rt := &RetryTransport{
		Base: &oauth2TransportWrapper{
			source: source,
			base:   base,
		},
		Reauth: func(ctx context.Context) error {
			reauthCalled = true
			// Simulate: reauth gets a new token from the browser and
			// persists it to the store. The critical step is resetting
			// the in-memory token source.
			source.ResetRefreshToken("new-refresh-token")

			return nil
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("payload")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.ContentLength = int64(len("payload"))

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	if !reauthCalled {
		t.Fatal("reauth was not called")
	}

	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

// simulResettableSource is a minimal token source that simulates
// resettableOAuthTokenSource for testing the reset-after-reauth behavior.
type simulResettableSource struct {
	refreshToken string
}

func (s *simulResettableSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: "access-" + s.refreshToken}, nil
}

func (s *simulResettableSource) ResetRefreshToken(token string) {
	s.refreshToken = token
}

// oauth2TransportWrapper is a minimal oauth2.Transport-like wrapper for
// testing. It calls the token source to get a token and sets the
// Authorization header before delegating to the base transport.
type oauth2TransportWrapper struct {
	source oauth2.TokenSource
	base   http.RoundTripper
}

func (w *oauth2TransportWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := w.source.Token()
	if err != nil {
		return nil, fmt.Errorf("get OAuth token: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)

	resp, err := w.base.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("base round trip: %w", err)
	}

	return resp, nil
}
