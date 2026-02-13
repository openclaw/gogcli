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
)

// mockTransport implements http.RoundTripper for testing
type mockTransport struct {
	responses []*http.Response
	errors    []error
	calls     int
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	idx := m.calls
	m.calls++

	if idx < len(m.errors) && m.errors[idx] != nil {
		return nil, m.errors[idx]
	}

	if idx < len(m.responses) {
		return m.responses[idx], nil
	}

	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func TestRetryTransport_Success(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}

	rt := NewRetryTransport(mock)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
	var resp *http.Response

	if r, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else {
		resp = r
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	if mock.calls != 1 {
		t.Errorf("expected 1 call, got %d", mock.calls)
	}
}

func TestRetryTransport_RateLimit_Retry(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: 429, Body: io.NopCloser(strings.NewReader("rate limited"))},
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}

	rt := NewRetryTransport(mock)
	rt.BaseDelay = 10 * time.Millisecond // Speed up test

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
	var resp *http.Response

	if r, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else {
		resp = r
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}

	if mock.calls != 2 {
		t.Errorf("expected 2 calls (1 retry), got %d", mock.calls)
	}
}

func TestRetryTransport_RateLimit_MaxRetries(t *testing.T) {
	// All responses are 429
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: 429, Body: io.NopCloser(strings.NewReader("rate limited"))},
			{StatusCode: 429, Body: io.NopCloser(strings.NewReader("rate limited"))},
			{StatusCode: 429, Body: io.NopCloser(strings.NewReader("rate limited"))},
			{StatusCode: 429, Body: io.NopCloser(strings.NewReader("rate limited"))},
		},
	}

	rt := NewRetryTransport(mock)
	rt.BaseDelay = 1 * time.Millisecond
	rt.MaxRetries429 = 2

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
	var resp *http.Response

	if r, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else {
		resp = r
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != 429 {
		t.Errorf("expected 429 after max retries, got %d", resp.StatusCode)
	}
	// 1 initial + 2 retries = 3 total

	if mock.calls != 3 {
		t.Errorf("expected 3 calls, got %d", mock.calls)
	}
}

func TestRetryTransport_ServerError_Retry(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: 503, Body: io.NopCloser(strings.NewReader("service unavailable"))},
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}

	rt := NewRetryTransport(mock)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
	var resp *http.Response

	if r, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else {
		resp = r
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}

	if mock.calls != 2 {
		t.Errorf("expected 2 calls, got %d", mock.calls)
	}
}

func TestRetryTransport_ClientError_NoRetry(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found"))},
		},
	}

	rt := NewRetryTransport(mock)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
	var resp *http.Response

	if r, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else {
		resp = r
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	if mock.calls != 1 {
		t.Errorf("expected 1 call (no retry for 4xx), got %d", mock.calls)
	}
}

func TestRetryTransport_ContextCanceled(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: 429, Body: io.NopCloser(strings.NewReader("rate limited"))},
		},
	}

	rt := NewRetryTransport(mock)
	rt.BaseDelay = 1 * time.Second // Long delay

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://example.com", nil)

	// Cancel context during the backoff
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	var resp *http.Response
	var err error

	if r, errCall := rt.RoundTrip(req); errCall != nil {
		err = errCall
	} else {
		resp = r
	}

	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRetryTransport_CircuitBreakerOpen(t *testing.T) {
	mock := &mockTransport{}

	rt := NewRetryTransport(mock)
	// Force circuit breaker open
	for i := 0; i < CircuitBreakerThreshold; i++ {
		rt.CircuitBreaker.RecordFailure()
	}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
	var resp *http.Response
	var err error

	if r, errCall := rt.RoundTrip(req); errCall != nil {
		err = errCall
	} else {
		resp = r
	}

	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if err == nil {
		t.Fatal("expected error when circuit breaker is open")
	}

	if !IsCircuitBreakerError(err) {
		t.Errorf("expected CircuitBreakerError, got %T", err)
	}

	if mock.calls != 0 {
		t.Errorf("expected 0 calls when circuit open, got %d", mock.calls)
	}
}

func TestRetryTransport_CircuitBreakerReset(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}

	rt := NewRetryTransport(mock)
	// Record failures but not enough to open
	for i := 0; i < CircuitBreakerThreshold-1; i++ {
		rt.CircuitBreaker.RecordFailure()
	}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
	var resp *http.Response

	if r, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else {
		resp = r
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// After success, failures should be reset
	if rt.CircuitBreaker.failures != 0 {
		t.Errorf("expected failures reset to 0, got %d", rt.CircuitBreaker.failures)
	}
}

func TestRetryTransport_RetryAfterHeader(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{
				StatusCode: 429,
				Header:     http.Header{"Retry-After": []string{"1"}},
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			},
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}

	rt := NewRetryTransport(mock)
	rt.BaseDelay = 1 * time.Hour // Would be very long without Retry-After

	start := time.Now()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
	var resp *http.Response

	if r, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else {
		resp = r
	}
	elapsed := time.Since(start)

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	// Should have waited ~1 second based on Retry-After header
	if elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("expected ~1s delay from Retry-After, got %v", elapsed)
	}
}

func TestRetryTransport_InvalidGrant(t *testing.T) {
	mock := &mockTransport{
		errors: []error{
			fmt.Errorf("oauth2: cannot fetch token: 400 Bad Request\nResponse: {\"error\":\"invalid_grant\",\"error_description\":\"Token has been expired or revoked.\"}"),
		},
	}

	rt := NewRetryTransport(mock)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)

	resp, err := rt.RoundTrip(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err == nil {
		t.Fatal("expected error")
	}

	if !IsInvalidGrantError(err) {
		t.Errorf("expected InvalidGrantError, got %T: %v", err, err)
	}

	// Verify the error message includes re-auth guidance
	if !strings.Contains(err.Error(), "--force-consent") {
		t.Errorf("expected --force-consent hint in error: %v", err)
	}
}

func TestRetryTransport_NonInvalidGrantError(t *testing.T) {
	mock := &mockTransport{
		errors: []error{
			errors.New("network connection refused"),
		},
	}

	rt := NewRetryTransport(mock)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)

	resp, err := rt.RoundTrip(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err == nil {
		t.Fatal("expected error")
	}

	if IsInvalidGrantError(err) {
		t.Errorf("should NOT be InvalidGrantError for a network error")
	}
}

func TestRetryTransport_401_Retry(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: 401, Body: io.NopCloser(strings.NewReader("unauthorized"))},
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}

	rt := NewRetryTransport(mock)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
	var resp *http.Response

	if r, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else {
		resp = r
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}

	if mock.calls != 2 {
		t.Errorf("expected 2 calls (1 retry), got %d", mock.calls)
	}
}

func TestRetryTransport_401_MaxRetries(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: 401, Body: io.NopCloser(strings.NewReader("unauthorized"))},
			{StatusCode: 401, Body: io.NopCloser(strings.NewReader("unauthorized"))},
			{StatusCode: 401, Body: io.NopCloser(strings.NewReader("unauthorized"))},
		},
	}

	rt := NewRetryTransport(mock)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)
	var resp *http.Response

	if r, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else {
		resp = r
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != 401 {
		t.Errorf("expected 401 after max retries, got %d", resp.StatusCode)
	}
	// 1 initial + 1 retry = 2 total (MaxUnauthorizedRetries = 1)

	if mock.calls != 2 {
		t.Errorf("expected 2 calls, got %d", mock.calls)
	}
}

func TestRetryTransport_CalculateBackoff_NoPanic(t *testing.T) {
	rt := NewRetryTransport(&mockTransport{})
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("calculateBackoff panicked: %v", r)
		}
	}()

	rt.BaseDelay = 0
	_ = rt.calculateBackoff(0, resp)

	rt.BaseDelay = 1 * time.Nanosecond
	_ = rt.calculateBackoff(0, resp)
}

func TestRetryTransport_WithRequestBody(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: 429, Body: io.NopCloser(strings.NewReader("rate limited"))},
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}

	rt := NewRetryTransport(mock)
	rt.BaseDelay = 1 * time.Millisecond

	body := strings.NewReader("request body")
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "https://example.com", body)
	var resp *http.Response

	if r, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	} else {
		resp = r
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}

	if mock.calls != 2 {
		t.Errorf("expected 2 calls, got %d", mock.calls)
	}
}

func TestEnsureReplayableBody(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "https://example.com", io.NopCloser(strings.NewReader("hello")))
	if req.GetBody != nil {
		t.Fatalf("expected nil GetBody")
	}

	if err := ensureReplayableBody(req); err != nil {
		t.Fatalf("ensureReplayableBody: %v", err)
	}

	if req.GetBody == nil {
		t.Fatalf("expected GetBody to be set")
	}
	var first []byte

	if b, err := io.ReadAll(req.Body); err != nil {
		t.Fatalf("read body: %v", err)
	} else {
		first = b
	}
	_ = req.Body.Close()
	var body io.ReadCloser

	if b, err := req.GetBody(); err != nil {
		t.Fatalf("GetBody: %v", err)
	} else {
		body = b
	}
	var second []byte

	if b, err := io.ReadAll(body); err != nil {
		t.Fatalf("read replay body: %v", err)
	} else {
		second = b
	}
	_ = body.Close()

	if string(first) != "hello" || string(second) != "hello" {
		t.Fatalf("unexpected body replay: %q %q", string(first), string(second))
	}
}
