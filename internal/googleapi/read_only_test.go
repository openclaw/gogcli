package googleapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/keep/v1"
)

type readOnlyTestTransport struct {
	calls int
}

func (t *readOnlyTestTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++

	return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
}

func TestReadOnlyTransport(t *testing.T) {
	base := &readOnlyTestTransport{}
	transport := readOnlyTransportFromContext(WithReadOnly(context.Background(), true), base)

	readRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/items", nil)
	if err != nil {
		t.Fatal(err)
	}

	roundTripErr := readOnlyTestRoundTrip(transport, readRequest)
	if roundTripErr != nil {
		t.Fatalf("GET: %v", roundTripErr)
	}

	queryRequest, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.test/v3/freeBusy", nil)
	if err != nil {
		t.Fatal(err)
	}

	roundTripErr = readOnlyTestRoundTrip(transport, queryRequest)
	if roundTripErr != nil {
		t.Fatalf("query POST: %v", roundTripErr)
	}

	writeRequest, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.test/gmail/v1/users/me/messages/send", nil)
	if err != nil {
		t.Fatal(err)
	}

	roundTripErr = readOnlyTestRoundTrip(transport, writeRequest)

	if !errors.Is(roundTripErr, ErrReadOnly) {
		t.Fatalf("write error = %v, want ErrReadOnly", roundTripErr)
	}

	if base.calls != 2 {
		t.Fatalf("base calls = %d, want 2", base.calls)
	}
}

func TestReadOnlyTransportDisabled(t *testing.T) {
	base := &readOnlyTestTransport{}
	transport := readOnlyTransportFromContext(context.Background(), base)

	request, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, "https://example.test/items/1", nil)
	if err != nil {
		t.Fatal(err)
	}

	roundTripErr := readOnlyTestRoundTrip(transport, request)
	if roundTripErr != nil {
		t.Fatal(roundTripErr)
	}

	if base.calls != 1 {
		t.Fatalf("base calls = %d, want 1", base.calls)
	}
}

func TestKeepServiceAccountUsesReadOnlyTransport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service-account.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := WithReadOnly(context.Background(), true)

	svc, err := newKeepWithServiceAccount(ctx, path, "user@example.com", func(context.Context, []byte, string, []string) (oauth2.TokenSource, error) {
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test"}), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Notes.Create(&keep.Note{}).Do()

	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("create error = %v, want ErrReadOnly", err)
	}
}

func readOnlyTestRoundTrip(transport http.RoundTripper, request *http.Request) error {
	response, err := transport.RoundTrip(request)

	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}

	if err != nil {
		return fmt.Errorf("round trip: %w", err)
	}

	return nil
}
