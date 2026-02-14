package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestCalendarUpdateAndDelete(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case strings.Contains(path, "/calendars/cal1@example.com/events/evt1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "evt1",
				"summary":  "Old",
				"start":    map[string]any{"dateTime": "2025-01-01T10:00:00Z"},
				"end":      map[string]any{"dateTime": "2025-01-01T11:00:00Z"},
				"htmlLink": "http://example.com/event",
			})
			return
		case strings.Contains(path, "/calendars/cal1@example.com/events/evt1") && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "evt1",
				"summary":  "Updated",
				"htmlLink": "http://example.com/event",
			})
			return
		case strings.Contains(path, "/calendars/cal1@example.com/events/evt1") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	jsonCtx := outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	// update requires changes
	updateCmd := &CalendarUpdateCmd{}
	if err := runKong(t, updateCmd, []string{"cal1@example.com", "evt1"}, ctx, flags); err == nil {
		t.Fatalf("expected no updates error")
	}

	// update json
	jsonOut := captureStdout(t, func() {
		updateCmd = &CalendarUpdateCmd{}
		if err := runKong(t, updateCmd, []string{"cal1@example.com", "evt1", "--summary", "Updated"}, jsonCtx, flags); err != nil {
			t.Fatalf("update: %v", err)
		}
	})
	if !strings.Contains(jsonOut, "Updated") {
		t.Fatalf("unexpected update json: %q", jsonOut)
	}

	// delete json
	_ = captureStdout(t, func() {
		deleteCmd := &CalendarDeleteCmd{}
		if err := runKong(t, deleteCmd, []string{"cal1@example.com", "evt1"}, jsonCtx, flags); err != nil {
			t.Fatalf("delete: %v", err)
		}
	})
}
