package cmd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/backup"
	"github.com/openclaw/gogcli/internal/config"
)

func backupOptionsForCmdTest(t *testing.T, opts backup.Options) backup.Options {
	t.Helper()
	configDir := t.TempDir()
	if opts.ConfigPath != "" {
		configDir = filepath.Dir(opts.ConfigPath)
	}
	home := t.TempDir()
	store, err := backup.NewConfigStore(config.Layout{
		ConfigDir:      configDir,
		ExplicitConfig: true,
	}, func() (string, error) { return home, nil })
	if err != nil {
		t.Fatalf("backup.NewConfigStore: %v", err)
	}
	opts.ConfigStore = store
	return opts
}

func saveBackupConfigForCmdTest(t *testing.T, path string, cfg backup.Config) {
	t.Helper()
	opts := backupOptionsForCmdTest(t, backup.Options{ConfigPath: path})
	if err := opts.ConfigStore.Save(path, cfg); err != nil {
		t.Fatalf("save backup config: %v", err)
	}
}

func TestBindBackupConfigStoreUsesRuntimeLayout(t *testing.T) {
	ambient := t.TempDir()
	injected := t.TempDir()
	t.Setenv("GOG_CONFIG_DIR", ambient)

	runtime := &app.Runtime{Layout: config.Layout{
		ConfigDir:      injected,
		ExplicitConfig: true,
	}}
	ctx := app.WithRuntime(context.Background(), runtime)
	opts := backup.Options{}
	if err := bindBackupConfigStore(ctx, &opts); err != nil {
		t.Fatalf("bindBackupConfigStore: %v", err)
	}
	if got, want := opts.ConfigStore.Path(), filepath.Join(injected, "backup.json"); got != want {
		t.Fatalf("config path = %q, want %q", got, want)
	}
}
