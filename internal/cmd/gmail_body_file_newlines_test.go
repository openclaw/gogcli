package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Regression coverage for Gmail file-input flags reporting a
// trailing-newline-trimmed length in --dry-run output. The bytes handed to
// --body-file/--body-html-file/--note-file are used verbatim, so the reported
// lengths must match the file size and stay consistent with inline flags and
// slides update-notes.
//
// Repro fixture from the issue: a payload plus three trailing newlines.
const (
	bodyFileWithTrailingNewlines = "line one\nline two\n\n\n" // 20 bytes
	htmlFileWithTrailingNewlines = "<p>hi</p>\n\n"            // 11 bytes
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// runDryRunRequest executes a command's Run in --dry-run JSON mode and returns
// the "request" object emitted by dryRunExit.
func runDryRunRequest(t *testing.T, run func(context.Context, *RootFlags) error) map[string]any {
	t.Helper()
	var out bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &out, io.Discard)
	flags := &RootFlags{Account: "a@b.com", DryRun: true}

	err := run(ctx, flags)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("expected dry-run exit code 0, got: %v", err)
	}

	var got map[string]any
	if unmarshalErr := json.Unmarshal(out.Bytes(), &got); unmarshalErr != nil {
		t.Fatalf("unmarshal dry-run output: %v\noutput=%q", unmarshalErr, out.String())
	}
	req, ok := got["request"].(map[string]any)
	if !ok {
		t.Fatalf("dry-run output missing request object: %v", got)
	}
	return req
}

func assertLen(t *testing.T, req map[string]any, field string, want int) {
	t.Helper()
	got, ok := req[field].(float64)
	if !ok {
		t.Fatalf("%s not a number in dry-run output: %#v", field, req[field])
	}
	if int(got) != want {
		t.Fatalf("%s = %d, want %d (file bytes must be preserved verbatim, no trailing-newline trim)", field, int(got), want)
	}
}

func TestGmailSend_BodyFilePreservesTrailingNewlines(t *testing.T) {
	bodyPath := writeTempFile(t, "body.txt", bodyFileWithTrailingNewlines)
	htmlPath := writeTempFile(t, "body.html", htmlFileWithTrailingNewlines)

	cmd := &GmailSendCmd{
		To:           "x@example.com",
		Subject:      "T",
		BodyFile:     bodyPath,
		BodyHTMLFile: htmlPath,
	}
	req := runDryRunRequest(t, cmd.Run)
	assertLen(t, req, "body_len", len(bodyFileWithTrailingNewlines))
	assertLen(t, req, "body_html_len", len(htmlFileWithTrailingNewlines))
}

func TestGmailDraftsCreate_BodyFilePreservesTrailingNewlines(t *testing.T) {
	bodyPath := writeTempFile(t, "body.txt", bodyFileWithTrailingNewlines)
	htmlPath := writeTempFile(t, "body.html", htmlFileWithTrailingNewlines)

	cmd := &GmailDraftsCreateCmd{
		To:           "x@example.com",
		Subject:      "T",
		BodyFile:     bodyPath,
		BodyHTMLFile: htmlPath,
	}
	req := runDryRunRequest(t, cmd.Run)
	assertLen(t, req, "body_len", len(bodyFileWithTrailingNewlines))
	assertLen(t, req, "body_html_len", len(htmlFileWithTrailingNewlines))
}

func TestGmailDraftsUpdate_BodyFilePreservesTrailingNewlines(t *testing.T) {
	bodyPath := writeTempFile(t, "body.txt", bodyFileWithTrailingNewlines)
	htmlPath := writeTempFile(t, "body.html", htmlFileWithTrailingNewlines)

	cmd := &GmailDraftsUpdateCmd{
		DraftID:      "draft1",
		Subject:      "T",
		BodyFile:     bodyPath,
		BodyHTMLFile: htmlPath,
	}
	req := runDryRunRequest(t, cmd.Run)
	assertLen(t, req, "body_len", len(bodyFileWithTrailingNewlines))
	assertLen(t, req, "body_html_len", len(htmlFileWithTrailingNewlines))
}

func TestGmailReply_BodyFilePreservesTrailingNewlines(t *testing.T) {
	bodyPath := writeTempFile(t, "body.txt", bodyFileWithTrailingNewlines)
	htmlPath := writeTempFile(t, "body.html", htmlFileWithTrailingNewlines)

	cmd := &GmailReplyCmd{
		MessageID: "msg1",
		Options: GmailReplyOptions{
			BodyFile:     bodyPath,
			BodyHTMLFile: htmlPath,
		},
	}
	req := runDryRunRequest(t, cmd.Run)
	assertLen(t, req, "body_len", len(bodyFileWithTrailingNewlines))
	assertLen(t, req, "body_html_len", len(htmlFileWithTrailingNewlines))
}

func TestGmailAutoReply_BodyFilePreservesTrailingNewlines(t *testing.T) {
	bodyPath := writeTempFile(t, "body.txt", bodyFileWithTrailingNewlines)

	cmd := &GmailAutoReplyCmd{
		Query:    []string{"is:unread"},
		Max:      20,
		BodyFile: bodyPath,
		Label:    "AutoReplied",
	}
	req := runDryRunRequest(t, cmd.Run)
	assertLen(t, req, "body_len", len(bodyFileWithTrailingNewlines))
}

func TestGmailForward_NoteFilePreservesTrailingNewlines(t *testing.T) {
	notePath := writeTempFile(t, "note.txt", bodyFileWithTrailingNewlines)

	cmd := &GmailForwardCmd{
		MessageID: "msg1",
		Options: GmailForwardOptions{
			To:       "x@example.com",
			NoteFile: notePath,
		},
	}
	req := runDryRunRequest(t, cmd.Run)
	assertLen(t, req, "note_len", len(bodyFileWithTrailingNewlines))
}
