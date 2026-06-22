package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/googleauth"
)

func TestAuthSetupDryRun(t *testing.T) {
	var stdout bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &stdout, &bytes.Buffer{})
	err := (&AuthSetupCmd{
		Email:         "user@example.com",
		Project:       "gog-test-project",
		ServicesCSV:   "gmail,docs",
		CreateProject: true,
		EnableAPIs:    true,
		Credentials:   "/tmp/client.json",
		Login:         true,
	}).Run(ctx, &RootFlags{DryRun: true, NoInput: true, ReadOnly: true})
	if ExitCode(err) != 0 {
		t.Fatalf("exit code = %d, want 0: %v", ExitCode(err), err)
	}

	var got struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			Project  string   `json:"project"`
			APIs     []string `json:"apis"`
			Readonly bool     `json:"readonly"`
		} `json:"request"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if !got.DryRun || got.Op != "auth.setup" || got.Request.Project != "gog-test-project" || !got.Request.Readonly {
		t.Fatalf("unexpected dry-run: %#v", got)
	}
	wantAPIs := []string{"docs.googleapis.com", "drive.googleapis.com", "gmail.googleapis.com"}
	if strings.Join(got.Request.APIs, ",") != strings.Join(wantAPIs, ",") {
		t.Fatalf("apis = %#v, want %#v", got.Request.APIs, wantAPIs)
	}
}

func TestAuthSetupGuidance(t *testing.T) {
	result := authSetupResult{
		Project:  "gog-test-project",
		Services: []string{"gmail", "drive"},
	}
	steps := authSetupNextSteps(&AuthSetupCmd{Email: "user@example.com"}, result, true, "work", true)
	joined := strings.Join(steps, "\n")
	for _, want := range []string{"auth/branding?project=gog-test-project", "auth/clients?project=gog-test-project", "gog --client work auth credentials", "gog --client work --readonly auth add user@example.com --services gmail,drive", "gog auth doctor --check"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("guidance missing %q:\n%s", want, joined)
		}
	}
}

func TestAuthSetupClientMatchesLoginResolution(t *testing.T) {
	ctx := authclient.WithClientResolver(context.Background(), func(email string, override string) (string, error) {
		if email != "user@example.com" || override != "" {
			t.Fatalf("resolver args = %q, %q", email, override)
		}

		return "mapped-client", nil
	})

	client, err := authSetupClient(ctx, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if client != "mapped-client" {
		t.Fatalf("client = %q, want mapped-client", client)
	}
}

func TestAuthSetupValidation(t *testing.T) {
	tests := []struct {
		name string
		cmd  AuthSetupCmd
		want string
	}{
		{name: "invalid project", cmd: AuthSetupCmd{Project: "BAD"}, want: "invalid --gcloud-project"},
		{name: "login email", cmd: AuthSetupCmd{Project: "valid-project", Login: true}, want: "--login requires an email"},
		{name: "create project", cmd: AuthSetupCmd{CreateProject: true}, want: "--create-project requires --gcloud-project"},
		{name: "open console", cmd: AuthSetupCmd{OpenConsole: true, Credentials: "/not/read.json"}, want: "--open-console requires --gcloud-project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", t.TempDir())
			err := tt.cmd.Run(context.Background(), &RootFlags{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestAuthSetupReadOnlyBlocksGcloudMutations(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := (&AuthSetupCmd{Project: "valid-project", CreateProject: true}).Run(context.Background(), &RootFlags{ReadOnly: true})
	if !errors.Is(err, googleapi.ErrReadOnly) {
		t.Fatalf("error = %v, want ErrReadOnly", err)
	}
}

func TestAPIServiceIDsForServices(t *testing.T) {
	got, err := googleauth.APIServiceIDsForServices([]googleauth.Service{
		googleauth.ServiceSheets,
		googleauth.ServiceDocs,
		googleauth.ServiceDrive,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"docs.googleapis.com", "drive.googleapis.com", "sheets.googleapis.com"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
