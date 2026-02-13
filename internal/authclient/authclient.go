package authclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/degree-analytics/ratatosk/internal/config"
)

type contextKey struct{}

func WithClient(ctx context.Context, client string) context.Context {
	client = strings.TrimSpace(client)
	if client == "" {
		return ctx
	}

	return context.WithValue(ctx, contextKey{}, client)
}

func ClientOverrideFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if v := ctx.Value(contextKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}

	return ""
}

func ResolveClient(ctx context.Context, email string) (string, error) {
	cfg, err := config.ReadConfig()
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}
	override := ClientOverrideFromContext(ctx)

	client, err := config.ResolveClientForAccount(cfg, email, override)
	if err != nil {
		return "", fmt.Errorf("resolve client: %w", err)
	}

	return client, nil
}

func ResolveClientWithOverride(email string, override string) (string, error) {
	cfg, err := config.ReadConfig()
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}

	client, err := config.ResolveClientForAccount(cfg, email, override)
	if err != nil {
		return "", fmt.Errorf("resolve client: %w", err)
	}

	return client, nil
}
