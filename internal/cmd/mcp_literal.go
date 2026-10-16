package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
)

func mcpLiteralString(req mcp.CallToolRequest, key string, fallback string, required bool, limit int) (string, error) {
	value := fallback
	if _, present := req.GetArguments()[key]; present {
		var err error
		value, err = req.RequireString(key)
		if err != nil {
			return "", err
		}
	}
	if (required && strings.TrimSpace(value) == "") || len(value) > limit || strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("invalid %s", key)
	}
	return value, nil
}

func mcpLiteralStrings(req mcp.CallToolRequest, key string, maximum int) ([]string, error) {
	if _, present := req.GetArguments()[key]; !present {
		return nil, nil
	}
	values, err := req.RequireStringSlice(key)
	if err != nil {
		return nil, err
	}
	if len(values) > maximum {
		return nil, fmt.Errorf("too many %s", key)
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || len(value) > 4096 || strings.ContainsRune(value, 0) {
			return nil, fmt.Errorf("invalid %s entry", key)
		}
	}
	return values, nil
}

func mcpOptionalStrings(req mcp.CallToolRequest, args []string, fields [][2]string) ([]string, error) {
	for _, field := range fields {
		if _, ok := req.GetArguments()[field[0]]; ok {
			value, err := mcpLiteralString(req, field[0], "", false, 64<<10)
			if err != nil {
				return nil, err
			}
			args = append(args, field[1]+"="+value)
		}
	}
	return args, nil
}

func mcpCheckWindow(req mcp.CallToolRequest) error {
	var from, to time.Time
	for _, field := range []string{"from", "to"} {
		if _, ok := req.GetArguments()[field]; !ok {
			continue
		}
		value, err := mcpLiteralString(req, field, "", true, 4096)
		if err != nil {
			return err
		}
		parsed, err := parseTimeExpr(value, time.Now().UTC(), time.UTC)
		if err != nil {
			return fmt.Errorf("invalid %s", field)
		}
		if field == "from" {
			from = parsed
		} else {
			to = parsed
		}
	}
	if !from.IsZero() && !to.IsZero() && !to.After(from) {
		return fmt.Errorf("window end must follow start")
	}
	return nil
}

func mcpQuery(req mcp.CallToolRequest, key string) (string, error) {
	value, err := mcpLiteralString(req, key, "", true, 64<<10)
	if err != nil {
		return "", err
	}
	if utf8.RuneCountInString(value) > 4000 {
		return "", fmt.Errorf("%s exceeds 4000 characters", key)
	}
	return value, nil
}

func mcpCheckKnownArguments(req mcp.CallToolRequest, tool mcpToolSpec) error {
	schema := newMCPTool(tool).InputSchema
	for key := range req.GetArguments() {
		if _, ok := schema.Properties[key]; !ok {
			return fmt.Errorf("unknown field %s", key)
		}
	}
	return nil
}

func mcpOptionalBools(req mcp.CallToolRequest, args []string, keys []string) ([]string, error) {
	for _, key := range keys {
		if value, present := req.GetArguments()[key]; present {
			enabled, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("%s must be boolean", key)
			}
			args = append(args, "--"+strings.ReplaceAll(key, "_", "-")+"="+strconv.FormatBool(enabled))
		}
	}
	return args, nil
}
