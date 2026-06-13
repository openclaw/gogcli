package cmd

import (
	"context"
	"strings"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
)

func resolveClientOverride(flags *RootFlags) string {
	if flags == nil {
		return ""
	}
	return flags.Client
}

func resolveClientForEmail(ctx context.Context, email string, flags *RootFlags) (string, error) {
	override := resolveClientOverride(flags)
	return authclient.ResolveClientWithOverride(ctx, email, override)
}

func normalizeClientForFlag(raw string) (string, error) {
	return config.NormalizeClientNameOrDefault(raw)
}

func resolveClientForEmailWithContext(ctx context.Context, email string, cmdClient string) (string, error) {
	override := strings.TrimSpace(cmdClient)
	if override == "" {
		override = authclient.ClientOverrideFromContext(ctx)
	}
	return authclient.ResolveClientWithOverride(ctx, email, override)
}
