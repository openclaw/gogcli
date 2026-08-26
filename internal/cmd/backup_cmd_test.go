package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/backup"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/secrets"
)

func syntheticBackupRemote() string {
	return (&url.URL{
		Scheme:   "https",
		User:     url.UserPassword("synthetic-user", "synthetic-password"),
		Host:     "example.invalid",
		Path:     "/backup.git",
		RawQuery: url.Values{"access_token": {"synthetic-query"}}.Encode(),
		Fragment: "synthetic-fragment",
	}).String()
}

func TestBackupPushDryRunDoesNotAuthenticateOrWrite(t *testing.T) {
	testCases := []struct {
		name     string
		command  []string
		op       string
		services string
	}{
		{
			name:     "all-service push",
			command:  []string{"backup", "push", "--services", "gmail,calendar"},
			op:       "backup.push",
			services: "gmail,calendar",
		},
		{
			name:     "gmail push",
			command:  []string{"backup", "gmail", "push"},
			op:       "backup.gmail.push",
			services: "gmail",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			home := filepath.Join(dir, "home")
			repo := filepath.Join(dir, "repo")
			identity := filepath.Join(dir, "age.key")
			remote := syntheticBackupRemote()
			authCalls := 0
			gmailCalls := 0
			runtime := &app.Runtime{
				Auth: app.AuthOperations{
					OpenSecretsStore: func() (secrets.Store, error) {
						authCalls++
						return nil, errors.New("dry-run opened authentication store")
					},
				},
				Services: app.Services{
					Gmail: func(context.Context, string) (*gmail.Service, error) {
						gmailCalls++
						return nil, errors.New("dry-run created Gmail service")
					},
				},
			}
			args := []string{
				"--home", home,
				"--account", "clawdbot@gmail.com",
				"--json", "--no-input", "--gmail-no-send", "--readonly", "--dry-run",
			}
			args = append(args, testCase.command...)
			args = append(args,
				"--repo", repo,
				"--remote", remote,
				"--config", filepath.Join(dir, "backup.json"),
				"--identity", identity,
				"--recipient", "age1syntheticrecipient",
				"--query", "newer_than:30d",
				"--max", "2",
				"--no-push",
			)
			result := executeWithTestRuntime(t, args, runtime)
			if result.err != nil {
				t.Fatalf("dry-run failed: %v\n%s", result.err, result.stderr)
			}
			if authCalls != 0 || gmailCalls != 0 {
				t.Fatalf("dry-run touched auth/Gmail: auth=%d gmail=%d", authCalls, gmailCalls)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatalf("read dry-run directory: %v", readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("dry-run created local files or directories: %#v", entries)
			}

			var payload struct {
				DryRun  bool   `json:"dry_run"`
				Op      string `json:"op"`
				Request struct {
					Services         []string `json:"services"`
					Repo             string   `json:"repo"`
					Remote           string   `json:"remote"`
					Identity         string   `json:"identity"`
					Recipients       []string `json:"recipients"`
					Push             bool     `json:"push"`
					Query            string   `json:"query"`
					Max              int64    `json:"max"`
					IncludeSpamTrash bool     `json:"include_spam_trash"`
					GmailCache       bool     `json:"gmail_cache"`
					GmailCheckpoints bool     `json:"gmail_checkpoints"`
				} `json:"request"`
			}
			if decodeErr := json.Unmarshal([]byte(result.stdout), &payload); decodeErr != nil {
				t.Fatalf("decode dry-run output: %v\n%s", decodeErr, result.stdout)
			}
			if !payload.DryRun || payload.Op != testCase.op {
				t.Fatalf("unexpected dry-run payload: %#v", payload)
			}
			request := payload.Request
			if strings.Join(request.Services, ",") != testCase.services || request.Repo != repo ||
				request.Remote != "https://redacted@example.invalid/backup.git?access_token=redacted#redacted" ||
				request.Identity != identity || strings.Join(request.Recipients, ",") != "age1syntheticrecipient" ||
				request.Push || request.Query != "newer_than:30d" || request.Max != 2 ||
				!request.IncludeSpamTrash || !request.GmailCache || !request.GmailCheckpoints {
				t.Fatalf("unexpected dry-run request: %#v", request)
			}
			for _, secret := range []string{"synthetic-user", "synthetic-password", "synthetic-query", "synthetic-fragment"} {
				if strings.Contains(result.stdout, secret) || strings.Contains(result.stderr, secret) {
					t.Fatalf("dry-run output exposed remote credential %q", secret)
				}
			}
		})
	}
}

func TestBackupPushDryRunResolvesDefaultDestinationWithoutWriting(t *testing.T) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve user home: %v", err)
	}
	for _, command := range [][]string{
		{"backup", "push", "--services", "gmail"},
		{"backup", "gmail", "push"},
	} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			dir := t.TempDir()
			args := []string{"--home", filepath.Join(dir, "home"), "--json", "--no-input", "--dry-run"}
			args = append(args, command...)
			result := executeWithTestRuntime(t, args, nil)
			if result.err != nil {
				t.Fatalf("dry-run failed: %v\n%s", result.err, result.stderr)
			}
			var payload struct {
				Request struct {
					Repo     string `json:"repo"`
					Remote   string `json:"remote"`
					Identity string `json:"identity"`
					Push     bool   `json:"push"`
				} `json:"request"`
			}
			if decodeErr := json.Unmarshal([]byte(result.stdout), &payload); decodeErr != nil {
				t.Fatalf("decode dry-run output: %v", decodeErr)
			}
			request := payload.Request
			if request.Repo != filepath.Join(userHome, "Projects", "backup-gog") ||
				request.Remote != backup.DefaultConfig().Remote ||
				request.Identity != filepath.Join(userHome, ".gog", "age.key") || !request.Push {
				t.Fatalf("unexpected default destination: %#v", request)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("dry-run created local files: entries=%#v err=%v", entries, readErr)
			}
		})
	}
}

func TestBackupPushDryRunRejectsInvalidServicesWithoutSideEffects(t *testing.T) {
	dir := t.TempDir()
	gmailCalls := 0
	runtime := &app.Runtime{
		Services: app.Services{
			Gmail: func(context.Context, string) (*gmail.Service, error) {
				gmailCalls++
				return nil, errors.New("dry-run created Gmail service")
			},
		},
	}
	result := executeWithTestRuntime(t, []string{
		"--home", filepath.Join(dir, "home"),
		"--account", "clawdbot@gmail.com",
		"--json", "--no-input", "--dry-run",
		"backup", "push", "--services", "gmail,nope",
		"--repo", filepath.Join(dir, "repo"),
	}, runtime)
	if result.err == nil || ExitCode(result.err) != 2 ||
		!strings.Contains(result.err.Error(), "unsupported backup service") {
		t.Fatalf("expected unsupported-service usage error, got %v", result.err)
	}
	if gmailCalls != 0 {
		t.Fatalf("dry-run created Gmail service %d times", gmailCalls)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dry-run directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid dry-run created local files or directories: %#v", entries)
	}
}

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

func TestBackupInitDryRunRedactsRemoteCredentials(t *testing.T) {
	dir := t.TempDir()
	remote := syntheticBackupRemote()
	var stdout bytes.Buffer
	err := (&BackupInitCmd{
		backupFlags: backupFlags{
			Config:   filepath.Join(dir, "backup.json"),
			Repo:     filepath.Join(dir, "repo"),
			Remote:   remote,
			Identity: filepath.Join(dir, "age.key"),
			NoPush:   true,
		},
	}).Run(newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard), &RootFlags{DryRun: true, NoInput: true})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("expected dry-run exit 0, got %#v", err)
	}
	for _, secret := range []string{"synthetic-user", "synthetic-password", "synthetic-query", "synthetic-fragment"} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("dry-run output exposed remote credential %q", secret)
		}
	}
	if !strings.Contains(stdout.String(), "https://redacted@example.invalid/backup.git?access_token=redacted#redacted") {
		t.Fatalf("missing redacted destination: %s", stdout.String())
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("dry-run created local files: entries=%#v err=%v", entries, readErr)
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
	for _, tc := range []struct {
		name   string
		remote string
		json   bool
	}{
		{name: "SSH remote", remote: "git@example.com:private/backup.git", json: true},
		{name: "HTTPS remote", remote: "https://github.com/steipete/backup-gog.git", json: true},
		{name: "credentialed remote JSON", remote: syntheticBackupRemote(), json: true},
		{name: "credentialed remote text", remote: syntheticBackupRemote()},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
				Remote:   tc.remote,
				Identity: identityPath,
			}); err != nil {
				t.Fatalf("SaveConfig: %v", err)
			}

			var output bytes.Buffer
			ctx := newCmdRuntimeOutputContext(t, &output, io.Discard)
			if tc.json {
				ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
			}
			err := (&BackupInitCmd{
				backupFlags: backupFlags{
					Config: configPath,
					NoPush: true,
				},
			}).Run(ctx, &RootFlags{NoInput: true})
			if err != nil {
				t.Fatalf("BackupInitCmd.Run: %v", err)
			}
			cfg, loadErr := store.Load(configPath)
			if loadErr != nil {
				t.Fatalf("LoadConfig: %v", loadErr)
			}
			if cfg.Remote != tc.remote {
				t.Fatalf("--no-push init changed configured remote: %q", cfg.Remote)
			}
			if !strings.Contains(output.String(), backup.RedactGitURL(tc.remote)) {
				t.Fatalf("output omitted sanitized configured remote: %q", output.String())
			}
			for _, secret := range []string{"synthetic-user", "synthetic-password", "synthetic-query", "synthetic-fragment"} {
				if strings.Contains(output.String(), secret) {
					t.Fatalf("backup init exposed remote credential %q", secret)
				}
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

func TestBackupStatusAndVerifyExposeReadFlags(t *testing.T) {
	for _, command := range []string{"status", "verify"} {
		t.Run(command, func(t *testing.T) {
			result := executeWithTestRuntime(t, []string{"schema", "backup " + command}, nil)
			if result.err != nil {
				t.Fatalf("schema: %v", result.err)
			}
			var doc schemaDoc
			if err := json.Unmarshal([]byte(result.stdout), &doc); err != nil {
				t.Fatalf("decode schema: %v", err)
			}
			if doc.Command == nil {
				t.Fatal("missing command schema")
			}
			flags := make(map[string]bool, len(doc.Command.Flags))
			for _, flag := range doc.Command.Flags {
				flags[flag.Name] = true
			}
			if !flags["no-pull"] {
				t.Fatal("missing --no-pull")
			}
			for _, hidden := range []string{"no-push", "recipient"} {
				if flags[hidden] {
					t.Fatalf("hidden compatibility flag --%s appears in default schema", hidden)
				}
			}

			hiddenResult := executeWithTestRuntime(t, []string{"schema", "--include-hidden", "backup " + command}, nil)
			if hiddenResult.err != nil {
				t.Fatalf("hidden schema: %v", hiddenResult.err)
			}
			var hiddenDoc schemaDoc
			if err := json.Unmarshal([]byte(hiddenResult.stdout), &hiddenDoc); err != nil {
				t.Fatalf("decode hidden schema: %v", err)
			}
			hiddenFlags := make(map[string]bool, len(hiddenDoc.Command.Flags))
			for _, flag := range hiddenDoc.Command.Flags {
				hiddenFlags[flag.Name] = flag.Hidden
			}
			for _, compat := range []string{"no-push", "recipient"} {
				if !hiddenFlags[compat] {
					t.Fatalf("missing hidden compatibility flag --%s", compat)
				}
			}
		})
	}
}

func TestBackupStatusAndVerifyDryRunDoNotCreateRepo(t *testing.T) {
	testCases := []struct {
		name string
		run  func(context.Context, *RootFlags, string) error
	}{
		{
			name: "status",
			run: func(ctx context.Context, flags *RootFlags, repo string) error {
				return (&BackupStatusCmd{
					backupReadFlags: backupReadFlags{Repo: repo},
				}).Run(ctx, flags)
			},
		},
		{
			name: "verify",
			run: func(ctx context.Context, flags *RootFlags, repo string) error {
				return (&BackupVerifyCmd{
					backupReadFlags: backupReadFlags{Repo: repo},
				}).Run(ctx, flags)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := filepath.Join(t.TempDir(), testCase.name+"-repo")
			var stdout bytes.Buffer
			ctx := newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard)
			err := testCase.run(ctx, &RootFlags{DryRun: true, NoInput: true}, repo)

			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 0 {
				t.Fatalf("expected dry-run exit 0, got %#v", err)
			}
			var payload struct {
				DryRun  bool           `json:"dry_run"`
				Op      string         `json:"op"`
				Request map[string]any `json:"request"`
			}
			if decodeErr := json.Unmarshal(stdout.Bytes(), &payload); decodeErr != nil {
				t.Fatalf("decode dry-run output: %v\n%s", decodeErr, stdout.String())
			}
			if !payload.DryRun || payload.Op != "backup."+testCase.name {
				t.Fatalf("unexpected dry-run payload: %#v", payload)
			}
			requestRepo, ok := payload.Request["repo"].(string)
			if !ok || requestRepo != repo {
				t.Fatalf("missing repo in request: %#v", payload.Request)
			}
			if _, statErr := os.Stat(repo); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("dry-run created repo: %v", statErr)
			}
		})
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
