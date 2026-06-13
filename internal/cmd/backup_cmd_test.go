package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/backup"
)

func TestBackupInitDryRunDoesNotWriteConfigOrRepo(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "backup.json")
	repoPath := filepath.Join(dir, "repo")

	var stdout bytes.Buffer
	err := (&BackupInitCmd{
		backupFlags: backupFlags{
			Config: configPath,
			Repo:   repoPath,
			NoPush: true,
		},
	}).Run(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), &RootFlags{DryRun: true, NoInput: true})

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("expected dry-run exit 0, got %#v", err)
	}
	if _, statErr := os.Stat(configPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("dry-run wrote config: %v", statErr)
	}
	if _, statErr := os.Stat(repoPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("dry-run created repo: %v", statErr)
	}

	var payload struct {
		DryRun  bool           `json:"dry_run"`
		Op      string         `json:"op"`
		Request map[string]any `json:"request"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode dry-run output: %v\n%s", decodeErr, stdout.String())
	}
	if !payload.DryRun || payload.Op != "backup.init" {
		t.Fatalf("unexpected dry-run payload: %#v", payload)
	}
	if payload.Request["repo"] != repoPath || payload.Request["push"] != false {
		t.Fatalf("unexpected request: %#v", payload.Request)
	}
	if payload.Request["remote"] != "" {
		t.Fatalf("dry-run --no-push should not use default remote: %#v", payload.Request)
	}
}

func TestBackupInitNoPushUsesLocalRepoWithoutDefaultRemote(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "backup.json")
	repoPath := filepath.Join(dir, "repo")
	identityPath := filepath.Join(dir, "age.key")

	var stdout bytes.Buffer
	err := (&BackupInitCmd{
		backupFlags: backupFlags{
			Config:   configPath,
			Repo:     repoPath,
			Identity: identityPath,
			NoPush:   true,
		},
	}).Run(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), &RootFlags{NoInput: true})
	if err != nil {
		t.Fatalf("BackupInitCmd.Run: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(repoPath, ".git")); statErr != nil {
		t.Fatalf("local repo was not initialized: %v", statErr)
	}
	cfg, loadErr := backupOptionsForCmdTest(t, backup.Options{ConfigPath: configPath}).ConfigStore.Load(configPath)
	if loadErr != nil {
		t.Fatalf("LoadConfig: %v", loadErr)
	}
	if cfg.Remote != "" {
		t.Fatalf("--no-push init used default remote: %q", cfg.Remote)
	}
}

func TestBackupInitNoPushPreservesConfiguredRemote(t *testing.T) {
	for _, remote := range []string{
		"git@example.com:private/backup.git",
		"https://github.com/steipete/backup-gog.git",
	} {
		t.Run(remote, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "backup.json")
			repoPath := filepath.Join(dir, "repo")
			identityPath := filepath.Join(dir, "age.key")
			if err := os.MkdirAll(repoPath, 0o700); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := exec.CommandContext(t.Context(), "git", "-C", repoPath, "init").Run(); err != nil {
				t.Fatalf("git init: %v", err)
			}
			store := backupOptionsForCmdTest(t, backup.Options{ConfigPath: configPath}).ConfigStore
			if err := store.Save(configPath, backup.Config{
				Repo:     repoPath,
				Remote:   remote,
				Identity: identityPath,
			}); err != nil {
				t.Fatalf("SaveConfig: %v", err)
			}

			err := (&BackupInitCmd{
				backupFlags: backupFlags{
					Config: configPath,
					NoPush: true,
				},
			}).Run(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), &RootFlags{NoInput: true})
			if err != nil {
				t.Fatalf("BackupInitCmd.Run: %v", err)
			}
			cfg, loadErr := store.Load(configPath)
			if loadErr != nil {
				t.Fatalf("LoadConfig: %v", loadErr)
			}
			if cfg.Remote != remote {
				t.Fatalf("--no-push init changed configured remote: %q", cfg.Remote)
			}
		})
	}
}

func TestWriteBackupResultUsesRuntimeOutput(t *testing.T) {
	var stdout bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard)
	if err := writeBackupResult(ctx, backup.Result{Repo: "/tmp/repo", Changed: true, Shards: 2}); err != nil {
		t.Fatalf("write result: %v", err)
	}

	var result backup.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Repo != "/tmp/repo" || !result.Changed || result.Shards != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestBackupExportReportsManifestSemanticCounts(t *testing.T) {
	repo, config, recipients := newBackupConfigForCmdTest(t)
	shard, err := backup.NewJSONLShard("contacts", "people", "acct", "data/contacts/acct/people/part-0001.jsonl.gz.age", []map[string]string{
		{"source": "connections"},
		{"source": "other"},
	})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	if _, pushErr := backup.PushSnapshot(t.Context(), backup.Snapshot{
		Services: []string{"contacts"},
		Accounts: []string{"acct"},
		Counts: map[string]int{
			"contacts.connections": 1,
			"contacts.other":       1,
			"contacts.people":      99,
		},
		Shards: []backup.PlainShard{shard},
	}, backupOptionsForCmdTest(t, backup.Options{ConfigPath: config, Recipients: recipients, Push: false})); pushErr != nil {
		t.Fatalf("PushSnapshot: %v", pushErr)
	}

	var stdout bytes.Buffer
	err = (&BackupExportCmd{
		backupReadFlags: backupReadFlags{Config: config, Repo: repo, NoPull: true},
		Out:             filepath.Join(t.TempDir(), "export"),
	}).Run(newCmdOutputContext(t, &stdout, io.Discard), &RootFlags{})
	if err != nil {
		t.Fatalf("BackupExportCmd.Run: %v", err)
	}
	for _, want := range []string{
		"count.contacts.connections\t1",
		"count.contacts.other\t1",
		"count.contacts.people\t2",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("export output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestBackupExportReportsManifestCountsForSemanticCollisions(t *testing.T) {
	repo, config, recipients := newBackupConfigForCmdTest(t)
	shard, err := backup.NewJSONLShard("drive", "contents", "acct", "data/drive/acct/contents/part-0001.jsonl.gz.age", []driveBackupContent{
		{FileID: "ok", Name: "ok", ExportName: "ok.txt", DataBase64: base64.StdEncoding.EncodeToString([]byte("ok"))},
		{FileID: "skipped", Name: "skipped", ExportName: "skipped.txt", Skipped: true},
		{FileID: "error", Name: "error", ExportName: "error.txt", Error: "export failed"},
	})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	if _, pushErr := backup.PushSnapshot(t.Context(), backup.Snapshot{
		Services: []string{"drive"},
		Accounts: []string{"acct"},
		Counts: map[string]int{
			"drive.contents":         1,
			"drive.contents.skipped": 1,
			"drive.contents.errors":  1,
		},
		Shards: []backup.PlainShard{shard},
	}, backupOptionsForCmdTest(t, backup.Options{ConfigPath: config, Recipients: recipients, Push: false})); pushErr != nil {
		t.Fatalf("PushSnapshot: %v", pushErr)
	}

	var stdout bytes.Buffer
	err = (&BackupExportCmd{
		backupReadFlags: backupReadFlags{Config: config, Repo: repo, NoPull: true},
		Out:             filepath.Join(t.TempDir(), "export"),
	}).Run(newCmdOutputContext(t, &stdout, io.Discard), &RootFlags{})
	if err != nil {
		t.Fatalf("BackupExportCmd.Run: %v", err)
	}
	for _, want := range []string{
		"count.drive.contents\t1",
		"count.drive.contents.errors\t1",
		"count.drive.contents.skipped\t1",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("export output missing %q:\n%s", want, stdout.String())
		}
	}
}
