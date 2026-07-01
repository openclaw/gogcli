package googleapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

var (
	errUnexpectedRequestBody = errors.New("unexpected request body")
	errRefreshFailed         = errors.New("refresh failed")
)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type closeTracker struct {
	closed bool
}

func (c *closeTracker) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (c *closeTracker) Close() error {
	c.closed = true
	return nil
}

func newTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{},
	}
}

type refreshableTestTokenSource struct {
	token        string
	refreshes    int
	tokenRequest int
}

func (s *refreshableTestTokenSource) Token() (*oauth2.Token, error) {
	s.tokenRequest++

	return &oauth2.Token{AccessToken: s.token}, nil
}

func (s *refreshableTestTokenSource) ForceRefresh(context.Context) error {
	s.refreshes++
	s.token = "fresh-token"

	return nil
}

func TestNewRetryTransportDefaults(t *testing.T) {
	rt := NewRetryTransport(nil)
	if rt.Base == nil {
		t.Fatalf("expected base transport")
	}

	if rt.MaxRetries429 == 0 || rt.MaxRetries5xx == 0 {
		t.Fatalf("expected defaults to be set")
	}

	if rt.CircuitBreaker == nil {
		t.Fatalf("expected circuit breaker")
	}
}

func TestRetryTransportRoundTripSuccess(t *testing.T) {
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return newTestResponse(http.StatusOK, "ok"), nil
	})

	rt := &RetryTransport{
		Base:          base,
		MaxRetries429: 1,
		MaxRetries5xx: 1,
		BaseDelay:     0,
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryTransportRoundTripRetries429(t *testing.T) {
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return newTestResponse(http.StatusTooManyRequests, "rate"), nil
		}

		return newTestResponse(http.StatusOK, "ok"), nil
	})

	rt := &RetryTransport{
		Base:          base,
		MaxRetries429: 1,
		MaxRetries5xx: 0,
		BaseDelay:     0,
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestRetryTransportRoundTripStopsAfter429Retries(t *testing.T) {
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return newTestResponse(http.StatusTooManyRequests, "rate"), nil
	})

	rt := &RetryTransport{
		Base:          base,
		MaxRetries429: 1,
		MaxRetries5xx: 0,
		BaseDelay:     0,
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}

	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestRetryTransportRoundTripRetries5xx(t *testing.T) {
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return newTestResponse(http.StatusInternalServerError, "err"), nil
		}

		return newTestResponse(http.StatusOK, "ok"), nil
	})

	cb := NewCircuitBreaker()
	rt := &RetryTransport{
		Base:           base,
		MaxRetries429:  0,
		MaxRetries5xx:  1,
		BaseDelay:      0,
		CircuitBreaker: cb,
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}

	if cb.State() != circuitStateClosed {
		t.Fatalf("expected circuit closed, got %s", cb.State())
	}
}

func TestRetryTransportRefreshesAuthOnceForInsufficientScope403(t *testing.T) {
	tokenSource := &refreshableTestTokenSource{token: "stale-token"}
	var gotBodies []string
	calls := 0

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++

		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}

		gotBodies = append(gotBodies, string(body))

		switch calls {
		case 1:
			if got := req.Header.Get("Authorization"); got != "Bearer stale-token" {
				t.Fatalf("first authorization = %q", got)
			}

			return newTestResponse(http.StatusForbidden, `{"error":{"code":403,"message":"Request had insufficient authentication scopes.","status":"PERMISSION_DENIED"}}`), nil
		case 2:
			if got := req.Header.Get("Authorization"); got != "Bearer fresh-token" {
				t.Fatalf("second authorization = %q", got)
			}

			return newTestResponse(http.StatusOK, "ok"), nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return nil, errUnexpectedRequestBody
		}
	})

	rt := &RetryTransport{
		Base: &oauth2.Transport{
			Source: tokenSource,
			Base:   base,
		},
		MaxRetries429: 0,
		MaxRetries5xx: 0,
		BaseDelay:     0,
		RefreshAuth:   tokenSource.ForceRefresh,
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

	if tokenSource.refreshes != 1 {
		t.Fatalf("refreshes = %d, want 1", tokenSource.refreshes)
	}

	if gotBodies[0] != "payload" || gotBodies[1] != "payload" {
		t.Fatalf("unexpected bodies: %#v", gotBodies)
	}
}

func TestRetryTransportPreservesInsufficientScope403WhenAuthRefreshFails(t *testing.T) {
	const body = `{"error":{"errors":[{"reason":"insufficientPermissions"}],"message":"Insufficient Permission","code":403}}`

	refreshes := 0
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++

		return newTestResponse(http.StatusForbidden, body), nil
	})

	rt := &RetryTransport{
		Base: base,
		RefreshAuth: func(context.Context) error {
			refreshes++

			return errRefreshFailed
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}

	gotBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	if string(gotBody) != body {
		t.Fatalf("response body = %q, want %q", gotBody, body)
	}

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	if refreshes != 1 {
		t.Fatalf("refreshes = %d, want 1", refreshes)
	}
}

func TestRetryTransportDoesNotRefreshAuthForOrdinary403(t *testing.T) {
	refreshes := 0
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++

		return newTestResponse(http.StatusForbidden, `{"error":{"code":403,"message":"The caller does not have permission","status":"PERMISSION_DENIED"}}`), nil
	})

	rt := &RetryTransport{
		Base: base,
		RefreshAuth: func(context.Context) error {
			refreshes++

			return nil
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	if refreshes != 0 {
		t.Fatalf("refreshes = %d, want 0", refreshes)
	}
}

func TestRetryTransportDoesNotRefreshAuthForNonReplayableInsufficientScope403(t *testing.T) {
	refreshes := 0
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++

		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}

		if string(body) != "payload" {
			t.Fatalf("body = %q, want payload", body)
		}

		return newTestResponse(http.StatusForbidden, `{"error":{"message":"Request had insufficient authentication scopes."}}`), nil
	})

	rt := &RetryTransport{
		Base: base,
		RefreshAuth: func(context.Context) error {
			refreshes++

			return nil
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("payload")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.ContentLength = 0

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	if refreshes != 0 {
		t.Fatalf("refreshes = %d, want 0", refreshes)
	}
}

func TestResponseIndicatesInsufficientScopesRecognizesGoogleVariants(t *testing.T) {
	tests := []string{
		`{"error":{"errors":[{"reason":"insufficientPermissions"}],"code":403}}`,
		`{"error":{"message":"Insufficient Permission","code":403}}`,
		`{"error":{"message":"Insufficient Permissions","code":403}}`,
	}

	for _, body := range tests {
		resp := newTestResponse(http.StatusForbidden, body)

		insufficient, err := responseIndicatesInsufficientScopes(resp)
		if err != nil {
			t.Fatalf("detect insufficient scopes for %q: %v", body, err)
		}

		if !insufficient {
			t.Fatalf("Google scope variant not recognized: %s", body)
		}

		preserved, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read preserved response: %v", err)
		}
		_ = resp.Body.Close()

		if string(preserved) != body {
			t.Fatalf("response body = %q, want %q", preserved, body)
		}
	}
}

func TestResponseIndicatesInsufficientScopesPreservesOversizedUnknownLengthBody(t *testing.T) {
	want := strings.Repeat("x", int(maxAuthRetryResponseBodyBytes)+128)
	resp := newTestResponse(http.StatusForbidden, want)
	resp.ContentLength = -1

	insufficient, err := responseIndicatesInsufficientScopes(resp)
	if err != nil {
		t.Fatalf("detect insufficient scopes: %v", err)
	}

	if insufficient {
		t.Fatal("oversized response should not trigger an auth retry")
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read preserved response: %v", err)
	}
	_ = resp.Body.Close()

	if string(got) != want {
		t.Fatalf("response body was not preserved: got %d bytes, want %d", len(got), len(want))
	}
}

func TestRetryTransportCircuitBreakerOpen(t *testing.T) {
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return newTestResponse(http.StatusOK, "ok"), nil
	})

	cb := NewCircuitBreaker()
	cb.open = true
	cb.lastFailure = time.Now()

	rt := &RetryTransport{
		Base:           base,
		CircuitBreaker: cb,
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
		t.Fatalf("expected circuit breaker error")
	}

	if calls != 0 {
		t.Fatalf("expected 0 calls, got %d", calls)
	}
}

func TestRetryTransportCalculateBackoffRetryAfter(t *testing.T) {
	rt := &RetryTransport{BaseDelay: time.Second}
	resp := &http.Response{Header: http.Header{"Retry-After": []string{"5"}}}

	if got := rt.calculateBackoff(0, resp); got != 5*time.Second {
		t.Fatalf("expected 5s, got %v", got)
	}

	resp = &http.Response{Header: http.Header{"Retry-After": []string{"-1"}}}
	if got := rt.calculateBackoff(0, resp); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}

	date := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
	resp = &http.Response{Header: http.Header{"Retry-After": []string{date}}}

	if got := rt.calculateBackoff(0, resp); got <= 0 {
		t.Fatalf("expected positive delay, got %v", got)
	}
}

func TestRetryTransportCalculateBackoffDefault(t *testing.T) {
	rt := &RetryTransport{BaseDelay: time.Nanosecond}
	resp := &http.Response{Header: http.Header{}}

	if got := rt.calculateBackoff(0, resp); got != time.Nanosecond {
		t.Fatalf("expected base delay, got %v", got)
	}
}

func TestRetryTransportSleepInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rt := &RetryTransport{}
	if err := rt.sleep(ctx, time.Second); err == nil {
		t.Fatalf("expected sleep error")
	}
}

func TestEnsureReplayableBodyMore(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("hello")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.ContentLength = int64(len("hello"))

	replayable, err := ensureReplayableBody(req)
	if err != nil {
		t.Fatalf("ensureReplayableBody: %v", err)
	}

	if !replayable {
		t.Fatalf("expected replayable body")
	}

	if req.GetBody == nil {
		t.Fatalf("expected GetBody")
	}

	body1, readErr := io.ReadAll(req.Body)
	if readErr != nil {
		t.Fatalf("read body: %v", readErr)
	}

	rc, err := req.GetBody()
	if err != nil {
		t.Fatalf("get body: %v", err)
	}

	body2, readErr := io.ReadAll(rc)
	if readErr != nil {
		t.Fatalf("read body copy: %v", readErr)
	}
	_ = rc.Close()

	if string(body1) != "hello" || string(body2) != "hello" {
		t.Fatalf("unexpected body: %q %q", body1, body2)
	}
}

func TestEnsureReplayableBodySkipsLargeKnownLength(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("hello")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.ContentLength = maxBufferedReplayBodyBytes + 1

	replayable, err := ensureReplayableBody(req)
	if err != nil {
		t.Fatalf("ensureReplayableBody: %v", err)
	}

	if replayable {
		t.Fatalf("expected non-replayable body")
	}

	if req.GetBody != nil {
		t.Fatalf("expected GetBody to remain nil")
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if string(body) != "hello" {
		t.Fatalf("body was consumed: %q", body)
	}
}

func TestEnsureReplayableBodySkipsUnknownLength(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("hello")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.ContentLength = 0

	replayable, err := ensureReplayableBody(req)
	if err != nil {
		t.Fatalf("ensureReplayableBody: %v", err)
	}

	if replayable {
		t.Fatalf("expected non-replayable body")
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if string(body) != "hello" {
		t.Fatalf("body was consumed: %q", body)
	}
}

func TestDrainAndClose(t *testing.T) {
	rc := &closeTracker{}
	drainAndClose(rc)

	if !rc.closed {
		t.Fatalf("expected close")
	}
}

func TestRetryTransportRoundTripResetsBody(t *testing.T) {
	var gotBodies []string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}

		gotBodies = append(gotBodies, string(body))

		return newTestResponse(http.StatusTooManyRequests, "rate"), nil
	})

	rt := &RetryTransport{
		Base:          base,
		MaxRetries429: 1,
		MaxRetries5xx: 0,
		BaseDelay:     0,
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

	if len(gotBodies) != 2 {
		t.Fatalf("expected 2 bodies, got %d", len(gotBodies))
	}

	if gotBodies[0] != "payload" || gotBodies[1] != "payload" {
		t.Fatalf("unexpected bodies: %#v", gotBodies)
	}
}

func TestRetryTransportRoundTripDoesNotRetryLargeNonReplayableBody(t *testing.T) {
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++

		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}

		if string(body) != "payload" {
			t.Errorf("unexpected body: %q", body)
			return nil, errUnexpectedRequestBody
		}

		return newTestResponse(http.StatusTooManyRequests, "rate"), nil
	})

	rt := &RetryTransport{
		Base:          base,
		MaxRetries429: 1,
		MaxRetries5xx: 0,
		BaseDelay:     0,
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", io.NopCloser(strings.NewReader("payload")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.ContentLength = maxBufferedReplayBodyBytes + 1

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}

	if calls != 1 {
		t.Fatalf("expected one call, got %d", calls)
	}
}

func TestRetryTransportRoundTripError(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errBoom
	})

	rt := &RetryTransport{Base: base}

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
}
