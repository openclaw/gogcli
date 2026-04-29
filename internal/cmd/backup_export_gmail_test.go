package cmd

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/backup"
)

func TestDecodeGmailRawAcceptsBase64URLVariants(t *testing.T) {
	payload := []byte("Subject: Hello\r\n\r\nBody")
	raw := base64.RawURLEncoding.EncodeToString(payload)
	got, err := decodeGmailRaw(raw)
	if err != nil {
		t.Fatalf("decodeGmailRaw raw: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("raw decoded = %q, want %q", got, payload)
	}

	padded := base64.URLEncoding.EncodeToString(payload)
	got, err = decodeGmailRaw(padded)
	if err != nil {
		t.Fatalf("decodeGmailRaw padded: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("padded decoded = %q, want %q", got, payload)
	}
}

func TestExportGmailMessagesWritesReadableEMLAndIndex(t *testing.T) {
	outDir := t.TempDir()
	payload := []byte("Subject: Hello\r\nFrom: a@example.com\r\n\r\nBody")
	message := gmailBackupMessage{
		ID:           "msg/one",
		ThreadID:     "thread-1",
		InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"),
		LabelIDs:     []string{"INBOX"},
		Raw:          base64.RawURLEncoding.EncodeToString(payload),
	}
	shard, err := backup.NewJSONLShard("gmail", "messages", "acct/hash", "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", []gmailBackupMessage{message})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}

	files, count, err := exportGmailMessages(outDir, shard, backupExportOptions{GmailFormat: "eml"})
	if err != nil {
		t.Fatalf("exportGmailMessages: %v", err)
	}
	if files != 2 || count != 1 {
		t.Fatalf("files,count = %d,%d want 2,1", files, count)
	}

	emlRel := backupExportMessageEMLPath("acct_hash", message)
	eml, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(emlRel)))
	if err != nil {
		t.Fatalf("read eml: %v", err)
	}
	if string(eml) != string(payload) {
		t.Fatalf("eml = %q, want %q", eml, payload)
	}
	index := readText(t, filepath.Join(outDir, "gmail", "acct_hash", "messages", "index.jsonl"))
	if !strings.Contains(index, `"id":"msg/one"`) || !strings.Contains(index, `"eml":"`+emlRel+`"`) {
		t.Fatalf("index missing expected fields: %s", index)
	}
}

func TestExportGmailMessagesWritesMarkdownAndAttachments(t *testing.T) {
	outDir := t.TempDir()
	payload := strings.Join([]string{
		"Subject: Report",
		"From: Alice <alice@example.com>",
		"To: Peter <peter@example.com>",
		"Date: Thu, 02 Apr 2026 10:00:00 +0000",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="b1"`,
		"",
		"--b1",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Body text.",
		"--b1",
		"Content-Type: application/pdf",
		"Content-Transfer-Encoding: base64",
		`Content-Disposition: attachment; filename="report.pdf"`,
		"",
		base64.StdEncoding.EncodeToString([]byte("pdf bytes")),
		"--b1--",
		"",
	}, "\r\n")
	message := gmailBackupMessage{
		ID:           "msg/one",
		ThreadID:     "thread-1",
		InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"),
		LabelIDs:     []string{"INBOX"},
		Raw:          base64.RawURLEncoding.EncodeToString([]byte(payload)),
	}
	shard, err := backup.NewJSONLShard("gmail", "messages", "acct/hash", "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", []gmailBackupMessage{message})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}

	files, count, err := exportGmailMessages(outDir, shard, backupExportOptions{GmailFormat: "markdown", GmailAttachments: "extract"})
	if err != nil {
		t.Fatalf("exportGmailMessages: %v", err)
	}
	if files != 3 || count != 1 {
		t.Fatalf("files,count = %d,%d want 3,1", files, count)
	}
	messageDir := backupExportMessageDir("acct_hash", message, "Report")
	mdRel := filepath.ToSlash(filepath.Join(messageDir, "message.md"))
	md := readText(t, filepath.Join(outDir, filepath.FromSlash(mdRel)))
	for _, want := range []string{
		`subject: "Report"`,
		"# Report",
		"Body text.",
		"- [report.pdf](attachments/report.pdf)",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
	attachment := readText(t, filepath.Join(outDir, filepath.FromSlash(filepath.Join(messageDir, "attachments", "report.pdf"))))
	if attachment != "pdf bytes" {
		t.Fatalf("attachment = %q", attachment)
	}
	index := readText(t, filepath.Join(outDir, "gmail", "acct_hash", "messages", "index.jsonl"))
	if !strings.Contains(index, `"markdown":"`+mdRel+`"`) ||
		!strings.Contains(index, `"attachments":["`+filepath.ToSlash(filepath.Join(messageDir, "attachments", "report.pdf"))+`"]`) ||
		strings.Contains(index, `"eml"`) {
		t.Fatalf("index missing expected markdown-only fields: %s", index)
	}
}

func TestExportGmailMessagesWritesMarkdownFallbackForMalformedMIME(t *testing.T) {
	outDir := t.TempDir()
	payload := strings.Join([]string{
		"Subject: Broken",
		"From: Alice <alice@example.com>",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="b1"`,
		"",
		"--b1",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"incomplete body",
	}, "\r\n")
	message := gmailBackupMessage{
		ID:           "broken",
		InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"),
		Raw:          base64.RawURLEncoding.EncodeToString([]byte(payload)),
	}
	shard, err := backup.NewJSONLShard("gmail", "messages", "acct/hash", "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", []gmailBackupMessage{message})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}

	files, count, err := exportGmailMessages(outDir, shard, backupExportOptions{GmailFormat: "markdown", GmailAttachments: "extract"})
	if err != nil {
		t.Fatalf("exportGmailMessages: %v", err)
	}
	if files != 2 || count != 1 {
		t.Fatalf("files,count = %d,%d want 2,1", files, count)
	}
	mdRel := filepath.ToSlash(filepath.Join(backupExportMessageDir("acct_hash", message, "Broken"), "message.md"))
	md := readText(t, filepath.Join(outDir, filepath.FromSlash(mdRel)))
	for _, want := range []string{
		`subject: "Broken"`,
		"parse_error:",
		"MIME parse failed",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
	index := readText(t, filepath.Join(outDir, "gmail", "acct_hash", "messages", "index.jsonl"))
	if !strings.Contains(index, `"markdown":"`+mdRel+`"`) {
		t.Fatalf("index missing markdown fallback: %s", index)
	}
}

func TestBackupEmailMarkdownBodyCleansHTMLFragments(t *testing.T) {
	got := backupEmailMarkdownBody(backupEmail{TextBody: "<p>Hello&nbsp;<b>Peter</b></p>"})
	if got != "Hello Peter" {
		t.Fatalf("body = %q, want %q", got, "Hello Peter")
	}

	got = backupEmailMarkdownBody(backupEmail{HTMLBody: "<html><body><p>Hi<br>there</p></body></html>"})
	if got != "Hi there" {
		t.Fatalf("html body = %q, want %q", got, "Hi there")
	}
}
