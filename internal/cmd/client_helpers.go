package cmd

import (
	"context"
	"strings"

	"github.com/degree-analytics/ratatosk/internal/authclient"
	"github.com/degree-analytics/ratatosk/internal/config"
)

func resolveClientOverride(flags *RootFlags, cmdClient string) string {
	if strings.TrimSpace(cmdClient) != "" {
		return cmdClient
	}
	if flags == nil {
		return ""
	}
	return flags.Client
}

func resolveClientForEmail(email string, flags *RootFlags, cmdClient string) (string, error) {
	override := resolveClientOverride(flags, cmdClient)
	return authclient.ResolveClientWithOverride(email, override)
}

func normalizeClientForFlag(raw string) (string, error) {
	return config.NormalizeClientNameOrDefault(raw)
}

func resolveClientForEmailWithContext(ctx context.Context, email string, cmdClient string) (string, error) {
	override := strings.TrimSpace(cmdClient)
	if override == "" {
		override = authclient.ClientOverrideFromContext(ctx)
	}
	return authclient.ResolveClientWithOverride(email, override)
}
