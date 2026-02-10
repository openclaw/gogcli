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
	// Build the command path: only real command words (skip <arg> placeholders).
	var cmdPath []string
	for _, part := range cmd {
		if strings.HasPrefix(part, "<") {
			break
		}
		cmdPath = append(cmdPath, strings.ToLower(part))
	}
	if len(cmdPath) == 0 {
		return nil
	}
	if matchEnabledCommand(allow, cmdPath) {
		return nil
	}
	label := strings.Join(cmdPath, " ")
	return usagef("command %q is not enabled (set --enable-commands to allow it)", label)
}

// matchEnabledCommand checks whether cmdPath (e.g. ["gmail","drafts","create"])
// is permitted by any entry in the allow map.
//
// An allowed entry matches if the command path starts with (or equals) that entry.
//   - "gmail"               → matches any gmail subcommand
//   - "gmail.drafts"        → matches gmail drafts and any deeper subcommand
//   - "gmail.drafts.create" → matches only gmail drafts create
func matchEnabledCommand(allow map[string]bool, cmdPath []string) bool {
	// Check every prefix of cmdPath: if any prefix is allowed, the command is ok.
	// e.g. for cmdPath ["gmail","drafts","create"], check:
	//   "gmail", "gmail.drafts", "gmail.drafts.create"
	built := ""
	for i, seg := range cmdPath {
		if i == 0 {
			built = seg
		} else {
			built += "." + seg
		}
		if allow[built] {
			return true
		}
	}
	return false
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
