package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/secrets"
	"github.com/openclaw/gogcli/internal/zoom"
)

type zoomAuthFixture struct {
	layout      config.Layout
	secretStore *memSecretsStore
}

func newZoomAuthFixture(t *testing.T) *zoomAuthFixture {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("GOG_ZOOM_ACCOUNT_ID", "")
	t.Setenv("GOG_ZOOM_CLIENT_ID", "")
	t.Setenv("GOG_ZOOM_CLIENT_SECRET", "")
	return &zoomAuthFixture{
		layout: config.Layout{
			ConfigDir:      configDir,
			ExplicitConfig: true,
		},
		secretStore: newMemSecretsStore(),
	}
}

func (f *zoomAuthFixture) context(ctx context.Context) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Layout = f.layout
		runtime.Auth.OpenSecretStore = func() (secrets.SecretStore, error) {
			return f.secretStore, nil
		}
	})
}

func (f *zoomAuthFixture) store(t *testing.T) *zoom.Store {
	t.Helper()
	store, err := zoom.NewStore(f.layout, func() (secrets.SecretStore, error) {
		return f.secretStore, nil
	}, os.LookupEnv)
	if err != nil {
		t.Fatalf("zoom.NewStore: %v", err)
	}
	return store
}

func (f *zoomAuthFixture) configDir() string {
	return f.layout.ConfigDir
}

func TestZoomAuthSetupCmd_StoresCredentialsWithoutValidation(t *testing.T) {
	fixture := newZoomAuthFixture(t)
	cmd := &ZoomAuthSetupCmd{
		Alias:        "work",
		AccountID:    "acct",
		ClientID:     "client",
		ClientSecret: "secret",
		SkipValidate: true,
	}
	var output bytes.Buffer
	ctx := fixture.context(newCmdRuntimeJSONOutputContext(t, &output, io.Discard))
	if err := cmd.Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(output.String(), `"saved": true`) {
		t.Fatalf("unexpected output: %s", output.String())
	}
	creds, err := fixture.store(t).LoadCredentials("work")
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds.AccountID != "acct" || creds.ClientID != "client" || creds.ClientSecret != "secret" {
		t.Fatalf("unexpected creds: %#v", creds)
	}
}

func TestZoomAuthDoctorCmd_NoCredentials(t *testing.T) {
	fixture := newZoomAuthFixture(t)
	var output bytes.Buffer
	ctx := fixture.context(newCmdRuntimeJSONOutputContext(t, &output, io.Discard))
	if err := (&ZoomAuthDoctorCmd{}).Run(ctx, &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(output.String(), `"status": "error"`) {
		t.Fatalf("unexpected output: %s", output.String())
	}
}

func TestZoomAuthSetupCmd_NoInputRequiresFlags(t *testing.T) {
	t.Setenv("GOG_ZOOM_ACCOUNT_ID", "")
	t.Setenv("GOG_ZOOM_CLIENT_ID", "")
	t.Setenv("GOG_ZOOM_CLIENT_SECRET", "")
	err := (&ZoomAuthSetupCmd{SkipValidate: true}).Run(context.Background(), &RootFlags{NoInput: true})
	if err == nil {
		t.Fatalf("expected usage error")
	}
}

func TestZoomAuthSetupCmd_DryRunDoesNotStoreCredentials(t *testing.T) {
	fixture := newZoomAuthFixture(t)
	cmd := &ZoomAuthSetupCmd{
		Alias:        "dry",
		SkipValidate: true,
	}
	var output bytes.Buffer
	ctx := fixture.context(newCmdRuntimeJSONOutputContext(t, &output, io.Discard))
	err := cmd.Run(ctx, &RootFlags{DryRun: true, NoInput: true})
	if ExitCode(err) != 0 {
		t.Fatalf("expected dry-run exit 0, got %v", err)
	}
	out := output.String()
	if strings.Contains(out, "topsecretvalue") {
		t.Fatalf("dry-run output leaked client secret: %s", out)
	}
	var parsed struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			Alias           string `json:"alias"`
			ClientSecretSet bool   `json:"client_secret_set"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("parse dry-run JSON: %v\n%s", err, out)
	}
	if !parsed.DryRun || parsed.Request.Alias != "dry" || parsed.Request.ClientSecretSet {
		t.Fatalf("unexpected dry-run payload: %#v", parsed)
	}
	if _, err := os.Stat(filepath.Join(fixture.configDir(), "zoom")); err == nil {
		t.Fatalf("dry-run created config directory")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat config directory: %v", err)
	}
}

func TestZoomAuthSetupCmd_ReadsPromptsFromRuntime(t *testing.T) {
	fixture := newZoomAuthFixture(t)
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	ctx := newCmdRuntimeIOContext(t, strings.NewReader("acct\nclient\nsecret\n"), &output, &diagnostics)
	ctx = fixture.context(ctx)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	err := (&ZoomAuthSetupCmd{
		Alias:        "prompted",
		SkipValidate: true,
	}).Run(ctx, &RootFlags{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	creds, err := fixture.store(t).LoadCredentials("prompted")
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds.AccountID != "acct" || creds.ClientID != "client" || creds.ClientSecret != "secret" {
		t.Fatalf("unexpected creds: %#v", creds)
	}
	if got := diagnostics.String(); !strings.Contains(got, "Zoom account ID: ") || !strings.Contains(got, "Zoom client secret: ") {
		t.Fatalf("unexpected prompts: %q", got)
	}
}
