package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/config"
)

func TestMCPCalendarWriteFieldPresence(t *testing.T) {
	if len(mcpCalendarWriteTools()) != 4 {
		t.Fatal("missing writes")
	}
	tool := findMCPTool(t, "calendar_update_event")
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"event_id": "e1", "description": "", "location": "", "guests_can_modify": false, "attendees": []any{"fixture@example.invalid;comment=comma, stays"}, "attachment_urls": []any{"https://example.invalid/a,b"}, "start": "2026-01-01T00:00:00Z", "end": "2026-01-01T01:00:00Z", "timezone": "UTC"}
	args, err := tool.BuildArgs(req)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, fragment := range []string{"--description=", "--location=", "--guests-can-modify=false", "--attendee-entry=fixture@example.invalid;comment=comma, stays", "--attachment-url=https://example.invalid/a,b", "--start-timezone=UTC", "--end-timezone=UTC", "--send-updates=none"} {
		if !strings.Contains(joined, fragment) {
			t.Fatal(fragment, args)
		}
	}
	for _, arguments := range []map[string]any{{"event_id": "e1", "scope": "future", "description": "x"}, {"event_id": "e1", "timezone": "UTC", "start": "2026-01-01T00:00:00Z"}, {"event_id": "e1", "description": "x", "with_meet": "true"}, {"event_id": "e1", "description": strings.Repeat("x", 65537)}, {"event_id": "e1", "visibility": "arbitrary"}} {
		req.Params.Arguments = arguments
		if _, err = tool.BuildArgs(req); err == nil {
			t.Fatal("invalid write accepted", arguments)
		}
	}
}

func TestMCPCalendarUpdateSerializedPatch(t *testing.T) {
	patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInput{SourceTitle: "", LiteralAttendees: []string{}}, calendarUpdateFields{Summary: true, Description: true, Location: true, Attendees: true, SourceTitle: true})
	if err != nil || !changed {
		t.Fatal(err)
	}
	data, err := json.Marshal(patch)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err = json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"summary", "description", "location", "attendees", "source"} {
		if _, present := payload[key]; !present {
			t.Fatal("clear omitted", key, string(data))
		}
	}
}

func TestMCPCalendarLiteralArrays(t *testing.T) {
	input := calendarCreateInput{CalendarID: "primary", Summary: "fixture", From: "2026-01-01T00:00:00Z", To: "2026-01-01T01:00:00Z", LiteralAttendees: []string{"fixture@example.invalid;comment=comma, stays"}, Attachments: []string{"https://example.invalid/a,b"}}
	plan, err := buildCalendarCreatePlan(defaultConfigStoreForTest(t), input, calendarCreateFields{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Event.Attendees) != 1 || plan.Event.Attendees[0].Comment != "comma, stays" || len(plan.Event.Attachments) != 1 {
		t.Fatal(plan.Event)
	}
}

func TestMCPCalendarNotifyCeiling(t *testing.T) {
	tool := findMCPTool(t, "calendar_create_event")
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"send_updates": "all"}
	policy := config.MCPPolicy{AllowTools: []string{"*"}, AllowWrite: true}
	if err := requireMCPCapabilities(tool, req, policy); err == nil {
		t.Fatal("write glob gained notifications")
	}
	policy.AllowCalendarNotify = true
	if err := requireMCPCapabilities(tool, req, policy); err != nil {
		t.Fatal(err)
	}
	policy.AllowCalendarNotify = false
	req.Params.Arguments = map[string]any{"send_updates": "none"}
	if err := requireMCPCapabilities(tool, req, policy); err != nil {
		t.Fatal(err)
	}
	if hasMCPTool(mcpFilterTools(policy, nil, &RootFlags{GmailNoSend: true}), "calendar_respond") {
		t.Fatal("Gmail no-send authorized RSVP")
	}
	if _, err := mcpEnabledToolsWithPolicy(McpCmd{AllowCalendarNotify: true}, nil, policy); err == nil {
		t.Fatal("runtime widened notification policy")
	}
}

func TestMCPCalendarUpdateRejectsSourceTitleWithClear(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"event_id": "e1", "source_url": "", "source_title": "lost title"}
	if _, err := mcpCalendarEventWriteTool("update").BuildArgs(req); err == nil {
		t.Fatal("accepted a title that would be silently discarded while clearing source")
	}
}
