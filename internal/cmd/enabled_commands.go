package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
)

func enforceEnabledCommands(kctx *kong.Context, enabled string, enabledExact string) error {
	enabled = strings.TrimSpace(enabled)
	enabledExact = strings.TrimSpace(enabledExact)
	if enabled == "" && enabledExact == "" {
		return nil
	}

	allow := parseEnabledCommands(enabled)
	exactAllow := parseEnabledCommands(enabledExact)
	if len(allow) == 0 && len(exactAllow) == 0 {
		return nil
	}
	if allow["*"] || allow["all"] || exactAllow["*"] || exactAllow["all"] {
		return nil
	}

	path := commandPath(kctx.Command())
	if len(path) == 0 {
		return nil
	}
	if commandPathMatches(allow, path) || commandPathMatchesExact(exactAllow, path) {
		return nil
	}

	return usagef("command %q is not enabled (set --enable-commands or --enable-commands-exact to allow it)", strings.Join(path, " "))
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
	if len(path) == 2 {
		for _, alias := range commandPathAliases(strings.Join(path, ".")) {
			if rules[alias] {
				return true
			}
		}
	}
	return false
}

func commandPathMatchesExact(rules map[string]bool, path []string) bool {
	if rules["*"] || rules["all"] {
		return true
	}

	joined := strings.Join(path, ".")
	if rules[joined] {
		return true
	}

	if len(path) == 2 {
		for _, alias := range commandPathAliases(joined) {
			if rules[alias] {
				return true
			}
		}
	}

	return false
}

func commandPathAliases(path string) []string {
	switch path {
	case "docs.page-layout":
		return []string{"docs.set-page-layout", "docs.page-setup"}
	default:
		return nil
	}
}
