package cmd

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
)

func mcpGmailSettingsTools() []mcpToolSpec {
	rename := mcpToolSpec{Name: "gmail_rename_label", Service: "gmail", Risk: mcpRiskWrite, Bounded: true, CommandPath: []string{"gmail", "labels", "rename"}, Description: "Rename a user Gmail label using native case-sensitive ID lookup. System labels cannot be renamed. Minimum output budget 4096 bytes.", Options: []mcp.ToolOption{mcp.WithString("label", mcp.Required()), mcp.WithString("new_name", mcp.Required())}}
	rename.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, rename); err != nil {
			return nil, err
		}
		label, err := mcpLiteralString(req, "label", "", true, 4096)
		if err != nil {
			return nil, err
		}
		name, err := mcpLiteralString(req, "new_name", "", true, 64<<10)
		if err != nil || utf8.RuneCountInString(name) > 4000 {
			return nil, fmt.Errorf("invalid label name")
		}
		return []string{"gmail", "labels", "rename", "--", label, name}, nil
	}
	list := mcpToolSpec{Name: "gmail_list_filters", Service: "gmail", Risk: mcpRiskRead, Bounded: true, CommandPath: []string{"gmail", "filters", "list"}, Description: "Read Gmail filters. Oversized nonpaged results fail explicitly. Minimum output budget 4096 bytes."}
	list.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, list); err != nil {
			return nil, err
		}
		return []string{"gmail", "filters", "list"}, nil
	}
	return []mcpToolSpec{rename, list, mcpGmailCreateFilterTool()}
}

func mcpGmailCreateFilterTool() mcpToolSpec {
	tool := mcpToolSpec{Name: "gmail_create_filter", Service: "gmail", Risk: mcpRiskWrite, Bounded: true, CommandPath: []string{"gmail", "filters", "create"}, Description: "Create a future-affecting Gmail filter from literal criteria and actions. Trash and label removal affect future matching mail. Forwarding is excluded. Minimum output budget 4096 bytes."}
	stringsKeys := []string{"from", "to", "subject", "query"}
	boolKeys := []string{"has_attachment", "archive", "mark_read", "star", "trash", "never_spam", "important"}
	for _, key := range stringsKeys {
		tool.Options = append(tool.Options, mcp.WithString(key))
	}
	for _, key := range boolKeys {
		tool.Options = append(tool.Options, mcp.WithBoolean(key))
	}
	for _, key := range []string{"add_labels", "remove_labels"} {
		tool.Options = append(tool.Options, mcp.WithArray(key, mcp.WithStringItems(), mcp.MaxItems(100)))
	}
	tool.BuildArgs = func(req mcp.CallToolRequest) ([]string, error) {
		if err := mcpCheckKnownArguments(req, tool); err != nil {
			return nil, err
		}
		args := []string{"gmail", "filters", "create"}
		criteria, action := false, false
		for _, key := range stringsKeys {
			if _, present := req.GetArguments()[key]; present {
				value, err := mcpLiteralString(req, key, "", false, 64<<10)
				if err != nil || utf8.RuneCountInString(value) > 4000 {
					return nil, fmt.Errorf("invalid filter criteria")
				}
				criteria = criteria || strings.TrimSpace(value) != ""
				args = append(args, "--"+key+"="+value)
			}
		}
		var err error
		args, err = mcpOptionalBools(req, args, boolKeys)
		if err != nil {
			return nil, err
		}
		for _, key := range boolKeys {
			if enabled, _ := req.GetArguments()[key].(bool); enabled {
				if key == "has_attachment" {
					criteria = true
				} else {
					action = true
				}
			}
		}
		for _, key := range []string{"add_labels", "remove_labels"} {
			labels, e := mcpLiteralStrings(req, key, 100)
			if e != nil {
				return nil, e
			}
			action = action || len(labels) > 0
			flag := "--" + strings.ReplaceAll(strings.TrimSuffix(key, "s"), "_", "-") + "-entry="
			for _, label := range labels {
				args = append(args, flag+label)
			}
		}
		if !criteria || !action {
			return nil, fmt.Errorf("filter requires at least one criterion and action")
		}
		return args, nil
	}
	return tool
}
