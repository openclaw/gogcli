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
	cmd := strings.Fields(kctx.Command())
	if len(cmd) == 0 {
		return nil
	}
	top := strings.ToLower(cmd[0])
	if !allow[top] {
		return usagef("command %q is not enabled (set --enable-commands to allow it)", top)
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
