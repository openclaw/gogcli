package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/api/calendar/v3"
)

func TestMCPCalendarReadArgsAndSchemas(t *testing.T) {
	tools := mcpCalendarReadTools()
	if len(tools) != 4 {
		t.Fatal(len(tools))
	}
	for _, row := range []struct {
		name      string
		arguments map[string]any
		fragment  string
	}{
		{"calendar_list_calendars", map[string]any{"max": 25, "page": "opaque"}, "--page=opaque"},
		{"calendar_get_event", map[string]any{"event_id": "--literal", "timezone": "UTC"}, "-- primary --literal"},
		{"calendar_search_events", map[string]any{"query": "--literal @file", "page": "opaque"}, "--page=opaque"},
		{"calendar_freebusy", map[string]any{"calendars": []any{"work,calendar@example.invalid"}, "from": "2026-01-01T00:00:00Z", "to": "2026-01-02T00:00:00Z"}, "--calendar-id=work,calendar@example.invalid"},
	} {
		tool := findMCPTool(t, row.name)
		req := mcp.CallToolRequest{}
		req.Params.Arguments = row.arguments
		args, err := tool.BuildArgs(req)
		if err != nil || !strings.Contains(strings.Join(args, " "), row.fragment) {
			t.Fatal(row.name, args, err)
		}
	}
	tool := findMCPTool(t, "calendar_freebusy")
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"from": "2026-01-02T00:00:00Z", "to": "2026-01-01T00:00:00Z"}
	if _, err := tool.BuildArgs(req); err == nil {
		t.Fatal("reversed window accepted")
	}
}

func TestMCPCalendarReadRPCAndPaging(t *testing.T) {
	svc, closeServer := newGoogleTestService(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/events") && r.URL.Query().Get("pageToken") != "opaque" {
			t.Error(r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "nextPageToken": "next"})
	})), calendar.NewService)
	defer closeServer()
	result := executeWithCalendarTestService(t, []string{"--json", "--account", "fixture@example.invalid", "calendar", "search", "fixture", "--page=opaque"}, svc)
	if result.err != nil {
		t.Fatal(result.err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["nextPageToken"] != "next" {
		t.Fatal(payload)
	}
}
