package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func mcpAllTools() []mcpToolSpec {
	return []mcpToolSpec{
		mcpGmailSearchTool(),
		mcpGmailGetMessageTool(),
		mcpGmailGetThreadTool(),
		mcpDriveSearchTool(),
		mcpDriveGetTool(),
		mcpDocsGetTool(),
		mcpSheetsReadRangeTool(),
		mcpCalendarEventsTool(),
		mcpDocsWriteTool(),
		mcpSheetsUpdateRangeTool(),
	}
}

func mcpGmailSearchTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "gmail_search",
		Service:     "gmail",
		Risk:        mcpRiskRead,
		Description: "Search Gmail messages with Gmail query syntax. Returns message summaries and optional sanitized bodies.",
		Options: []mcp.ToolOption{
			mcp.WithString("query", mcp.Description("Gmail search query, e.g. newer_than:7d from:person@example.com"), mcp.Required()),
			mcp.WithInteger("max", mcp.Description("Maximum results"), mcp.DefaultNumber(10), mcp.Min(1), mcp.Max(100)),
			mcp.WithBoolean("include_body", mcp.Description("Include decoded message body"), mcp.DefaultBool(false)),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			query, err := requireMCPString(req, "query")
			if err != nil {
				return nil, err
			}
			args := []string{"gmail", "messages", "search", "--max", strconv.Itoa(clampMCPInt(req.GetInt("max", 10), 1, 100))}
			if req.GetBool("include_body", false) {
				args = append(args, "--include-body")
			}
			return append(args, "--", query), nil
		},
	}
}

func mcpGmailGetMessageTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "gmail_get_message",
		Service:     "gmail",
		Risk:        mcpRiskRead,
		Description: "Get one Gmail message by ID. Sanitized content is enabled by default.",
		Options: []mcp.ToolOption{
			mcp.WithString("message_id", mcp.Description("Gmail message ID"), mcp.Required()),
			mcp.WithBoolean("sanitize_content", mcp.Description("Strip URLs/HTML and omit raw payloads"), mcp.DefaultBool(true)),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			messageID, err := requireMCPString(req, "message_id")
			if err != nil {
				return nil, err
			}
			args := []string{"gmail", "get"}
			if req.GetBool("sanitize_content", true) {
				args = append(args, "--sanitize-content")
			}
			return append(args, "--", messageID), nil
		},
	}
}

func mcpGmailGetThreadTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "gmail_get_thread",
		Service:     "gmail",
		Risk:        mcpRiskRead,
		Description: "Get one Gmail thread by ID. Sanitized content is enabled by default.",
		Options: []mcp.ToolOption{
			mcp.WithString("thread_id", mcp.Description("Gmail thread ID"), mcp.Required()),
			mcp.WithBoolean("sanitize_content", mcp.Description("Strip URLs/HTML and omit raw payloads"), mcp.DefaultBool(true)),
			mcp.WithBoolean("full", mcp.Description("Include full message bodies"), mcp.DefaultBool(false)),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			threadID, err := requireMCPString(req, "thread_id")
			if err != nil {
				return nil, err
			}
			args := []string{"gmail", "thread", "get"}
			if req.GetBool("sanitize_content", true) {
				args = append(args, "--sanitize-content")
			}
			if req.GetBool("full", false) {
				args = append(args, "--full")
			}
			return append(args, "--", threadID), nil
		},
	}
}

func mcpDriveSearchTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "drive_search",
		Service:     "drive",
		Risk:        mcpRiskRead,
		Description: "Search Google Drive files using text search or Drive query language.",
		Options: []mcp.ToolOption{
			mcp.WithString("query", mcp.Description("Search text or Drive query"), mcp.Required()),
			mcp.WithInteger("max", mcp.Description("Maximum results"), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
			mcp.WithBoolean("raw_query", mcp.Description("Treat query as Drive query language"), mcp.DefaultBool(false)),
			mcp.WithString("parent", mcp.Description("Optional parent folder/shared drive ID")),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			query, err := requireMCPString(req, "query")
			if err != nil {
				return nil, err
			}
			args := []string{"drive", "search", "--max", strconv.Itoa(clampMCPInt(req.GetInt("max", 20), 1, 100))}
			if req.GetBool("raw_query", false) {
				args = append(args, "--raw-query")
			}
			if parent := strings.TrimSpace(req.GetString("parent", "")); parent != "" {
				args = append(args, "--parent", parent)
			}
			return append(args, "--", query), nil
		},
	}
}

func mcpDriveGetTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "drive_get",
		Service:     "drive",
		Risk:        mcpRiskRead,
		Description: "Get Google Drive file metadata by ID.",
		Options: []mcp.ToolOption{
			mcp.WithString("file_id", mcp.Description("Drive file ID"), mcp.Required()),
			mcp.WithString("fields", mcp.Description("Optional Drive API field mask")),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			fileID, err := requireMCPString(req, "file_id")
			if err != nil {
				return nil, err
			}
			args := []string{"drive", "get"}
			if fields := strings.TrimSpace(req.GetString("fields", "")); fields != "" {
				args = append(args, "--fields", fields)
			}
			return append(args, "--", fileID), nil
		},
	}
}

func mcpDocsGetTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "docs_get",
		Service:     "docs",
		Risk:        mcpRiskRead,
		Description: "Read a Google Doc as wrapped text, all tabs, or one tab.",
		Options: []mcp.ToolOption{
			mcp.WithString("document_id", mcp.Description("Google Docs document ID"), mcp.Required()),
			mcp.WithString("tab", mcp.Description("Optional tab title or ID")),
			mcp.WithBoolean("all_tabs", mcp.Description("Read all tabs"), mcp.DefaultBool(false)),
			mcp.WithInteger("max_bytes", mcp.Description("Maximum text bytes, 0 for unlimited"), mcp.DefaultNumber(2000000), mcp.Min(0)),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			docID, err := requireMCPString(req, "document_id")
			if err != nil {
				return nil, err
			}
			args := []string{"docs", "cat", "--max-bytes", strconv.Itoa(clampMCPInt(req.GetInt("max_bytes", 2000000), 0, 20_000_000))}
			if tab := strings.TrimSpace(req.GetString("tab", "")); tab != "" {
				args = append(args, "--tab", tab)
			}
			if req.GetBool("all_tabs", false) {
				args = append(args, "--all-tabs")
			}
			return append(args, "--", docID), nil
		},
	}
}

func mcpSheetsReadRangeTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "sheets_read_range",
		Service:     "sheets",
		Risk:        mcpRiskRead,
		Description: "Read values from a Google Sheets range.",
		Options: []mcp.ToolOption{
			mcp.WithString("spreadsheet_id", mcp.Description("Google Sheets spreadsheet ID"), mcp.Required()),
			mcp.WithString("range", mcp.Description("A1 notation or named range"), mcp.Required()),
			mcp.WithString("render", mcp.Description("Value render option"), mcp.Enum("FORMATTED_VALUE", "UNFORMATTED_VALUE", "FORMULA")),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			spreadsheetID, err := requireMCPString(req, "spreadsheet_id")
			if err != nil {
				return nil, err
			}
			rangeSpec, err := requireMCPString(req, "range")
			if err != nil {
				return nil, err
			}
			args := []string{"sheets", "get"}
			if render := strings.TrimSpace(req.GetString("render", "")); render != "" {
				args = append(args, "--render", render)
			}
			return append(args, "--", spreadsheetID, rangeSpec), nil
		},
	}
}

func mcpCalendarEventsTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "calendar_events",
		Service:     "calendar",
		Risk:        mcpRiskRead,
		Description: "List Google Calendar events from primary or selected calendars.",
		Options: []mcp.ToolOption{
			mcp.WithString("calendar_id", mcp.Description("Calendar ID or selector; default primary")),
			mcp.WithString("from", mcp.Description("Start time: RFC3339, date, or relative value")),
			mcp.WithString("to", mcp.Description("End time: RFC3339, date, or relative value")),
			mcp.WithBoolean("today", mcp.Description("Today only"), mcp.DefaultBool(false)),
			mcp.WithBoolean("tomorrow", mcp.Description("Tomorrow only"), mcp.DefaultBool(false)),
			mcp.WithInteger("days", mcp.Description("Next N days"), mcp.DefaultNumber(0), mcp.Min(0), mcp.Max(31)),
			mcp.WithInteger("max", mcp.Description("Maximum results"), mcp.DefaultNumber(10), mcp.Min(1), mcp.Max(250)),
			mcp.WithString("query", mcp.Description("Free text search")),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			args := []string{"calendar", "events"}
			calendarID := strings.TrimSpace(req.GetString("calendar_id", ""))
			for _, pair := range [][2]string{{"from", "--from"}, {"to", "--to"}, {"query", "--query"}} {
				if v := strings.TrimSpace(req.GetString(pair[0], "")); v != "" {
					args = append(args, pair[1], v)
				}
			}
			if req.GetBool("today", false) {
				args = append(args, "--today")
			}
			if req.GetBool("tomorrow", false) {
				args = append(args, "--tomorrow")
			}
			if days := req.GetInt("days", 0); days > 0 {
				args = append(args, "--days", strconv.Itoa(clampMCPInt(days, 1, 31)))
			}
			args = append(args, "--max", strconv.Itoa(clampMCPInt(req.GetInt("max", 10), 1, 250)))
			if calendarID != "" {
				args = append(args, "--", calendarID)
			}
			return args, nil
		},
	}
}

func mcpDocsWriteTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "docs_write",
		Service:     "docs",
		Risk:        mcpRiskWrite,
		Description: "Write text to a Google Doc. Requires --allow-write on the MCP server.",
		Options: []mcp.ToolOption{
			mcp.WithString("document_id", mcp.Description("Google Docs document ID"), mcp.Required()),
			mcp.WithString("text", mcp.Description("Text or markdown to write"), mcp.Required()),
			mcp.WithString("tab", mcp.Description("Optional tab title or ID")),
			mcp.WithBoolean("append", mcp.Description("Append instead of replacing"), mcp.DefaultBool(true)),
			mcp.WithBoolean("replace", mcp.Description("Replace all existing content"), mcp.DefaultBool(false)),
			mcp.WithBoolean("markdown", mcp.Description("Convert markdown to Docs formatting"), mcp.DefaultBool(false)),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			docID, err := requireMCPString(req, "document_id")
			if err != nil {
				return nil, err
			}
			text, err := requireMCPText(req, "text")
			if err != nil {
				return nil, err
			}
			args := []string{"docs", "write", "--text", text}
			reqArgs := req.GetArguments()
			replace := req.GetBool("replace", false)
			appendProvided := false
			if reqArgs != nil {
				_, appendProvided = reqArgs["append"]
			}
			appendMode := req.GetBool("append", true)
			if replace && appendProvided && appendMode {
				return nil, fmt.Errorf("append and replace are mutually exclusive")
			}
			switch {
			case replace:
				args = append(args, "--replace")
			case appendMode:
				args = append(args, "--append")
			default:
				return nil, fmt.Errorf("append=false requires replace=true to avoid implicit document replacement")
			}
			if req.GetBool("markdown", false) {
				args = append(args, "--markdown")
			}
			if tab := strings.TrimSpace(req.GetString("tab", "")); tab != "" {
				args = append(args, "--tab", tab)
			}
			return append(args, "--", docID), nil
		},
	}
}

func mcpSheetsUpdateRangeTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "sheets_update_range",
		Service:     "sheets",
		Risk:        mcpRiskWrite,
		Description: "Update values in a Google Sheets range. Requires --allow-write on the MCP server.",
		Options: []mcp.ToolOption{
			mcp.WithString("spreadsheet_id", mcp.Description("Google Sheets spreadsheet ID"), mcp.Required()),
			mcp.WithString("range", mcp.Description("A1 notation or named range"), mcp.Required()),
			mcp.WithString("values_json", mcp.Description("JSON 2D array of values"), mcp.Required()),
			mcp.WithString("input", mcp.Description("Value input option"), mcp.Enum("RAW", "USER_ENTERED"), mcp.DefaultString("USER_ENTERED")),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			spreadsheetID, err := requireMCPString(req, "spreadsheet_id")
			if err != nil {
				return nil, err
			}
			rangeSpec, err := requireMCPString(req, "range")
			if err != nil {
				return nil, err
			}
			valuesJSON, err := requireMCPLiteralValuesJSON(req, "values_json")
			if err != nil {
				return nil, err
			}
			input := strings.TrimSpace(req.GetString("input", "USER_ENTERED"))
			if input == "" {
				input = "USER_ENTERED"
			}
			return []string{"sheets", "update", "--values-json", valuesJSON, "--input", input, "--", spreadsheetID, rangeSpec}, nil
		},
	}
}

func requireMCPText(req mcp.CallToolRequest, key string) (string, error) {
	value, err := req.RequireString(key)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("empty %s", key)
	}
	return value, nil
}

func requireMCPLiteralValuesJSON(req mcp.CallToolRequest, key string) (string, error) {
	value, err := requireMCPText(req, key)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "-" || strings.HasPrefix(trimmed, "@") {
		return "", fmt.Errorf("%s must be literal JSON, not stdin or @file input", key)
	}
	var rows [][]any
	dec := json.NewDecoder(bytes.NewReader([]byte(trimmed)))
	dec.UseNumber()
	if unmarshalErr := dec.Decode(&rows); unmarshalErr != nil {
		return "", fmt.Errorf("invalid %s JSON 2D array: %w", key, unmarshalErr)
	}
	var extra any
	if extraErr := dec.Decode(&extra); extraErr != io.EOF {
		return "", fmt.Errorf("invalid %s JSON 2D array: trailing content", key)
	}
	canonical, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("canonicalize %s: %w", key, err)
	}
	return string(canonical), nil
}
