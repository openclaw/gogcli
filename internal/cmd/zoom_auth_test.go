package cmd

import (
	"context"
	"testing"

	"github.com/steipete/gogcli/internal/zoom"
)

func withTempZoomAuthStore(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "test-pass")
	t.Setenv("GOG_ZOOM_ACCOUNT_ID", "")
	t.Setenv("GOG_ZOOM_CLIENT_ID", "")
	t.Setenv("GOG_ZOOM_CLIENT_SECRET", "")
}

func TestZoomAuthSetupCmd_StoresCredentialsWithoutValidation(t *testing.T) {
	withTempZoomAuthStore(t)
	cmd := &ZoomAuthSetupCmd{
		Alias:        "work",
		AccountID:    "acct",
		ClientID:     "client",
		ClientSecret: "secret",
		SkipValidate: true,
	}
	if err := cmd.Run(newCmdJSONContext(t), &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	creds, err := zoom.LoadCredentials("work")
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds.AccountID != "acct" || creds.ClientID != "client" || creds.ClientSecret != "secret" {
		t.Fatalf("unexpected creds: %#v", creds)
	}
}

func TestZoomAuthDoctorCmd_NoCredentials(t *testing.T) {
	withTempZoomAuthStore(t)
	if err := (&ZoomAuthDoctorCmd{}).Run(newCmdJSONContext(t), &RootFlags{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestZoomAuthSetupCmd_NoInputRequiresFlags(t *testing.T) {
	withTempZoomAuthStore(t)
	err := (&ZoomAuthSetupCmd{SkipValidate: true}).Run(context.Background(), &RootFlags{NoInput: true})
	if err == nil {
		t.Fatalf("expected usage error")
	}
}
