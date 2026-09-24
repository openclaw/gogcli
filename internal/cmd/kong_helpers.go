package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
)

func validateExplicitBatchFlag(kctx *kong.Context) error {
	if !flagOnCommandLine(kctx, "batch") {
		return nil
	}
	for _, flag := range kctx.Flags() {
		if flag.Name == "batch" && strings.TrimSpace(flag.Target.String()) == "" {
			return usage("--batch requires a non-empty batch ID; omit --batch to submit immediately")
		}
	}
	return nil
}

// flagProvided reports whether a command should treat the flag as supplied. A value
// a baked profile locked counts: commands that build partial requests from which
// flags were given would otherwise drop a locked value on the floor, leaving the
// lock set on the struct but absent from the request.
func flagProvided(kctx *kong.Context, name string) bool {
	return flagOnCommandLine(kctx, name) || lockedFlagStateFor(kctx).has(name)
}

// flagOnCommandLine reports only what the caller typed. Lock enforcement and
// output-mode precedence need that narrower question: one to detect an override
// attempt, the other to rank an explicit flag against an environment default.
func flagOnCommandLine(kctx *kong.Context, name string) bool {
	if kctx == nil {
		return false
	}
	for _, trace := range kctx.Path {
		if trace.Flag != nil && trace.Flag.Name == name {
			return true
		}
	}
	return false
}
