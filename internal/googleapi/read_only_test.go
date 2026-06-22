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

	queryRequest, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://www.googleapis.com/calendar/v3/freeBusy", nil)
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

func TestReadOnlyPOSTAllowlist(t *testing.T) {
	allowed := []string{
		"https://www.googleapis.com/calendar/v3/freeBusy",
		"https://searchconsole.googleapis.com/webmasters/v3/sites/example/searchAnalytics/query",
		"https://searchconsole.googleapis.com/v1/urlInspection/index:inspect",
		"https://photoslibrary.googleapis.com/v1/mediaItems:search",
		"https://sheets.googleapis.com/v4/spreadsheets/id/values:batchGetByDataFilter",
		"https://driveactivity.googleapis.com/v2/activity:query",
		"https://analyticsdata.googleapis.com/v1beta/properties/1:runReport",
	}
	for _, requestURL := range allowed {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, requestURL, nil)
		if err != nil {
			t.Fatal(err)
		}

		if !readOnlyPOSTRequest(request) {
			t.Errorf("readOnlyPOSTRequest(%q) = false, want true", requestURL)
		}
	}

	blocked := []string{
		"http://www.googleapis.com/calendar/v3/freeBusy",
		"https://example.test/calendar/v3/freeBusy",
		"https://www.googleapis.com/v2/activity:query",
		"https://driveactivity.googleapis.com/v2/items:query",
		"https://sheets.googleapis.com/v4/spreadsheets/id:batchUpdate",
	}
	for _, requestURL := range blocked {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, requestURL, nil)
		if err != nil {
			t.Fatal(err)
		}

		if readOnlyPOSTRequest(request) {
			t.Errorf("readOnlyPOSTRequest(%q) = true, want false", requestURL)
		}
	}

	override, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://www.googleapis.com/calendar/v3/freeBusy", nil)
	if err != nil {
		t.Fatal(err)
	}

	override.Header.Set("X-HTTP-Method-Override", http.MethodDelete)

	if ReadOnlyRequestAllowed(override) {
		t.Error("POST with X-HTTP-Method-Override unexpectedly allowed")
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
