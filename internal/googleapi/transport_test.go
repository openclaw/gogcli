package googleapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
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
	recordBackoff(rt)

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

func TestRetryTransport_RateLimit_ResponseCeiling(t *testing.T) {
	const ceiling = 12

	mock := &mockTransport{responses: make([]*http.Response, ceiling)}
	for i := range mock.responses {
		mock.responses[i] = &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader("rate limited")),
		}
	}
	rt := NewRetryTransport(mock)
	rt.BaseDelay = 0
	delays := recordBackoff(rt)

	resp := roundTrip(t, rt)
	defer resp.Body.Close()

	if mock.calls != ceiling || resp != mock.responses[ceiling-1] || len(*delays) != 0 {
		t.Fatalf("calls = %d, status = %d, waits = %v; want the twelfth response, with no waits", mock.calls, resp.StatusCode, *delays)
	}

	if body, err := io.ReadAll(resp.Body); err != nil || string(body) != "rate limited" {
		t.Fatalf("body = %q, err = %v; want the original rate-limit body", body, err)
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
	synctest.Test(t, func(t *testing.T) {
		mock := &mockTransport{responses: []*http.Response{{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader("rate limited")),
		}}}
		rt := NewRetryTransport(mock)
		rt.nowFn = time.Now
		rt.sleepFn = func(ctx context.Context, _ time.Duration) error {
			<-ctx.Done()
			return ctx.Err()
		}

		// Seed the shared gate without retrying, then cancel a new request while it waits.
		seed, _ := http.NewRequestWithContext(WithoutRetries(t.Context()), http.MethodGet, "https://example.com", nil)

		resp, err := rt.RoundTrip(seed)
		if err != nil || resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("seeding the gate: response = %v, error = %v", resp, err)
		}
		_ = resp.Body.Close()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		body := &closeTracker{}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com", body)
		result := make(chan error, 1)

		go func() {
			resp, err := rt.RoundTrip(req)
			if resp != nil {
				_ = resp.Body.Close()
			}

			result <- err
		}()

		synctest.Wait()

		if mock.calls != 1 {
			t.Fatalf("base calls while paused = %d, want 1", mock.calls)
		}

		cancel()

		if err := <-result; !errors.Is(err, context.Canceled) || !body.closed {
			t.Fatalf("error = %v, request body closed = %t; want cancellation with the body closed", err, body.closed)
		}
	})
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
	for _, tt := range []struct {
		header string
		wait   time.Duration
	}{
		{header: "1", wait: time.Second},
		{header: "600", wait: time.Minute},
	} {
		t.Run(tt.header, func(t *testing.T) {
			limited := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": {tt.header}},
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}
			rt := NewRetryTransport(&mockTransport{responses: []*http.Response{limited}})
			rt.BaseDelay = 10 * time.Second
			delays := recordBackoff(rt)

			resp := roundTrip(t, rt)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK || len(*delays) != 2 || (*delays)[0] != tt.wait {
				t.Fatalf("status = %d, waits = %v; want success after %v and a release spread", resp.StatusCode, *delays, tt.wait)
			}
		})
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
	_ = rt.calculateBackoff(0, resp, time.Now())

	rt.BaseDelay = 1 * time.Nanosecond
	_ = rt.calculateBackoff(0, resp, time.Now())
}

func TestRetryTransport_WithRequestBody(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: 429, Body: io.NopCloser(strings.NewReader("rate limited"))},
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}

	rt := NewRetryTransport(mock)
	recordBackoff(rt)

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

	req.ContentLength = int64(len("hello"))
	if req.GetBody != nil {
		t.Fatalf("expected nil GetBody")
	}

	replayable, err := ensureReplayableBody(req)
	if err != nil {
		t.Fatalf("ensureReplayableBody: %v", err)
	}

	if !replayable {
		t.Fatalf("expected replayable body")
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

func rateLimit403Body(reason string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(`{"error":{"code":403,"message":"User-rate limit exceeded.","errors":[{"message":"User-rate limit exceeded.","domain":"usageLimits","reason":"` + reason + `"}],"status":"PERMISSION_DENIED"}}`))
}

// recordBackoff replaces the transport's backoff wait and returns the delays
// it was asked to wait, so retry timing is asserted without real sleeps.
func recordBackoff(rt *RetryTransport) *[]time.Duration {
	now := time.Unix(0, 0)
	delays := &[]time.Duration{}
	rt.nowFn = func() time.Time { return now }
	rt.sleepFn = func(_ context.Context, d time.Duration) error {
		*delays = append(*delays, d)
		now = now.Add(d)

		return nil
	}

	return delays
}

func roundTrip(t *testing.T, rt *RetryTransport) *http.Response {
	t.Helper()

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com", nil)

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return resp
}

func TestRetryTransport_RateLimit403_Retry(t *testing.T) {
	for _, reason := range []string{"rateLimitExceeded", "userRateLimitExceeded"} {
		t.Run(reason, func(t *testing.T) {
			mock := &mockTransport{
				responses: []*http.Response{
					{StatusCode: http.StatusForbidden, Body: rateLimit403Body(reason)},
					{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
				},
			}

			rt := NewRetryTransport(mock)
			rt.BaseDelay = time.Second
			delays := recordBackoff(rt)

			resp := roundTrip(t, rt)
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
			}

			if mock.calls != 2 {
				t.Errorf("expected 2 calls (1 retry), got %d", mock.calls)
			}

			// Exponential backoff with up to 50% jitter: 1s..1.5s for the first retry.
			if len(*delays) != 2 || (*delays)[0] < time.Second || (*delays)[0] >= 1500*time.Millisecond {
				t.Errorf("backoff delays = %v, want [1s, 1.5s) followed by a release spread", *delays)
			}
		})
	}
}

func TestRetryTransport_RateLimit403_RetryAfterHeader(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Retry-After": []string{"7"}},
				Body:       rateLimit403Body("rateLimitExceeded"),
			},
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
		},
	}

	rt := NewRetryTransport(mock)
	rt.BaseDelay = time.Hour // Exponential backoff would ask for at least an hour.
	delays := recordBackoff(rt)

	resp := roundTrip(t, rt)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	if len(*delays) != 2 || (*delays)[0] != 7*time.Second {
		t.Errorf("backoff delays = %v, want 7s from Retry-After followed by a release spread", *delays)
	}
}

func TestRetryTransport_RateLimit403_ResponseCeilingKeepsBody(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{
			{StatusCode: http.StatusForbidden, Body: rateLimit403Body("rateLimitExceeded")},
			{StatusCode: http.StatusForbidden, Body: rateLimit403Body("rateLimitExceeded")},
			{StatusCode: http.StatusForbidden, Body: rateLimit403Body("rateLimitExceeded")},
		},
	}

	rt := NewRetryTransport(mock)
	rt.MaxRetries429 = 2
	delays := recordBackoff(rt)

	resp := roundTrip(t, rt)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 after max retries, got %d", resp.StatusCode)
	}

	if mock.calls != 2 || len(*delays) != 2 {
		t.Errorf("calls = %d, backoff delays = %v; want 2 calls and one gate wait plus spread", mock.calls, *delays)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if !strings.Contains(string(body), `"reason":"rateLimitExceeded"`) {
		t.Errorf("rate-limit body not preserved for the caller: %q", body)
	}
}

func TestRetryTransport_RateLimit403And429ShareOneBudget(t *testing.T) {
	t.Run("mixed responses retry until success", func(t *testing.T) {
		mock := &mockTransport{
			responses: []*http.Response{
				{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("rate limited"))},
				{StatusCode: http.StatusForbidden, Body: rateLimit403Body("userRateLimitExceeded")},
				{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
			},
		}

		rt := NewRetryTransport(mock)
		delays := recordBackoff(rt)

		resp := roundTrip(t, rt)
		defer resp.Body.Close()

		if resp.StatusCode != 200 || mock.calls != 3 || len(*delays) != 4 {
			t.Errorf("status = %d, calls = %d, delays = %v; want 200 after 3 calls and two gate waits plus spreads", resp.StatusCode, mock.calls, *delays)
		}
	})

	t.Run("the budget counts both kinds", func(t *testing.T) {
		mock := &mockTransport{
			responses: []*http.Response{
				{StatusCode: http.StatusForbidden, Body: rateLimit403Body("rateLimitExceeded")},
				{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("rate limited"))},
				{StatusCode: http.StatusForbidden, Body: rateLimit403Body("rateLimitExceeded")},
				{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))},
			},
		}

		rt := NewRetryTransport(mock)
		rt.MaxRetries429 = 3
		delays := recordBackoff(rt)

		resp := roundTrip(t, rt)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden || mock.calls != 3 || len(*delays) != 4 {
			t.Errorf("status = %d, calls = %d, delays = %v; want the third response (403) after two gate waits plus spreads", resp.StatusCode, mock.calls, *delays)
		}
	})
}

func TestRetryTransport_Forbidden_NoRetry(t *testing.T) {
	bodies := map[string]string{
		"forbidden":          `{"error":{"code":403,"errors":[{"reason":"forbidden"}]}}`,
		"dailyLimitExceeded": `{"error":{"code":403,"errors":[{"reason":"dailyLimitExceeded"}]}}`,
		"quotaExceeded":      `{"error":{"code":403,"errors":[{"reason":"quotaExceeded"}]}}`,
		"non-JSON body":      `rateLimitExceeded`,
		"truncated JSON":     `{"error":{"code":403,"errors":[{"reason":"rateLimitExceeded"`,
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			mock := &mockTransport{
				responses: []*http.Response{{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(body))}},
			}

			rt := NewRetryTransport(mock)
			delays := recordBackoff(rt)

			resp := roundTrip(t, rt)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusForbidden || mock.calls != 1 || len(*delays) != 0 {
				t.Errorf("status = %d, calls = %d, delays = %v; want the 403 once, without retry", resp.StatusCode, mock.calls, *delays)
			}

			got, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}

			if string(got) != body {
				t.Errorf("403 body = %q, want %q preserved for the caller", got, body)
			}
		})
	}
}

// prefixThenErrorReader returns its prefix with err on the first read and a
// clean EOF afterwards, like a body whose connection broke mid-response.
type prefixThenErrorReader struct {
	prefix string
	err    error
	done   bool
}

func (r *prefixThenErrorReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}

	r.done = true

	return copy(p, r.prefix), r.err
}

func TestRetryTransport_Forbidden_KeepsBodyReadError(t *testing.T) {
	mock := &mockTransport{
		responses: []*http.Response{{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(&prefixThenErrorReader{prefix: `{"error":{"errors":[{"reas`, err: io.ErrUnexpectedEOF}),
		}},
	}

	rt := NewRetryTransport(mock)
	// With RefreshAuth the insufficient-scope check inspects the body as well.
	rt.RefreshAuth = func(context.Context) error { return nil }

	resp := roundTrip(t, rt)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read error = %v, want io.ErrUnexpectedEOF preserved for the caller", err)
	}

	if string(body) != `{"error":{"errors":[{"reas` {
		t.Fatalf("body = %q, want the bytes read before the error", body)
	}

	if mock.calls != 1 {
		t.Errorf("expected 1 call (a truncated body is not a rate limit), got %d", mock.calls)
	}
}

func TestRetryTransport_RateLimit403_DefaultBudgetOutlastsPerMinuteQuota(t *testing.T) {
	// mockTransport answers 200 once its scripted responses run out. Eleven
	// rejections stay below the 12-response ceiling.
	mock := &mockTransport{responses: make([]*http.Response, MaxRateLimitRetries-1)}
	for i := range mock.responses {
		mock.responses[i] = &http.Response{StatusCode: http.StatusForbidden, Body: rateLimit403Body("rateLimitExceeded")}
	}

	rt := NewRetryTransport(mock)
	delays := recordBackoff(rt)

	resp := roundTrip(t, rt)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 after retries, got %d", resp.StatusCode)
	}

	var total time.Duration
	for _, d := range *delays {
		total += d
	}

	// Gmail's "Units per minute per user" quota refills over a minute, so a
	// request that keeps getting rejected must keep backing off for at least
	// that long before the ceiling ends its retries.
	if total < time.Minute || total > MaxRateLimitWait {
		t.Errorf("total backoff = %v over %d waits, want between 1m and %v", total, len(*delays), MaxRateLimitWait)
	}
}

func TestRetryTransport_RateLimit_SharedGate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		type call struct {
			path string
			at   time.Time
		}
		calls := make(chan call, 4)
		firstLimited := false
		rt := NewRetryTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls <- call{req.URL.Path, time.Now()}

			if req.URL.Path == "/first" && !firstLimited {
				firstLimited = true

				return &http.Response{
					StatusCode: http.StatusForbidden,
					Header:     http.Header{"Retry-After": {"2"}},
					Body:       rateLimit403Body("userRateLimitExceeded"),
				}, nil
			}

			if req.URL.Path == "/extend" || req.URL.Path == "/shorter" {
				resp := newTestResponse(http.StatusTooManyRequests, "rate limited")

				after := "3"
				if req.URL.Path == "/shorter" {
					after = "1"
				}

				resp.Header.Set("Retry-After", after)

				return resp, nil
			}

			return newTestResponse(http.StatusOK, "ok"), nil
		}))
		rt.nowFn = time.Now
		rt.sleepFn = func(ctx context.Context, d time.Duration) error {
			select {
			case <-time.After(d): // synctest advances this timer on its fake clock.
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		results := make(chan error, 3)
		startRequest := func(path string) {
			go func() {
				req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com"+path, nil)

				resp, err := rt.RoundTrip(req)
				if resp != nil {
					_ = resp.Body.Close()
				}

				results <- err
			}()
		}

		start := time.Now()

		startRequest("/first")
		synctest.Wait()

		if len(calls) != 1 {
			t.Fatalf("base calls after first response = %d, want 1", len(calls))
		}

		if got := <-calls; got.path != "/first" || !got.at.Equal(start) {
			t.Fatalf("initial call = %+v, want /first at %v", got, start)
		}

		startRequest("/second")
		startRequest("/third")
		time.Sleep(time.Second)
		synctest.Wait()

		if len(calls) != 0 {
			t.Fatalf("base calls during initial pause = %d, want 0", len(calls))
		}

		for _, path := range []string{"/extend", "/shorter"} {
			// Single-attempt calls bypass an active gate but still report rate limits.
			req, _ := http.NewRequestWithContext(WithoutRetries(t.Context()), http.MethodGet, "https://example.com"+path, nil)

			resp, err := rt.RoundTrip(req)
			if err != nil || resp.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("%s: response = %v, error = %v", path, resp, err)
			}
			_ = resp.Body.Close()

			if len(calls) != 1 {
				t.Fatalf("%s: base calls = %d, want the single bypass call", path, len(calls))
			}

			if got := <-calls; got.path != path || got.at.Sub(start) != time.Second {
				t.Fatalf("bypass call = %+v, want %s at 1s", got, path)
			}
		}

		time.Sleep(3*time.Second - time.Nanosecond)
		synctest.Wait()

		if len(calls) != 0 {
			t.Fatalf("base calls during shared pause = %d, want 0", len(calls))
		}

		for range 3 {
			if err := <-results; err != nil {
				t.Fatalf("round trip: %v", err)
			}
		}

		if len(calls) != 3 {
			t.Fatalf("base calls after the shared pause = %d, want 3", len(calls))
		}
		seen := make(map[string]bool)

		for range 3 {
			got := <-calls

			elapsed := got.at.Sub(start)
			if elapsed < 4*time.Second || elapsed > 4250*time.Millisecond || seen[got.path] {
				t.Errorf("unexpected base call %+v at %v", got, elapsed)
			}
			seen[got.path] = true
		}
	})
}

func TestRetryTransport_RateLimit_SharedStreak(t *testing.T) {
	statuses := []int{
		http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusOK, // Leave the streak at one.
		http.StatusTooManyRequests, http.StatusOK, // The next request starts at the second backoff.
		http.StatusOK, http.StatusOK, // Decay to zero and stay there.
		http.StatusTooManyRequests, http.StatusOK, // The next rate limit uses the base delay.
	}
	mock := &mockTransport{responses: make([]*http.Response, len(statuses))}

	for i, status := range statuses {
		mock.responses[i] = &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(http.StatusText(status))),
		}
	}

	rt := NewRetryTransport(mock)
	delays := recordBackoff(rt)

	for range 5 {
		resp := roundTrip(t, rt)

		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	}

	want := []time.Duration{time.Second, 2 * time.Second, 2 * time.Second, time.Second}
	if len(*delays) != 2*len(want) {
		t.Fatalf("waits = %v, want four backoffs each followed by a release spread", *delays)
	}

	for i, base := range want {
		delay, spread := (*delays)[2*i], (*delays)[2*i+1]
		if delay < base || delay >= base+base/2 || spread < 0 || spread > 250*time.Millisecond {
			t.Errorf("wait %d = %v + %v, want backoff in [%v, %v) plus up to 250ms spread", i, delay, spread, base, base+base/2)
		}
	}
}

func TestRetryTransport_RateLimit_TimeBudget(t *testing.T) {
	mock := &mockTransport{responses: make([]*http.Response, 4)}
	for i := range mock.responses {
		mock.responses[i] = &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Retry-After": {"50"}},
			Body:       io.NopCloser(strings.NewReader("still rate limited")),
		}
	}
	rt := NewRetryTransport(mock)
	recordBackoff(rt)
	start := rt.now()

	resp := roundTrip(t, rt)
	defer resp.Body.Close()

	if mock.calls != 3 || resp != mock.responses[2] || rt.now().Sub(start) != 120*time.Second {
		t.Fatalf("calls = %d, status = %d, waited %v; want the third response after 120s", mock.calls, resp.StatusCode, rt.now().Sub(start))
	}

	if body, err := io.ReadAll(resp.Body); err != nil || string(body) != "still rate limited" {
		t.Fatalf("body = %q, err = %v; want the original rate-limit body", body, err)
	}
}

func TestRetryTransport_RateLimit_TimeBudgetIncludesServerErrorRetry(t *testing.T) {
	limited := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": {"50"}},
		Body:       io.NopCloser(strings.NewReader("still rate limited")),
	}
	mock := &mockTransport{responses: []*http.Response{
		{StatusCode: limited.StatusCode, Header: limited.Header},
		limited,
		{StatusCode: http.StatusServiceUnavailable},
	}}
	rt := NewRetryTransport(mock)
	recordBackoff(rt)
	start := rt.now()
	sleep := rt.sleepFn
	rt.sleepFn = func(ctx context.Context, delay time.Duration) error {
		if delay == ServerErrorRetryDelay {
			// Another request is rate-limited during this request's 5xx retry delay.
			rt.recordRateLimit(&http.Response{Header: http.Header{"Retry-After": {"60"}}}, rt.now())
		}

		return sleep(ctx, delay)
	}

	resp := roundTrip(t, rt)
	defer resp.Body.Close()

	if mock.calls != 3 || resp != limited || rt.now().Sub(start) != 120*time.Second {
		t.Fatalf("calls = %d, status = %d, waited %v; want the last rate-limit response after 120s", mock.calls, resp.StatusCode, rt.now().Sub(start))
	}

	if body, err := io.ReadAll(resp.Body); err != nil || string(body) != "still rate limited" {
		t.Fatalf("body = %q, err = %v; want the original rate-limit body", body, err)
	}
}

func TestRetryTransport_RateLimit_InFlightWaveEscalatesOnce(t *testing.T) {
	rt := NewRetryTransport(&mockTransport{})
	now := time.Unix(1000, 0)
	rt.nowFn = func() time.Time { return now }

	// recordRateLimit only reads the status and headers, so no body is needed.
	reject := func(retryAfter string, sentAt time.Time) time.Duration {
		header := http.Header{}
		if retryAfter != "" {
			header.Set("Retry-After", retryAfter)
		}

		_, delay := rt.recordRateLimit(&http.Response{StatusCode: http.StatusTooManyRequests, Header: header}, sentAt)

		return delay
	}

	// Ten concurrent requests leave together; the first rejection escalates.
	sent := now
	now = now.Add(100 * time.Millisecond)

	first := reject("", sent)
	if first < time.Second || first >= 1500*time.Millisecond {
		t.Fatalf("first delay = %v, want the base backoff", first)
	}

	pause := rt.pauseUntil

	// The other nine were already in flight and must not escalate or move the pause.
	for range 9 {
		now = now.Add(10 * time.Millisecond)

		reject("", sent)
	}

	if rt.rateLimitStreak != 1 {
		t.Fatalf("streak = %d after one in-flight wave, want 1", rt.rateLimitStreak)
	}

	if !rt.pauseUntil.Equal(pause) {
		t.Fatalf("in-flight rejections moved the pause from %v to %v", pause, rt.pauseUntil)
	}

	// An in-flight rejection that names a longer wait still extends the pause.
	reject("30", sent)

	if want := now.Add(30 * time.Second); !rt.pauseUntil.Equal(want) {
		t.Fatalf("pauseUntil = %v, want Retry-After end %v", rt.pauseUntil, want)
	}

	if rt.rateLimitStreak != 1 {
		t.Fatalf("streak = %d after Retry-After from the same wave, want 1", rt.rateLimitStreak)
	}

	// A request sent after the escalation is new evidence and escalates.
	now = now.Add(time.Second)
	reject("", now)

	if rt.rateLimitStreak != 2 {
		t.Fatalf("streak = %d after a fresh rejection, want 2", rt.rateLimitStreak)
	}
}
