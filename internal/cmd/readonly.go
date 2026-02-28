package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
)

var readOnlyBlockedTokens = map[string]struct{}{
	"accept":    {},
	"add":       {},
	"append":    {},
	"archive":   {},
	"clear":     {},
	"copy":      {},
	"create":    {},
	"decline":   {},
	"del":       {},
	"delete":    {},
	"done":      {},
	"edit":      {},
	"format":    {},
	"grade":     {},
	"insert":    {},
	"join":      {},
	"leave":     {},
	"login":     {},
	"logout":    {},
	"mark":      {},
	"modify":    {},
	"move":      {},
	"patch":     {},
	"propose":   {},
	"remove":    {},
	"renew":     {},
	"replace":   {},
	"respond":   {},
	"restore":   {},
	"rm":        {},
	"run":       {},
	"sed":       {},
	"send":      {},
	"set":       {},
	"setup":     {},
	"start":     {},
	"stop":      {},
	"trash":     {},
	"undo":      {},
	"unarchive": {},
	"unset":     {},
	"untrash":   {},
	"update":    {},
	"upload":    {},
	"verify":    {},
	"watch":     {},
	"write":     {},
}

var readOnlyBlockedPaths = map[string]struct{}{
	"calendar focus-time":       {},
	"calendar out-of-office":    {},
	"calendar propose-time":     {},
	"calendar working-location": {},
	"chat dm space":             {},
}

func enforceReadOnlyCommand(kctx *kong.Context, enabled bool) error {
	if !enabled || kctx == nil {
		return nil
	}

	tokens := selectedCommandTokens(kctx)
	if len(tokens) == 0 {
		return nil
	}

	commandPath := strings.Join(tokens, " ")
	if _, blocked := readOnlyBlockedPaths[commandPath]; blocked {
		return usagef("read-only mode blocks mutating command: %s (re-run without --read-only to allow writes)", commandPath)
	}

	for _, token := range tokens {
		if _, blocked := readOnlyBlockedTokens[token]; blocked {
			return usagef("read-only mode blocks mutating command: %s (re-run without --read-only to allow writes)", commandPath)
		}
		for _, part := range strings.Split(token, "-") {
			if _, blocked := readOnlyBlockedTokens[part]; blocked {
				return usagef("read-only mode blocks mutating command: %s (re-run without --read-only to allow writes)", commandPath)
			}
		}
	}

	return nil
}

func selectedCommandTokens(kctx *kong.Context) []string {
	if kctx == nil {
		return nil
	}

	tokens := make([]string, 0, len(kctx.Path))
	for _, p := range kctx.Path {
		if p == nil || p.Command == nil {
			continue
		}
		name := strings.TrimSpace(strings.ToLower(p.Command.Name))
		if name == "" || name == "gog" {
			continue
		}
		tokens = append(tokens, name)
	}

	return tokens
}
