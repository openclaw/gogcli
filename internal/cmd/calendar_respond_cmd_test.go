package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

func TestCalendarRespondCmd_Text(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case strings.Contains(path, "/calendars/cal1@example.com/events/evt1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "evt1",
				"summary": "Meeting",
				"attendees": []map[string]any{
					{"email": "a@b.com", "self": true},
				},
				// Invalid overrides payload: missing "minutes". Old code PATCHed full event, triggering API validation.
				"reminders": map[string]any{
					"useDefault": false,
					"overrides": []map[string]any{
						{"method": "popup"},
					},
				},
			})
			return
		case strings.Contains(path, "/calendars/cal1@example.com/events/evt1") && r.Method == http.MethodPatch:
			body, _ := io.ReadAll(r.Body)
			var patch map[string]any
			if err := json.Unmarshal(body, &patch); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			if _, ok := patch["reminders"]; ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    400,
						"message": "Missing override reminder minutes",
					},
				})
				return
			}
			if _, ok := patch["attendees"]; !ok {
				t.Fatalf("PATCH missing attendees. body=%s", string(body))
			}
			for k := range patch {
				if k != "attendees" {
					t.Fatalf("PATCH should only contain attendees; got key %q. body=%s", k, string(body))
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "evt1",
				"summary":  "Meeting",
				"htmlLink": "http://example.com/event",
			})
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

	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &CalendarRespondCmd{}
		if err := runKong(t, cmd, []string{"cal1@example.com", "evt1", "--status", "accepted", "--comment", "ok"}, ctx, flags); err != nil {
			t.Fatalf("respond: %v", err)
		}
	})
	if !strings.Contains(out, "response_status") {
		t.Fatalf("unexpected output: %q", out)
	}
}
