package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/mailmime"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/tracking"
	"github.com/openclaw/gogcli/internal/ui"
)

func TestResolveTrackingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	cmd := &GmailSendCmd{}
	cmd.BodyHTML = "<html></html>"
	ctx := withAuthStore(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), newMemSecretsStore())

	// Multiple recipients without split should fail.
	if _, err := cmd.resolveTrackingConfig(ctx, "a@b.com", []string{"a@b.com", "b@b.com"}, nil, nil, cmd.BodyHTML); err == nil {
		t.Fatalf("expected error for multiple recipients without split")
	}

	cmd.TrackSplit = true
	cmd.BodyHTML = ""
	if _, err := cmd.resolveTrackingConfig(ctx, "a@b.com", []string{"a@b.com"}, nil, nil, cmd.BodyHTML); err == nil {
		t.Fatalf("expected error for missing body html")
	} else if ExitCode(err) != 2 {
		t.Fatalf("missing HTML exit = %d, want 2: %v", ExitCode(err), err)
	}

	cmd.BodyHTML = "<html></html>"
	if _, err := cmd.resolveTrackingConfig(ctx, "a@b.com", []string{"a@b.com"}, nil, nil, cmd.BodyHTML); err == nil {
		t.Fatalf("expected error for unconfigured tracking")
	} else if ExitCode(err) != exitCodeConfig {
		t.Fatalf("unconfigured tracking exit = %d, want %d: %v", ExitCode(err), exitCodeConfig, err)
	}

	key, err := tracking.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   "https://example.com",
		TrackingKey: key,
		AdminKey:    "admin",
	}
	saveTrackingConfigForTest(t, cfg)

	got, err := cmd.resolveTrackingConfig(ctx, "a@b.com", []string{"a@b.com"}, nil, nil, cmd.BodyHTML)
	if err != nil {
		t.Fatalf("resolveTrackingConfig: %v", err)
	}
	if got == nil || !got.IsConfigured() {
		t.Fatalf("expected configured tracking, got %#v", got)
	}
}

// TestResolveTrackingConfig_CountsParsedRecipients proves the --track
// exactly-1-recipient gate counts address-aware parsed recipients: a single
// display-name mailbox whose comma splitCSV used to miscount as two
// recipients passes the gate, and a same-address pair deduped within the flag
// counts as one. Each call stops at the empty-HTML-body check — the error
// must be the HTML one, not the count one — so the test proves the gate
// passed without touching the env-dependent tracking config load. The same
// parsed list must also yield a single send batch.
func TestResolveTrackingConfig_CountsParsedRecipients(t *testing.T) {
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	cases := []struct {
		name  string
		value string
		bare  string
	}{
		{name: "display name comma", value: `"Smith, John" <john@example.com>`, bare: "john@example.com"},
		{name: "case duplicate pair", value: "a@x.com, A@X.COM", bare: "a@x.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			to, err := parseRecipientCSV("--to", tc.value)
			if err != nil {
				t.Fatalf("parseRecipientCSV: %v", err)
			}
			if len(to) != 1 {
				t.Fatalf("expected 1 parsed recipient, got %#v", to)
			}

			cmd := &GmailSendCmd{Track: true}
			_, err = cmd.resolveTrackingConfig(ctx, "a@b.com", to, nil, nil, "")
			if err == nil || !strings.Contains(err.Error(), "HTML body") {
				t.Fatalf("expected HTML-body error (count gate passed), got: %v", err)
			}
			if strings.Contains(err.Error(), "exactly 1 recipient") {
				t.Fatalf("count gate rejected a single parsed recipient: %v", err)
			}

			batches := buildSendBatches(to, nil, nil, true, false)
			if len(batches) != 1 {
				t.Fatalf("expected 1 batch, got %#v", batches)
			}
			if batches[0].TrackingRecipient != tc.bare {
				t.Fatalf("tracking recipient = %q, want bare %q", batches[0].TrackingRecipient, tc.bare)
			}
		})
	}
}

func TestFirstRecipient(t *testing.T) {
	if got := firstRecipient([]string{"a"}, []string{"b"}, []string{"c"}); got != "a" {
		t.Fatalf("unexpected first recipient: %q", got)
	}
	if got := firstRecipient(nil, []string{"b"}, nil); got != "b" {
		t.Fatalf("unexpected first recipient: %q", got)
	}
	if got := firstRecipient(nil, nil, []string{"c"}); got != "c" {
		t.Fatalf("unexpected first recipient: %q", got)
	}
}

func TestWriteSendResults_JSONMultiple(t *testing.T) {
	out := captureStdout(t, func() {
		u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
		if err != nil {
			t.Fatalf("ui.New: %v", err)
		}
		ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

		if err := writeSendResults(ctx, u, "from@example.com", []sendResult{
			{MessageID: "m1", ThreadID: "t1", To: "a@example.com"},
			{MessageID: "m2", ThreadID: "t2", To: "b@example.com"},
		}, []mailmime.AttachmentMetadata{{Filename: "report.pdf", Size: 42}}); err != nil {
			t.Fatalf("writeSendResults: %v", err)
		}
	})
	var parsed struct {
		Messages []struct {
			Attachments []mailmime.AttachmentMetadata `json:"attachments"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(parsed.Messages) != 2 {
		t.Fatalf("unexpected messages: %#v", parsed.Messages)
	}
	for i, message := range parsed.Messages {
		if len(message.Attachments) != 1 || message.Attachments[0].Filename != "report.pdf" || message.Attachments[0].Size != 42 {
			t.Fatalf("messages[%d] attachments = %#v", i, message.Attachments)
		}
	}
}
