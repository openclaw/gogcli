package cmd

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDriveCommand_ValidationErrors(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}

	moveCmd := &DriveMoveCmd{}
	if err := runKong(t, moveCmd, []string{"file1"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "missing --parent") {
		t.Fatalf("expected parent error, got %v", err)
	}

	lsCmd := &DriveLsCmd{}
	if err := runKong(t, lsCmd, []string{"--all", "--parent", "p1"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "--all cannot be combined with --parent") {
		t.Fatalf("expected mutually exclusive error, got %v", err)
	}

	shareCmd := &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "must specify --to") {
		t.Fatalf("expected share target error, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--to", "domain", "--domain", "example.com", "--role", "owner"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "invalid --role") {
		t.Fatalf("expected role error for domain share, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--to", "anyone", "--role", "owner"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "invalid --role") {
		t.Fatalf("expected role error, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--to", "user"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "missing --email") {
		t.Fatalf("expected missing email error, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--to", "domain"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "missing --domain") {
		t.Fatalf("expected missing domain error, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--to", "user", "--email", "a@b.com", "--discoverable"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "discoverable") {
		t.Fatalf("expected discoverable error, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--email", "a@b.com", "--domain", "example.com"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous target error, got %v", err)
	}
}

func TestDriveDeleteUnshare_NoInput(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", NoInput: true}

	deleteCmd := &DriveDeleteCmd{}
	if err := runKong(t, deleteCmd, []string{"file1"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("expected refusing error, got %v", err)
	}

	unshareCmd := &DriveUnshareCmd{}
	if err := runKong(t, unshareCmd, []string{"file1", "perm1"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("expected refusing error, got %v", err)
	}

	shareCmd := &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--to", "anyone"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "refusing to share drive file file1 with anyone (public)") {
		t.Fatalf("expected refusing error, got %v", err)
	}
}

func TestDriveDelete_DryRunJSON(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		err := (&DriveDeleteCmd{FileID: "file1", Permanent: true}).Run(ctx, &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true})
		if ExitCode(err) != 0 {
			t.Fatalf("expected dry-run exit, got %v", err)
		}
	})

	var payload struct {
		Op      string         `json:"op"`
		Request map[string]any `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if payload.Op != "drive.delete" {
		t.Fatalf("unexpected op: %#v", payload)
	}
	if payload.Request["file_id"] != "file1" || payload.Request["permanent"] != true {
		t.Fatalf("unexpected request: %#v", payload.Request)
	}
}
