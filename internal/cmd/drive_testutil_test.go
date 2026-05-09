package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
)

func newDriveTestService(t *testing.T, h http.Handler) (*drive.Service, func()) {
	t.Helper()

	return newGoogleTestService(t, h, drive.NewService)
}

func stubDriveService(svc *drive.Service) func(context.Context, string) (*drive.Service, error) {
	return func(context.Context, string) (*drive.Service, error) { return svc, nil }
}

func stubDriveServiceForTest(t *testing.T, svc *drive.Service) {
	t.Helper()
	stubGoogleTestService(t, &newDriveService, svc)
}

func newDriveMetadataTestService(t *testing.T, mimeType string) (*drive.Service, func()) {
	t.Helper()

	return newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files/id1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "id1",
			"name":     "Doc",
			"mimeType": mimeType,
		})
	}))
}

func requireQuery(t *testing.T, r *http.Request, key, want string) {
	t.Helper()
	if got := r.URL.Query().Get(key); got != want {
		t.Fatalf("expected %s=%s, got: %q (raw=%q)", key, want, got, r.URL.RawQuery)
	}
}

func requireSupportsAllDrives(t *testing.T, r *http.Request) {
	t.Helper()
	requireQuery(t, r, "supportsAllDrives", "true")
}

func readBody(t *testing.T, r *http.Request) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}
