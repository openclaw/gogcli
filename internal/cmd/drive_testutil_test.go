package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func newDriveTestService(t *testing.T, h http.Handler) (*drive.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(h)

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, srv.Close
}

func stubDriveService(svc *drive.Service) func(context.Context, string) (*drive.Service, error) {
	return func(context.Context, string) (*drive.Service, error) { return svc, nil }
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
