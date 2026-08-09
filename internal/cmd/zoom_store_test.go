package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
	"github.com/openclaw/gogcli/internal/zoom"
)

func TestCommandZoomStoreUsesRuntimeLayout(t *testing.T) {
	ambient := t.TempDir()
	injected := t.TempDir()
	t.Setenv("GOG_CONFIG_DIR", ambient)

	runtime := &app.Runtime{Layout: config.Layout{
		ConfigDir:      injected,
		ExplicitConfig: true,
	}}
	runtime.Auth.OpenSecretStore = func() (secrets.SecretStore, error) {
		return newMemSecretsStore(), nil
	}
	ctx := app.WithRuntime(context.Background(), runtime)
	store, err := commandZoomStore(ctx)
	if err != nil {
		t.Fatalf("commandZoomStore: %v", err)
	}
	if err := store.WriteMetadata("work", zoom.Metadata{AccountID: "acct", ClientID: "client"}); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(injected, "zoom", "work.json")); err != nil {
		t.Fatalf("injected metadata missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ambient, "zoom", "work.json")); !os.IsNotExist(err) {
		t.Fatalf("ambient metadata touched: %v", err)
	}
}
