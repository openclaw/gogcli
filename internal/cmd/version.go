package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
)

//go:embed VERSION
var embeddedVersion string

const sentinelDev = "dev"

var (
	version       = sentinelDev
	commit        = ""
	date          = ""
	readBuildInfo = debug.ReadBuildInfo
)

func resolvedVersion() string {
	v := strings.TrimSpace(version)
	if v != "" && v != sentinelDev {
		return v
	}
	info, ok := readBuildInfo()
	if ok {
		moduleVersion := strings.TrimSpace(info.Main.Version)
		if moduleVersion != "" && moduleVersion != "(devel)" {
			return moduleVersion
		}
	}
	if baked := strings.TrimSpace(embeddedVersion); baked != "" {
		return baked
	}
	return sentinelDev
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
