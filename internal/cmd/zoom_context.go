package cmd

import (
	"context"

	"github.com/steipete/gogcli/internal/zoom"
)

type zoomIncludePasswordsContextKey struct{}

func withZoomIncludePasswords(ctx context.Context, include bool) context.Context {
	return context.WithValue(ctx, zoomIncludePasswordsContextKey{}, include)
}

func zoomIncludePasswordsFromContext(ctx context.Context) bool {
	if include, ok := ctx.Value(zoomIncludePasswordsContextKey{}).(bool); ok {
		return include
	}
	return zoom.IncludePasswordsFromEnv()
}
