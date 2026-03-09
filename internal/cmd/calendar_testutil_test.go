package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func newCalendarServiceForTest(t *testing.T, h http.Handler) (*calendar.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(h)
	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		srv.Close()
		t.Fatalf("NewService: %v", err)
	}
	return svc, srv.Close
}

func newTestCalendarService(t *testing.T, h http.Handler) (*calendar.Service, func()) {
	t.Helper()
	return newCalendarServiceForTest(t, h)
}

func newCalendarOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()

	u, err := ui.New(ui.Options{Stdout: stdout, Stderr: stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func newCalendarJSONContext(t *testing.T) context.Context {
	t.Helper()
	return newCalendarJSONOutputContext(t, io.Discard, io.Discard)
}

func newCalendarJSONOutputContext(t *testing.T, stdout, stderr io.Writer) context.Context {
	t.Helper()
	return outfmt.WithMode(newCalendarOutputContext(t, stdout, stderr), outfmt.Mode{JSON: true})
}
