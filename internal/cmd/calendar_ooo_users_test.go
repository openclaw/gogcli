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
	"google.golang.org/api/people/v1"

	"github.com/openclaw/gogcli/internal/ui"
)

func TestCalendarOOOCmd_JSON(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/events") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["eventType"] != "outOfOffice" {
				t.Fatalf("expected outOfOffice eventType, got %#v", body["eventType"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "evt1",
				"summary": "Out of office",
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result := executeWithCalendarTestService(t, []string{"--json", "--account", "a@b.com", "calendar", "ooo", "--from", "2025-01-01T09:00:00Z", "--to", "2025-01-01T17:00:00Z"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout
	if !strings.Contains(out, "event") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCalendarOOOCmdRejectsDateOnlyAndAllDay(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = withDefaultTestRuntime(ctx)
	flags := &RootFlags{Account: "a@b.com"}

	if err := (&CalendarOOOCmd{From: "2025-01-01", To: "2025-01-02"}).Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "out-of-office requires RFC3339 datetime") {
		t.Fatalf("expected date-only OOO validation error, got %v", err)
	}
	if err := (&CalendarOOOCmd{From: "2025-01-01T09:00:00Z", To: "2025-01-01T17:00:00Z", AllDay: true}).Run(ctx, flags); err == nil || !strings.Contains(err.Error(), "cannot be all-day") {
		t.Fatalf("expected all-day OOO validation error, got %v", err)
	}
}

func TestCalendarUsersCmd_TextAndJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "people:listDirectoryPeople") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"people": []map[string]any{
				{
					"names":          []map[string]any{{"displayName": "User One"}},
					"emailAddresses": []map[string]any{{"value": "user@example.com"}},
				},
			},
			"nextPageToken": "npt",
		})
	}))
	defer srv.Close()

	svc, err := people.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	textResult := executeWithPeopleDirectoryTestService(t, []string{"--account", "a@b.com", "calendar", "users", "--max", "1"}, svc)
	if textResult.err != nil {
		t.Fatalf("Execute: %v", textResult.err)
	}
	if !strings.Contains(textResult.stderr, "Tip: Use any email") {
		t.Fatalf("unexpected stderr: %q", textResult.stderr)
	}
	if !strings.Contains(textResult.stdout, "user@example.com") {
		t.Fatalf("unexpected text output: %q", textResult.stdout)
	}

	jsonResult := executeWithPeopleDirectoryTestService(t, []string{"--json", "--account", "a@b.com", "calendar", "users", "--max", "1"}, svc)
	if jsonResult.err != nil {
		t.Fatalf("Execute: %v", jsonResult.err)
	}
	if !strings.Contains(jsonResult.stdout, "users") {
		t.Fatalf("unexpected json output: %q", jsonResult.stdout)
	}
}
