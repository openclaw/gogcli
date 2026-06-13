package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/authclient"
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
	if len(got.Services) != 2 {
		t.Fatalf("expected de-duped services, got %#v", got.Services)
	}
}

func TestAuthManageCmd_DryRunNoInputSkipsServer(t *testing.T) {
	var stdout bytes.Buffer
	called := false
	ctx := authclient.WithClient(newCmdRuntimeJSONOutputContext(t, &stdout, &bytes.Buffer{}), "work")
	ctx = withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Auth.StartManageServer = func(context.Context, googleauth.ManageServerOptions) error {
			called = true
			return nil
		}
	})

	err := (&AuthManageCmd{
		ServicesCSV:  string(googleauth.ServiceGmail) + "," + string(googleauth.ServiceDrive),
		ForceConsent: true,
		Timeout:      2 * time.Minute,
		ListenAddr:   " 127.0.0.1:8080 ",
		RedirectHost: "gog.example.com",
	}).Run(ctx, &RootFlags{DryRun: true, NoInput: true})
	if ExitCode(err) != 0 {
		t.Fatalf("exit code = %d, want 0: %v", ExitCode(err), err)
	}
	if called {
		t.Fatal("manage server was called during dry-run")
	}

	var got struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			Client       string   `json:"client"`
			ForceConsent bool     `json:"force_consent"`
			ListenAddr   string   `json:"listen_addr"`
			RedirectURI  string   `json:"redirect_uri"`
			Services     []string `json:"services"`
			Timeout      string   `json:"timeout"`
		} `json:"request"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode dry-run: %v\noutput=%q", err, stdout.String())
	}
	if !got.DryRun || got.Op != "auth.manage" {
		t.Fatalf("unexpected dry-run envelope: %#v", got)
	}
	if got.Request.Client != "work" ||
		!got.Request.ForceConsent ||
		got.Request.ListenAddr != "127.0.0.1:8080" ||
		got.Request.RedirectURI != "https://gog.example.com/oauth2/callback" ||
		got.Request.Timeout != "2m0s" ||
		len(got.Request.Services) != 2 ||
		got.Request.Services[0] != string(googleauth.ServiceGmail) ||
		got.Request.Services[1] != string(googleauth.ServiceDrive) {
		t.Fatalf("unexpected dry-run request: %#v", got.Request)
	}
}

func TestAuthManageCmd_NoInputFailsBeforeServer(t *testing.T) {
	called := false
	ctx := withTestRuntime(context.Background(), func(runtime *app.Runtime) {
		runtime.Auth.StartManageServer = func(context.Context, googleauth.ManageServerOptions) error {
			called = true
			return nil
		}
	})

	err := (&AuthManageCmd{}).Run(ctx, &RootFlags{NoInput: true})
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2: %v", ExitCode(err), err)
	}
	if called {
		t.Fatal("manage server was called with --no-input")
	}
	if !strings.Contains(err.Error(), "use 'gog auth import'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthManageCmdKeychainPreflightUsesRuntime(t *testing.T) {
	called := false
	ctx := withTestRuntime(context.Background(), func(runtime *app.Runtime) {
		runtime.KeyringOptions.Backend = "keychain"
		runtime.Auth.EnsureKeychainAccess = func(context.Context) error {
			called = true
			return nil
		}
		runtime.Auth.StartManageServer = func(ctx context.Context, _ googleauth.ManageServerOptions) error {
			return ensureKeychainAccessIfNeeded(ctx)
		}
	})

	if err := runKong(t, &AuthManageCmd{}, nil, ctx, nil); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !called {
		t.Fatal("runtime keychain preflight was not called")
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

func TestStartAuthManageServerRequiresRuntime(t *testing.T) {
	err := startAuthManageServer(context.Background(), googleauth.ManageServerOptions{})
	if !errors.Is(err, errRuntimeServiceRequired) {
		t.Fatalf("error = %v, want %v", err, errRuntimeServiceRequired)
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
			StartManageServer: func(ctx context.Context, _ googleauth.ManageServerOptions) error {
				return authclient.UpdateEmailReferences(ctx, "old@example.com", "new@example.com")
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
