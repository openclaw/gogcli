package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachmentByIndex(t *testing.T) {
	svc := newGmailAttachmentTestService(t, []byte("x"), "doc.pdf", "application/pdf")
	ctx := context.Background()

	att, err := attachmentByIndex(ctx, svc, "m1", 0)
	if err != nil {
		t.Fatalf("index 0: %v", err)
	}
	if att.AttachmentID != "a1" {
		t.Fatalf("attachmentId = %q, want a1", att.AttachmentID)
	}

	if _, err := attachmentByIndex(ctx, svc, "m1", 3); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("out-of-range err = %v", err)
	}

	// A negative index is rejected before any fetch.
	if _, err := attachmentByIndex(ctx, svc, "m1", -1); err == nil || !strings.Contains(err.Error(), ">= 0") {
		t.Fatalf("negative err = %v", err)
	}
}

func TestAttachmentOutputs_IndexedMode(t *testing.T) {
	atts := []attachmentInfo{
		{AttachmentIndex: 0, Filename: "a.pdf", Size: 10, MimeType: "application/pdf", AttachmentID: "LONGID-AAA"},
		{AttachmentIndex: 1, Filename: "b.png", Size: 20, MimeType: "image/png", AttachmentID: "LONGID-BBB"},
	}

	plain := attachmentOutputs(atts, false)
	if plain[0].AttachmentID != "LONGID-AAA" || plain[1].AttachmentID != "LONGID-BBB" {
		t.Fatalf("default mode should keep real ids: %#v", plain)
	}

	indexed := attachmentOutputs(atts, true)
	if indexed[0].AttachmentIndex == nil || *indexed[0].AttachmentIndex != 0 ||
		indexed[1].AttachmentIndex == nil || *indexed[1].AttachmentIndex != 1 {
		t.Fatalf("indexed mode should surface positions: %#v", indexed)
	}
	if indexed[0].AttachmentID != "" {
		t.Fatalf("indexed mode must not emit the real id: %#v", indexed[0])
	}
	if indexed[0].Filename != "a.pdf" || indexed[1].Size != 20 {
		t.Fatalf("indexed mode must not touch non-id fields: %#v", indexed)
	}
}

func TestExecute_Gmail_IndexedAttachmentIDs_FromEnv(t *testing.T) {
	// The env var enables indexed mode without the CLI flag (kong reads the env tag).
	t.Setenv("GOG_GMAIL_USE_INDEXED_ATTACHMENT_IDS", "1")
	svc := newGmailAttachmentTestService(t, []byte("data"), "doc.pdf", "application/pdf")
	result := executeWithGmailTestService(t, []string{
		"--json", "--account", "a@b.com", "gmail", "get", "m1",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	var parsed struct {
		Attachments []struct {
			AttachmentIndex *int `json:"attachmentIndex"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("decode: %v\nout=%q", err, result.stdout)
	}
	if len(parsed.Attachments) != 1 || parsed.Attachments[0].AttachmentIndex == nil || *parsed.Attachments[0].AttachmentIndex != 0 {
		t.Fatalf("env var should enable indexed ids: %#v", parsed.Attachments)
	}
}

func TestExecute_GmailAttachment_IndexResolvesToAttachment(t *testing.T) {
	data := []byte("index-download-content")
	svc := newGmailAttachmentTestService(t, data, "doc.pdf", "application/pdf")

	// In indexed mode, index 0 resolves to the message's first attachment (a1).
	parsed := executeGmailAttachmentJSON(t, svc,
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "--use-indexed-attachment-ids", "m1", "0",
		"--out", tempFilePath(t, "doc.pdf"), "--inline",
	)
	decoded, err := base64.StdEncoding.DecodeString(parsed["contentBase64"].(string))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(decoded, data) {
		t.Fatalf("content = %q, want %q", decoded, data)
	}
}

func TestExecute_GmailAttachment_IndexOutOfRange(t *testing.T) {
	svc := newGmailAttachmentTestService(t, []byte("x"), "doc.pdf", "application/pdf")
	result := executeWithGmailTestService(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "--use-indexed-attachment-ids", "m1", "5",
		"--out", tempFilePath(t, "doc.pdf"),
	}, svc)
	if result.err == nil || !strings.Contains(result.err.Error(), "out of range") {
		t.Fatalf("err = %v, want out-of-range", result.err)
	}
}

func TestExecute_GmailAttachment_IndexedModeRejectsNonIndex(t *testing.T) {
	svc := newGmailAttachmentTestService(t, []byte("x"), "doc.pdf", "application/pdf")
	result := executeWithGmailTestService(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "--use-indexed-attachment-ids", "m1", "a1",
		"--out", tempFilePath(t, "doc.pdf"),
	}, svc)
	if result.err == nil || !strings.Contains(result.err.Error(), "must be a 0-based index") {
		t.Fatalf("err = %v, want index-only rejection", result.err)
	}
}

func TestExecute_GmailAttachment_IndexedDryRunValidatesLocally(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "attachment.bin")
	invalid := executeWithTestRuntime(t, []string{
		"--json", "--dry-run", "gmail", "attachment", "--use-indexed-attachment-ids",
		"m1", "not-an-index", "--out", outPath,
	}, nil)
	if invalid.err == nil || !strings.Contains(invalid.err.Error(), "must be a 0-based index") {
		t.Fatalf("invalid dry-run err = %v, want index validation", invalid.err)
	}

	valid := executeWithTestRuntime(t, []string{
		"--json", "--dry-run", "gmail", "attachment", "--use-indexed-attachment-ids",
		"m1", "0", "--out", outPath,
	}, nil)
	if valid.err != nil {
		t.Fatalf("valid dry-run: %v\nstderr=%q", valid.err, valid.stderr)
	}
	var plan struct {
		Request struct {
			AttachmentIndex *int   `json:"attachment_index"`
			AttachmentID    string `json:"attachment_id"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(valid.stdout), &plan); err != nil {
		t.Fatalf("decode dry-run: %v\nout=%q", err, valid.stdout)
	}
	if plan.Request.AttachmentIndex == nil || *plan.Request.AttachmentIndex != 0 || plan.Request.AttachmentID != "" {
		t.Fatalf("unexpected indexed dry-run plan: %#v", plan.Request)
	}
}

func TestExecute_GmailAttachment_IndexedMode_FilenameUsesIndex(t *testing.T) {
	dir := t.TempDir()
	svc := newGmailAttachmentTestService(t, []byte("data"), "doc.pdf", "application/pdf")
	parsed := executeGmailAttachmentJSON(t, svc,
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "--use-indexed-attachment-ids", "m1", "0", "--out", dir,
	)
	if path, _ := parsed["path"].(string); !strings.HasSuffix(path, "m1_0_attachment.bin") {
		t.Fatalf("indexed-mode default filename must embed the index, got path=%q", path)
	}
}

func TestExecute_GmailAttachment_NumericArgIsRawIDWithoutFlag(t *testing.T) {
	// Without the flag, "0" is a (real) attachmentId, not an index: the test server
	// only serves a1, so downloading id "0" fails rather than resolving to index 0.
	svc := newGmailAttachmentTestService(t, []byte("x"), "doc.pdf", "application/pdf")
	result := executeWithGmailTestService(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "m1", "0",
		"--out", tempFilePath(t, "doc.pdf"),
	}, svc)
	if result.err == nil {
		t.Fatalf("expected download of raw id %q to fail, got success", "0")
	}
}

func TestExecute_GmailGet_IndexedAttachmentIDs(t *testing.T) {
	svc := newGmailAttachmentTestService(t, []byte("data"), "doc.pdf", "application/pdf")
	result := executeWithGmailTestService(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "get", "--use-indexed-attachment-ids", "m1",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	var parsed struct {
		Message struct {
			Payload struct {
				Parts []struct {
					Body struct {
						AttachmentID string `json:"attachmentId"`
					} `json:"body"`
				} `json:"parts"`
			} `json:"payload"`
		} `json:"message"`
		Attachments []struct {
			AttachmentIndex *int `json:"attachmentIndex"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("decode: %v\nout=%q", err, result.stdout)
	}
	if len(parsed.Attachments) != 1 || parsed.Attachments[0].AttachmentIndex == nil || *parsed.Attachments[0].AttachmentIndex != 0 {
		t.Fatalf("gmail get should surface index in indexed mode: %#v", parsed.Attachments)
	}
	// The raw message dump must omit the opaque id in indexed mode.
	if len(parsed.Message.Payload.Parts) != 1 || parsed.Message.Payload.Parts[0].Body.AttachmentID != "" {
		t.Fatalf("raw message dump should omit the attachmentId: %#v", parsed.Message.Payload.Parts)
	}
	if strings.Contains(result.stdout, "a1") {
		t.Fatalf("indexed mode must not leak the opaque attachmentId anywhere: out=%q", result.stdout)
	}
}
