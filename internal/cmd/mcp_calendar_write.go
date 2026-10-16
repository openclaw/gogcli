package cmd

import (
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func mcpCalendarWriteTools() []mcpToolSpec {
	return []mcpToolSpec{mcpCalendarEventWriteTool("create"), mcpCalendarEventWriteTool("update"), mcpCalendarMoveTool(), mcpCalendarRespondTool()}
}

var mcpCalendarEventStringFlags = [][2]string{{"summary", "--summary"}, {"start", "--from"}, {"end", "--to"}, {"start_timezone", "--start-timezone"}, {"end_timezone", "--end-timezone"}, {"description", "--description"}, {"location", "--location"}, {"source_url", "--source-url"}, {"source_title", "--source-title"}, {"color", "--event-color"}, {"visibility", "--visibility"}, {"transparency", "--transparency"}}

func mcpCalendarEventWriteTool(action string) mcpToolSpec {
	tool := mcpToolSpec{Name: "calendar_" + action + "_event", Service: "calendar", Risk: mcpRiskWrite, Bounded: true, CommandPath: []string{"calendar", action}, Description: "Write an event through native Calendar validation. Notification default none; all/externalOnly require calendar_notify. Reminders may email the acting user. Returns a bounded receipt with retry_safe=false. Minimum output budget 4096 bytes."}
	tool.Options = []mcp.ToolOption{mcp.WithString("calendar_id", mcp.DefaultString("primary"))}
	for _, field := range mcpCalendarEventStringFlags {
		options := []mcp.PropertyOption{}
		if action == "create" && (field[0] == "summary" || field[0] == "start" || field[0] == "end") {
			options = append(options, mcp.Required())
		}
		tool.Options = append(tool.Options, mcp.WithString(field[0], options...))
	}
	tool.Options = append(tool.Options, mcp.WithString("timezone"), mcp.WithString("send_updates", mcp.DefaultString("none"), mcp.Enum("none", "all", "externalOnly")))
	for _, key := range []string{"all_day", "no_reminders", "with_meet", "guests_can_invite", "guests_can_modify", "guests_can_see_others"} {
		tool.Options = append(tool.Options, mcp.WithBoolean(key))
	}
	for _, field := range []struct {
		key   string
		limit int
	}{{"attendees", 200}, {"recurrence", 100}, {"reminders", 5}, {"attachment_urls", 100}} {
		tool.Options = append(tool.Options, mcp.WithArray(field.key, mcp.WithStringItems(), mcp.MaxItems(field.limit)))
	}
	if action == "update" {
		tool.Options = append(tool.Options, mcp.WithString("event_id", mcp.Required()), mcp.WithArray("add_attendees", mcp.WithStringItems(), mcp.MaxItems(200)), mcp.WithBoolean("regenerate_meet"), mcp.WithString("scope", mcp.Enum("all", "single", "future")), mcp.WithString("original_start"))
	}
	tool.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, tool); err != nil {
			return nil, err
		}
		return buildMCPCalendarEventArgs(req, action)
	}
	return tool
}

func buildMCPCalendarEventArgs(req mcp.CallToolRequest, action string) ([]string, error) {
	calendarID, err := mcpLiteralString(req, "calendar_id", "primary", true, 4096)
	if err != nil {
		return nil, err
	}
	args := []string{"calendar", action}
	positionals := []string{calendarID}
	if action == "update" {
		eventID, e := mcpLiteralString(req, "event_id", "", true, 4096)
		if e != nil {
			return nil, e
		}
		positionals = append(positionals, eventID)
	}
	if err = validateMCPCalendarEventInput(req, action); err != nil {
		return nil, err
	}
	args, err = mcpOptionalStrings(req, args, mcpCalendarEventStringFlags)
	if err != nil {
		return nil, err
	}
	if zone, present := req.GetArguments()["timezone"]; present {
		value, ok := zone.(string)
		if !ok {
			return nil, fmt.Errorf("timezone must be string")
		}
		if action == "create" {
			args = append(args, "--timezone="+value)
		} else {
			args = append(args, "--start-timezone="+value, "--end-timezone="+value)
		}
	}
	mode, err := mcpCalendarNotificationMode(req)
	if err != nil {
		return nil, err
	}
	args = append(args, "--send-updates="+mode)
	args, err = mcpOptionalBools(req, args, []string{"all_day", "guests_can_invite", "guests_can_modify", "guests_can_see_others"})
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"no_reminders", "with_meet", "regenerate_meet"} {
		if value, present := req.GetArguments()[key]; present {
			enabled, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("%s must be boolean", key)
			}
			if enabled {
				args = append(args, "--"+strings.ReplaceAll(key, "_", "-"))
			}
		}
	}
	for _, field := range []struct {
		key, flag, clear string
		limit            int
	}{{"attendees", "--attendee-entry=", "--attendees=", 200}, {"add_attendees", "--add-attendee-entry=", "", 200}, {"recurrence", "--rrule=", "--rrule=", 100}, {"reminders", "--reminder-entry=", "--reminder=", 5}, {"attachment_urls", "--attachment-url=", "--attachment=", 100}} {
		values, e := mcpLiteralStrings(req, field.key, field.limit)
		if e != nil {
			return nil, e
		}
		if _, present := req.GetArguments()[field.key]; present && len(values) == 0 && action == "update" && field.clear != "" {
			args = append(args, field.clear)
		}
		for _, value := range values {
			args = append(args, field.flag+value)
		}
	}
	if action == "update" {
		args, err = mcpCalendarScopeArgs(req, args)
		if err != nil {
			return nil, err
		}
	}
	return append(append(args, "--"), positionals...), nil
}

func validateMCPCalendarEventInput(req mcp.CallToolRequest, action string) error {
	if err := validateMCPCalendarEventTimes(req, action); err != nil {
		return err
	}
	if _, replace := req.GetArguments()["attendees"]; replace {
		if _, add := req.GetArguments()["add_attendees"]; add {
			return fmt.Errorf("attendee replacement and addition conflict")
		}
	}
	for _, key := range []string{"attendees", "add_attendees"} {
		values, err := mcpLiteralStrings(req, key, 200)
		if err != nil {
			return err
		}
		for _, value := range values {
			attendee := parseAttendee(value)
			if attendee == nil {
				return fmt.Errorf("invalid attendee")
			}
			address, e := mail.ParseAddress(attendee.Email)
			if e != nil || address.Address != attendee.Email {
				return fmt.Errorf("attendee must be a literal email address")
			}
		}
	}
	reminders, err := mcpLiteralStrings(req, "reminders", 5)
	if err != nil {
		return err
	}
	disabled, _ := req.GetArguments()["no_reminders"].(bool)
	if disabled && len(reminders) > 0 {
		return fmt.Errorf("reminders conflict with no_reminders")
	}
	if _, err = buildReminders(reminders, disabled); err != nil {
		return err
	}
	for _, field := range []struct {
		key    string
		values []string
	}{{"visibility", []string{"default", "public", "private", "confidential"}}, {"transparency", []string{"opaque", "transparent", "busy", "free"}}} {
		if value, present := req.GetArguments()[field.key]; present {
			text, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid enum")
			}
			found := false
			for _, allowed := range field.values {
				found = found || allowed == text
			}
			if !found {
				return fmt.Errorf("invalid enum")
			}
		}
	}
	if value, present := req.GetArguments()["color"]; present {
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("color must be string")
		}
		if text != "" {
			number, e := strconv.Atoi(text)
			if e != nil || number < 1 || number > 11 {
				return fmt.Errorf("invalid color")
			}
		}
	}
	urls, err := mcpLiteralStrings(req, "attachment_urls", 100)
	if err != nil {
		return err
	}
	if source, present := req.GetArguments()["source_url"]; present {
		text, ok := source.(string)
		if !ok {
			return fmt.Errorf("source_url must be string")
		}
		if text != "" {
			urls = append(urls, text)
		} else if title, _ := req.GetArguments()["source_title"].(string); title != "" {
			return fmt.Errorf("source title conflicts with clearing source")
		}
	}
	for _, value := range urls {
		parsed, e := url.Parse(value)
		if e != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return fmt.Errorf("invalid public URL")
		}
	}
	if req.GetBool("with_meet", false) && req.GetBool("regenerate_meet", false) {
		return fmt.Errorf("meet flags conflict")
	}
	return nil
}

func mcpCalendarNotificationMode(req mcp.CallToolRequest) (string, error) {
	mode, err := mcpLiteralString(req, "send_updates", "none", true, 64)
	if err != nil {
		return "", err
	}
	if mode != "none" && mode != "all" && mode != "externalOnly" {
		return "", fmt.Errorf("invalid send_updates")
	}
	return mode, nil
}

func mcpCalendarScopeArgs(req mcp.CallToolRequest, args []string) ([]string, error) {
	scope, err := mcpLiteralString(req, "scope", "all", true, 64)
	if err != nil {
		return nil, err
	}
	if scope != "all" && scope != "single" && scope != "future" {
		return nil, fmt.Errorf("invalid recurrence scope")
	}
	start, err := mcpLiteralString(req, "original_start", "", scope != "all", 4096)
	if err != nil {
		return nil, err
	}
	if start != "" {
		if _, _, err = originalStartRange(start); err != nil {
			return nil, fmt.Errorf("invalid original_start")
		}
		args = append(args, "--original-start="+start)
	}
	return append(args, "--scope="+scope), nil
}

func mcpCalendarMoveTool() mcpToolSpec {
	tool := mcpToolSpec{Name: "calendar_move_event", Service: "calendar", Risk: mcpRiskWrite, Bounded: true, CommandPath: []string{"calendar", "move"}, Description: "Move an event to a new organizer calendar. Notifications default none and require their own opt-in. Returns a non-retryable bounded receipt.", Options: []mcp.ToolOption{mcp.WithString("calendar_id", mcp.DefaultString("primary")), mcp.WithString("event_id", mcp.Required()), mcp.WithString("destination_calendar_id", mcp.Required()), mcp.WithString("send_updates", mcp.DefaultString("none"), mcp.Enum("none", "all", "externalOnly"))}}
	tool.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, tool); err != nil {
			return nil, err
		}
		mode, err := mcpCalendarNotificationMode(req)
		if err != nil {
			return nil, err
		}
		args := []string{"calendar", "move", "--send-updates=" + mode, "--"}
		for _, key := range []string{"calendar_id", "event_id", "destination_calendar_id"} {
			fallback := ""
			if key == "calendar_id" {
				fallback = "primary"
			}
			value, e := mcpLiteralString(req, key, fallback, true, 4096)
			if e != nil {
				return nil, e
			}
			args = append(args, value)
		}
		return args, nil
	}
	return tool
}

func mcpCalendarRespondTool() mcpToolSpec {
	tool := mcpToolSpec{Name: "calendar_respond", Service: "calendar", Risk: mcpRiskWrite, Capability: mcpCapabilityCalendarNotify, Bounded: true, CommandPath: []string{"calendar", "respond"}, Description: "Respond to an invitation. RSVP may notify the organizer and requires calendar_notify even with send_updates none. Returns a non-retryable bounded receipt.", Options: []mcp.ToolOption{mcp.WithString("calendar_id", mcp.DefaultString("primary")), mcp.WithString("event_id", mcp.Required()), mcp.WithString("status", mcp.Required(), mcp.Enum("accepted", "declined", "tentative", "needsAction")), mcp.WithString("comment"), mcp.WithString("send_updates", mcp.DefaultString("none"), mcp.Enum("none", "all", "externalOnly"))}}
	tool.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, tool); err != nil {
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
		status, err := mcpLiteralString(req, "status", "", true, 64)
		if err != nil {
			return nil, err
		}
		if status != "accepted" && status != "declined" && status != "tentative" && status != "needsAction" {
			return nil, fmt.Errorf("invalid RSVP status")
		}
		mode, err := mcpCalendarNotificationMode(req)
		if err != nil {
			return nil, err
		}
		args := make([]string, 0, 7)
		args = append(args, "calendar", "respond", "--status="+status, "--send-updates="+mode)
		args, err = mcpOptionalStrings(req, args, [][2]string{{"comment", "--comment"}})
		if err != nil {
			return nil, err
		}
		return append(args, "--", calendarID, eventID), nil
	}
	return tool
}

func validateMCPCalendarEventTimes(req mcp.CallToolRequest, action string) error {
	if action == "create" {
		for _, key := range []string{"summary", "start", "end"} {
			if _, err := mcpLiteralString(req, key, "", true, 64<<10); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"start", "end"} {
		if _, present := req.GetArguments()[key]; present {
			value, err := mcpLiteralString(req, key, "", true, 4096)
			if err != nil {
				return err
			}
			if _, err = time.Parse(time.RFC3339Nano, value); err != nil {
				if _, err = time.Parse(time.DateOnly, value); err != nil {
					return fmt.Errorf("invalid event time")
				}
			}
		}
	}
	if start, ok := req.GetArguments()["start"].(string); ok {
		if end, ok := req.GetArguments()["end"].(string); ok {
			layout := time.RFC3339Nano
			if len(start) == 10 {
				layout = time.DateOnly
			}
			from, e := time.Parse(layout, start)
			to, e2 := time.Parse(layout, end)
			if e != nil || e2 != nil || !to.After(from) {
				return fmt.Errorf("event end must follow start")
			}
		}
	}
	for _, key := range []string{"timezone", "start_timezone", "end_timezone"} {
		if value, present := req.GetArguments()[key]; present {
			zone, ok := value.(string)
			if !ok || zone == "" || zone == "Local" {
				return fmt.Errorf("invalid timezone")
			}
			if _, err := time.LoadLocation(zone); err != nil {
				return fmt.Errorf("invalid timezone")
			}
		}
	}
	if _, shared := req.GetArguments()["timezone"]; shared {
		if _, ok := req.GetArguments()["start_timezone"]; ok {
			return fmt.Errorf("timezone conflicts with start_timezone")
		}
		if _, ok := req.GetArguments()["end_timezone"]; ok {
			return fmt.Errorf("timezone conflicts with end_timezone")
		}
		if action == "update" {
			if _, ok := req.GetArguments()["start"]; !ok {
				return fmt.Errorf("shared timezone requires both event times")
			}
			if _, ok := req.GetArguments()["end"]; !ok {
				return fmt.Errorf("shared timezone requires both event times")
			}
		}
	}
	return nil
}
