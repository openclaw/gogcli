package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
)

const devVersion = "dev"

var (
	version       = devVersion
	commit        = ""
	date          = ""
	readBuildInfo = debug.ReadBuildInfo
)

func resolvedVersion() string {
	v := strings.TrimSpace(version)
	if v != "" && v != devVersion && !strings.HasSuffix(v, "-dev") {
		return v
	}
	info, ok := readBuildInfo()
	if ok {
		moduleVersion := strings.TrimSpace(info.Main.Version)
		if moduleVersion != "" && moduleVersion != "(devel)" {
			return moduleVersion
		}
	}
	if v == "" {
		return devVersion
	}
	return v
}

func VersionString() string {
	v := resolvedVersion()
	if strings.TrimSpace(commit) == "" && strings.TrimSpace(date) == "" {
		return v
	}
	if strings.TrimSpace(commit) == "" {
		return fmt.Sprintf("%s (%s)", v, strings.TrimSpace(date))
	}
	if strings.TrimSpace(date) == "" {
		return fmt.Sprintf("%s (%s)", v, strings.TrimSpace(commit))
	}
	return fmt.Sprintf("%s (%s %s)", v, strings.TrimSpace(commit), strings.TrimSpace(date))
}

type VersionCmd struct{}

func (c *VersionCmd) Run(ctx context.Context) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"version": resolvedVersion(),
			"commit":  strings.TrimSpace(commit),
			"date":    strings.TrimSpace(date),
		})
	}
	fmt.Fprintln(os.Stdout, VersionString())
	return nil
}
