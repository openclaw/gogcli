package googleapi

import (
	"context"
	"net/http"
)

// MutationAttempt describes one provider attempt. A 5xx or transport failure
// has an unknown outcome and must not authorize replay of a write.
type MutationAttempt struct {
	Method     string
	Path       string
	StatusCode int
	Unknown    bool
}
type mutationObserverKey struct{}

// WithMutationObserver enables single-attempt writes and reports their HTTP
// outcomes. Observers must be safe for simultaneous provider requests.
func WithMutationObserver(ctx context.Context, observer func(MutationAttempt)) context.Context {
	return context.WithValue(ctx, mutationObserverKey{}, observer)
}

func mutationObserverFromContext(ctx context.Context) func(MutationAttempt) {
	observer, _ := ctx.Value(mutationObserverKey{}).(func(MutationAttempt))
	return observer
}

func mutationObserverTransportFromContext(ctx context.Context, base http.RoundTripper) http.RoundTripper {
	observer := mutationObserverFromContext(ctx)
	if observer == nil {
		return base
	}

	return &mutationObserverTransport{base: base, observer: observer}
}

type mutationObserverTransport struct {
	base     http.RoundTripper
	observer func(MutationAttempt)
}

//nolint:wrapcheck // observing a request must preserve the transport result and error
func (t *mutationObserverTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		attempt := MutationAttempt{Method: req.Method, Path: req.URL.Path, Unknown: err != nil}
		if resp != nil {
			attempt.StatusCode = resp.StatusCode
			attempt.Unknown = attempt.Unknown || resp.StatusCode >= 500
		}

		t.observer(attempt)
	}

	return resp, err
}
