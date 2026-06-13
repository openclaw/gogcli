package cmd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
)

func TestManualAuthOperationsUseRuntimeLayout(t *testing.T) {
	ambient := filepath.Join(t.TempDir(), "ambient")
	injected := filepath.Join(t.TempDir(), "injected")
	t.Setenv("GOG_CONFIG_DIR", ambient)

	t.Run("authorize", func(t *testing.T) {
		var gotDir string
		runtime := &app.Runtime{
			Layout: config.Layout{
				ConfigDir:      injected,
				ExplicitConfig: true,
			},
			Auth: app.AuthOperations{
				AuthorizeGoogle: func(_ context.Context, opts googleauth.AuthorizeOptions) (string, error) {
					gotDir = opts.ManualStateStore.Dir()
					return "token", nil
				},
			},
		}
		ctx := app.WithRuntime(context.Background(), runtime)
		if _, err := authorizeGoogleAccount(ctx, googleauth.AuthorizeOptions{Manual: true}); err != nil {
			t.Fatalf("authorizeGoogleAccount: %v", err)
		}
		if gotDir != injected {
			t.Fatalf("manual state dir = %q, want %q", gotDir, injected)
		}
	})

	t.Run("url", func(t *testing.T) {
		var gotDir string
		runtime := &app.Runtime{
			Layout: config.Layout{
				ConfigDir:      injected,
				ExplicitConfig: true,
			},
			Auth: app.AuthOperations{
				ManualAuthURL: func(_ context.Context, opts googleauth.AuthorizeOptions) (googleauth.ManualAuthURLResult, error) {
					gotDir = opts.ManualStateStore.Dir()
					return googleauth.ManualAuthURLResult{URL: "https://example.com"}, nil
				},
			},
		}
		ctx := app.WithRuntime(context.Background(), runtime)
		if _, err := buildManualAuthURL(ctx, googleauth.AuthorizeOptions{Manual: true}); err != nil {
			t.Fatalf("buildManualAuthURL: %v", err)
		}
		if gotDir != injected {
			t.Fatalf("manual state dir = %q, want %q", gotDir, injected)
		}
	})
}
