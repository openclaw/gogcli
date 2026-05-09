package cmd

import (
	"context"
	"io"
	"net/http"
	"testing"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func newCalendarServiceForTest(t *testing.T, h http.Handler) (*calendar.Service, func()) {
	t.Helper()

	return newGoogleTestService(t, h, calendar.NewService)
}

func newTestCalendarService(t *testing.T, h http.Handler) (*calendar.Service, func()) {
	t.Helper()
	return newCalendarServiceForTest(t, h)
}

func stubCalendarServiceForTest(t *testing.T, svc *calendar.Service) {
	t.Helper()
	stubGoogleTestService(t, &newCalendarService, svc)
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
