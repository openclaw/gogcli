package googleapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMutationObserverSingleAttemptAndOutcome(t *testing.T) {
	for _, status := range []int{200, 400, 500} {
		attempts := 0
		var observed []MutationAttempt
		ctx := WithMutationObserver(context.Background(), func(attempt MutationAttempt) { observed = append(observed, attempt) })
		base := mutationObserverTransportFromContext(ctx, roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"event"}`))}, nil
		}))
		transport := NewRetryTransport(base)
		transport.SingleAttemptWrites = true

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://calendar.example.invalid/calendars/primary/events", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}

		response, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()

		if attempts != 1 || len(observed) != 1 || observed[0].StatusCode != status || observed[0].Unknown != (status >= 500) {
			t.Fatal(attempts, observed)
		}
	}
}
