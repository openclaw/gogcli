package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
)

var (
	version = "0.13.0-dev"
	commit  = ""
	date    = ""
	// fork identifies the downstream fork this binary was built from, if any.
	// Populated via ldflags at build time; empty for upstream builds.
	fork = ""
)

func VersionString() string {
	v := strings.TrimSpace(version)
	if v == "" {
		v = "dev"
	}
	base := v
	if strings.TrimSpace(commit) != "" && strings.TrimSpace(date) != "" {
		base = fmt.Sprintf("%s (%s %s)", v, strings.TrimSpace(commit), strings.TrimSpace(date))
	} else if strings.TrimSpace(commit) != "" {
		base = fmt.Sprintf("%s (%s)", v, strings.TrimSpace(commit))
	} else if strings.TrimSpace(date) != "" {
		base = fmt.Sprintf("%s (%s)", v, strings.TrimSpace(date))
	}
	if f := strings.TrimSpace(fork); f != "" {
		return fmt.Sprintf("%s [fork: %s]", base, f)
	}
	return base
}

type VersionCmd struct{}

func (c *VersionCmd) Run(ctx context.Context) error {
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"version": strings.TrimSpace(version),
			"commit":  strings.TrimSpace(commit),
			"date":    strings.TrimSpace(date),
		}
		if f := strings.TrimSpace(fork); f != "" {
			payload["fork"] = f
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}
	fmt.Fprintln(os.Stdout, VersionString())
	return nil
}
