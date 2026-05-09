package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
)

func enforceEnabledCommands(kctx *kong.Context, enabled string) error {
	enabled = strings.TrimSpace(enabled)
	if enabled == "" {
		return nil
	}
	allow := parseEnabledCommands(enabled)
	if len(allow) == 0 {
		return nil
	}
	if allow["*"] || allow["all"] {
		return nil
	}
	path := commandPath(kctx.Command())
	if len(path) == 0 {
		return nil
	}
	if !commandPathMatches(allow, path) {
		return usagef("command %q is not enabled (set --enable-commands to allow it)", strings.Join(path, " "))
	}
	return nil
}

func enforceDisabledCommands(kctx *kong.Context, disabled string) error {
	disabled = strings.TrimSpace(disabled)
	if disabled == "" {
		return nil
	}
	deny := parseEnabledCommands(disabled)
	if len(deny) == 0 {
		return nil
	}
	path := commandPath(kctx.Command())
	if len(path) == 0 {
		return nil
	}
	if commandPathMatches(deny, path) {
		return usagef("command %q is disabled (blocked by --disable-commands)", strings.Join(path, " "))
	}
	return nil
}

func parseEnabledCommands(value string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		out[part] = true
	}
	return out
}

func commandPath(command string) []string {
	fields := strings.Fields(command)
	path := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.HasPrefix(field, "<") {
			break
		}
		path = append(path, strings.ToLower(field))
	}
	return path
}

func commandPathMatches(rules map[string]bool, path []string) bool {
	if rules["*"] || rules["all"] {
		return true
	}
	for i := range path {
		if rules[strings.Join(path[:i+1], ".")] {
			return true
		}
	}
	return false
}
