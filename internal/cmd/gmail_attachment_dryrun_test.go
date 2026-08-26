package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/secrets"
)

func TestGmailAttachmentDownloadDryRunsAvoidAuthenticationAndWrites(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command []string
		op      string
		id      string
		indexed bool
		draft   bool
		custom  bool
	}{
		{
			name:    "thread get defaults to current directory",
			command: []string{"gmail", "thread", "get", "https://mail.google.com/mail/u/0/#inbox/18abcdef123", "--download"},
			op:      "gmail.thread.get.download",
			id:      "18abcdef123",
		},
		{
			name:    "thread attachments preserve explicit indexed destination",
			command: []string{"gmail", "thread", "attachments", "18abcdef123", "--download", "--use-indexed-attachment-ids"},
			op:      "gmail.thread.attachments.download",
			id:      "18abcdef123",
			indexed: true,
			custom:  true,
		},
		{
			name:    "draft get preserves configuration destination",
			command: []string{"gmail", "drafts", "get", "draft-123", "--download", "--use-indexed-attachment-ids"},
			op:      "gmail.drafts.get.download",
			id:      "draft-123",
			indexed: true,
			draft:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			home := filepath.Join(dir, "home")
			outputDir := "."
			if tc.custom {
				outputDir = filepath.Join(dir, "attachments")
			}
			if tc.draft {
				outputDir = filepath.Join(home, "config", "gmail-attachments")
			}
			authCalls, gmailCalls := 0, 0
			runtime := &app.Runtime{
				Auth: app.AuthOperations{
					OpenSecretsStore: func() (secrets.Store, error) {
						authCalls++
						return nil, errors.New("attachment dry-run opened the keyring")
					},
				},
				Services: app.Services{
					Gmail: func(context.Context, string) (*gmail.Service, error) {
						gmailCalls++
						return nil, errors.New("attachment dry-run created a Gmail client")
					},
				},
			}
			args := []string{"--home", home, "--json", "--no-input", "--readonly", "--gmail-no-send", "--dry-run"}
			args = append(args, tc.command...)
			if tc.custom {
				args = append(args, "--out-dir", outputDir)
			}
			result := executeWithTestRuntime(t, args, runtime)
			if result.err != nil {
				t.Fatalf("dry-run: %v\n%s", result.err, result.stderr)
			}
			if authCalls != 0 || gmailCalls != 0 {
				t.Fatalf("dry-run used auth or Gmail: auth=%d gmail=%d", authCalls, gmailCalls)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("dry-run wrote local data: entries=%#v err=%v", entries, err)
			}
			var got struct {
				DryRun  bool   `json:"dry_run"`
				Op      string `json:"op"`
				Request struct {
					ThreadID string `json:"thread_id"`
					DraftID  string `json:"draft_id"`
					OutDir   string `json:"out_dir"`
					Download bool   `json:"download"`
					Indexed  bool   `json:"use_indexed_attachment_ids"`
				} `json:"request"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
				t.Fatalf("decode dry-run: %v", err)
			}
			id := got.Request.ThreadID
			if tc.draft {
				id = got.Request.DraftID
			}
			if !got.DryRun || got.Op != tc.op || id != tc.id || got.Request.OutDir != outputDir ||
				!got.Request.Download || got.Request.Indexed != tc.indexed {
				t.Fatalf("unexpected dry-run plan: %#v", got)
			}
		})
	}
}
