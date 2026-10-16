package cmd

import "github.com/mark3labs/mcp-go/mcp"

func mcpNewDeleteTools() []mcpToolSpec {
	label := mcpToolSpec{Name: "gmail_delete_label", Service: "gmail", Risk: mcpRiskWrite, Capability: mcpCapabilityGmailSettingsDelete, Bounded: true, CommandPath: []string{"gmail", "labels", "delete"}, Description: "Delete a user label. Requires gmail_settings_delete, write authorization and operator startup --force. System labels remain protected. Returns a non-retryable bounded receipt.", Options: []mcp.ToolOption{mcp.WithString("label", mcp.Required())}}
	label.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, label); err != nil {
			return nil, err
		}
		id, err := mcpLiteralString(req, "label", "", true, 4096)
		if err != nil {
			return nil, err
		}
		return []string{"gmail", "labels", "delete", "--", id}, nil
	}
	filter := mcpToolSpec{Name: "gmail_delete_filter", Service: "gmail", Risk: mcpRiskWrite, Capability: mcpCapabilityGmailSettingsDelete, Bounded: true, CommandPath: []string{"gmail", "filters", "delete"}, Description: "Delete a Gmail filter. Requires gmail_settings_delete, write authorization and operator startup --force. Returns a non-retryable bounded receipt.", Options: []mcp.ToolOption{mcp.WithString("filter_id", mcp.Required())}}
	filter.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, filter); err != nil {
			return nil, err
		}
		id, err := mcpLiteralString(req, "filter_id", "", true, 4096)
		if err != nil {
			return nil, err
		}
		return []string{"gmail", "filters", "delete", "--", id}, nil
	}
	event := mcpToolSpec{Name: "calendar_cancel_event", Service: "calendar", Risk: mcpRiskWrite, Capability: mcpCapabilityCalendarDelete, Bounded: true, CommandPath: []string{"calendar", "delete"}, Description: "Cancel an event or recurring scope. Requires calendar_delete, write authorization and operator startup --force. Notifications additionally require calendar_notify; default none. Multi-step failure returns known progress and retry_safe=false.", Options: []mcp.ToolOption{mcp.WithString("calendar_id", mcp.DefaultString("primary")), mcp.WithString("event_id", mcp.Required()), mcp.WithString("scope", mcp.Enum("all", "single", "future")), mcp.WithString("original_start"), mcp.WithString("send_updates", mcp.DefaultString("none"), mcp.Enum("none", "all", "externalOnly"))}}
	event.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, event); err != nil {
			return nil, err
		}
		calendarID, err := mcpLiteralString(req, "calendar_id", "primary", true, 4096)
		if err != nil {
			return nil, err
		}
		eventID, err := mcpLiteralString(req, "event_id", "", true, 4096)
		if err != nil {
			return nil, err
		}
		mode, err := mcpCalendarNotificationMode(req)
		if err != nil {
			return nil, err
		}
		args, err := mcpCalendarScopeArgs(req, []string{"calendar", "delete", "--send-updates=" + mode})
		if err != nil {
			return nil, err
		}
		return append(args, "--", calendarID, eventID), nil
	}
	return []mcpToolSpec{label, filter, event}
}
