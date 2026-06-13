package cmd

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
)

func TestAuthManageCmd_ServicesAndOptions(t *testing.T) {
	var got googleauth.ManageServerOptions
	ctx := withTestRuntime(context.Background(), func(runtime *app.Runtime) {
		runtime.Auth.StartManageServer = func(ctx context.Context, opts googleauth.ManageServerOptions) error {
			got = opts
			return nil
		}
	})

	if err := runKong(t, &AuthManageCmd{}, []string{"--services", "gmail,drive,gmail", "--force-consent", "--timeout", "2m", "--listen-addr", "0.0.0.0:8080", "--redirect-host", "gog.example.com"}, ctx, nil); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !got.ForceConsent {
		t.Fatalf("expected force-consent")
	}
	if got.Timeout != 2*time.Minute {
		t.Fatalf("unexpected timeout: %v", got.Timeout)
	}
	if got.ListenAddr != "0.0.0.0:8080" {
		t.Fatalf("unexpected listen addr: %q", got.ListenAddr)
	}
	if got.RedirectURI != "https://gog.example.com/oauth2/callback" {
		t.Fatalf("unexpected redirect uri: %q", got.RedirectURI)
	}
	if got.UpdateEmailReferences == nil {
		t.Fatal("expected email reference updater")
	}
	if len(got.Services) != 2 {
		t.Fatalf("expected de-duped services, got %#v", got.Services)
	}
}

func TestAuthManageCmd_InvalidService(t *testing.T) {
	ctx := withTestRuntime(context.Background(), func(runtime *app.Runtime) {
		runtime.Auth.StartManageServer = func(context.Context, googleauth.ManageServerOptions) error { return nil }
	})

	if err := runKong(t, &AuthManageCmd{}, []string{"--services", "nope"}, ctx, nil); err == nil {
		t.Fatalf("expected error")
	} else {
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 2 {
			t.Fatalf("expected usage exit code 2, got %#v", err)
		}
	}
}

func TestAuthManageCmd_DefaultServices_UserPreset(t *testing.T) {
	var got googleauth.ManageServerOptions
	ctx := withTestRuntime(context.Background(), func(runtime *app.Runtime) {
		runtime.Auth.StartManageServer = func(ctx context.Context, opts googleauth.ManageServerOptions) error {
			got = opts
			return nil
		}
	})

	if err := runKong(t, &AuthManageCmd{}, nil, ctx, nil); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if len(got.Services) != len(googleauth.UserServices()) {
		t.Fatalf("unexpected services: %v", got.Services)
	}
	for _, s := range got.Services {
		if s == googleauth.ServiceKeep {
			t.Fatalf("unexpected keep in services: %v", got.Services)
		}
	}
}

func TestAuthManageCmd_KeepRejected(t *testing.T) {
	ctx := withTestRuntime(context.Background(), func(runtime *app.Runtime) {
		runtime.Auth.StartManageServer = func(context.Context, googleauth.ManageServerOptions) error { return nil }
	})

	if err := runKong(t, &AuthManageCmd{}, []string{"--services", "keep"}, ctx, nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestExecuteAuthManageUsesRuntimeEmailReferenceUpdater(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ambientStore := defaultConfigStoreForTest(t)
	if err := ambientStore.Write(config.File{
		AccountAliases: map[string]string{"work": "old@example.com"},
	}); err != nil {
		t.Fatalf("write ambient config: %v", err)
	}
	runtimeStore := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := runtimeStore.Write(config.File{
		AccountAliases: map[string]string{"work": "old@example.com"},
	}); err != nil {
		t.Fatalf("write runtime config: %v", err)
	}

	result := executeWithTestRuntime(t, []string{"auth", "manage"}, &app.Runtime{
		Config: runtimeStore,
		Auth: app.AuthOperations{
			StartManageServer: func(_ context.Context, opts googleauth.ManageServerOptions) error {
				return opts.UpdateEmailReferences("old@example.com", "new@example.com")
			},
		},
	})
	if result.err != nil {
		t.Fatalf("execute: %v", result.err)
	}

	runtimeCfg, err := runtimeStore.Read()
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	if runtimeCfg.AccountAliases["work"] != "new@example.com" {
		t.Fatalf("runtime aliases = %#v", runtimeCfg.AccountAliases)
	}
	ambientCfg, err := ambientStore.Read()
	if err != nil {
		t.Fatalf("read ambient config: %v", err)
	}
	if ambientCfg.AccountAliases["work"] != "old@example.com" {
		t.Fatalf("ambient aliases changed: %#v", ambientCfg.AccountAliases)
	}
}
