package cmd

import (
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

func mcpCalendarReadTools() []mcpToolSpec {
	return []mcpToolSpec{mcpCalendarListCalendarsTool(), mcpCalendarGetEventTool(), mcpCalendarSearchEventsTool(), mcpCalendarFreeBusyTool()}
}

func mcpCalendarListCalendarsTool() mcpToolSpec {
	tool := mcpToolSpec{Name: "calendar_list_calendars", Service: "calendar", Risk: mcpRiskRead, Bounded: true, CommandPath: []string{"calendar", "calendars"}, Description: "List one page of visible calendars and its continuation token. Minimum output budget 4096 bytes.", Options: []mcp.ToolOption{mcp.WithInteger("max", mcp.DefaultNumber(100), mcp.Min(1), mcp.Max(250)), mcp.WithString("page")}}
	tool.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, tool); err != nil {
			return nil, err
		}
		maximum, err := mcpBoundedInt(req, "max", 100, 1, 250)
		if err != nil {
			return nil, err
		}
		args := []string{"calendar", "calendars", "--max=" + strconv.FormatInt(maximum, 10)}
		page, err := mcpLiteralString(req, "page", "", false, 4096)
		if err != nil {
			return nil, err
		}
		if page != "" {
			args = append(args, "--page="+page)
		}
		return args, nil
	}
	return tool
}

func mcpCalendarGetEventTool() mcpToolSpec {
	tool := mcpToolSpec{Name: "calendar_get_event", Service: "calendar", Risk: mcpRiskRead, Bounded: true, CommandPath: []string{"calendar", "event"}, Description: "Read one event with existing timezone and password redaction. Minimum output budget 4096 bytes.", Options: []mcp.ToolOption{mcp.WithString("calendar_id", mcp.DefaultString("primary")), mcp.WithString("event_id", mcp.Required()), mcp.WithString("timezone")}}
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
		args := []string{"calendar", "event"}
		zone, err := mcpLiteralString(req, "timezone", "", false, 4096)
		if err != nil {
			return nil, err
		}
		if zone != "" {
			if _, err = displayTimezoneOverride(zone); err != nil {
				return nil, err
			}
			args = append(args, "--timezone="+zone)
		}
		return append(args, "--", calendarID, eventID), nil
	}
	return tool
}

func mcpCalendarSearchEventsTool() mcpToolSpec {
	tool := mcpToolSpec{Name: "calendar_search_events", Service: "calendar", Risk: mcpRiskRead, Bounded: true, CommandPath: []string{"calendar", "search"}, Description: "Search a complete provider page, default window -30/+90 days. Follow nextPageToken. Minimum output budget 4096 bytes.", Options: []mcp.ToolOption{mcp.WithString("query", mcp.Required()), mcp.WithString("calendar_id", mcp.DefaultString("primary")), mcp.WithString("from"), mcp.WithString("to"), mcp.WithInteger("max", mcp.DefaultNumber(25), mcp.Min(1), mcp.Max(100)), mcp.WithString("page")}}
	tool.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, tool); err != nil {
			return nil, err
		}
		if err := mcpCheckWindow(req); err != nil {
			return nil, err
		}
		query, err := mcpQuery(req, "query")
		if err != nil {
			return nil, err
		}
		calendarID, err := mcpLiteralString(req, "calendar_id", "primary", true, 4096)
		if err != nil {
			return nil, err
		}
		maximum, err := mcpBoundedInt(req, "max", 25, 1, 100)
		if err != nil {
			return nil, err
		}
		args := []string{"calendar", "search", "--calendar=" + calendarID, "--max=" + strconv.FormatInt(maximum, 10)}
		page, err := mcpLiteralString(req, "page", "", false, 4096)
		if err != nil {
			return nil, err
		}
		if page != "" {
			args = append(args, "--page="+page)
		}
		args, err = mcpOptionalStrings(req, args, [][2]string{{"from", "--from"}, {"to", "--to"}})
		if err != nil {
			return nil, err
		}
		return append(args, "--", query), nil
	}
	return tool
}

func mcpCalendarFreeBusyTool() mcpToolSpec {
	tool := mcpToolSpec{Name: "calendar_freebusy", Service: "calendar", Risk: mcpRiskRead, Bounded: true, CommandPath: []string{"calendar", "freebusy"}, Description: "Read busy intervals and per-calendar errors for at most 50 literal calendars. No invitations are sent. Minimum output budget 4096 bytes.", Options: []mcp.ToolOption{mcp.WithString("from", mcp.Required()), mcp.WithString("to", mcp.Required()), mcp.WithArray("calendars", mcp.WithStringItems(), mcp.MaxItems(50))}}
	tool.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, tool); err != nil {
			return nil, err
		}
		if err := mcpCheckWindow(req); err != nil {
			return nil, err
		}
		from, err := mcpLiteralString(req, "from", "", true, 4096)
		if err != nil {
			return nil, err
		}
		to, err := mcpLiteralString(req, "to", "", true, 4096)
		if err != nil {
			return nil, err
		}
		args := []string{"calendar", "freebusy", "--from=" + from, "--to=" + to}
		calendars, err := mcpLiteralStrings(req, "calendars", 50)
		if err != nil {
			return nil, err
		}
		for _, calendarID := range calendars {
			args = append(args, "--calendar-id="+calendarID)
		}
		return args, nil
	}
	return tool
}
