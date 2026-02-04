package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

// ==================== gmail_attachments.go tests ====================

func TestAttachmentOutputFromInfo(t *testing.T) {
	info := attachmentInfo{
		Filename:     "test.pdf",
		Size:         1024,
		MimeType:     "application/pdf",
		AttachmentID: "att123",
	}
	out := attachmentOutputFromInfo(info)

	if out.Filename != "test.pdf" {
		t.Fatalf("expected filename test.pdf, got %s", out.Filename)
	}
	if out.Size != 1024 {
		t.Fatalf("expected size 1024, got %d", out.Size)
	}
	if out.SizeHuman != "1.0 KB" {
		t.Fatalf("expected 1.0 KB, got %s", out.SizeHuman)
	}
	if out.MimeType != "application/pdf" {
		t.Fatalf("expected application/pdf, got %s", out.MimeType)
	}
	if out.AttachmentID != "att123" {
		t.Fatalf("expected att123, got %s", out.AttachmentID)
	}
}

func TestAttachmentOutputs_Empty(t *testing.T) {
	result := attachmentOutputs(nil)
	if result != nil {
		t.Fatalf("expected nil for empty input, got %v", result)
	}

	result = attachmentOutputs([]attachmentInfo{})
	if result != nil {
		t.Fatalf("expected nil for empty slice, got %v", result)
	}
}

func TestAttachmentOutputs_Multiple(t *testing.T) {
	infos := []attachmentInfo{
		{Filename: "a.txt", Size: 100, MimeType: "text/plain", AttachmentID: "a1"},
		{Filename: "b.pdf", Size: 2048, MimeType: "application/pdf", AttachmentID: "b2"},
	}
	result := attachmentOutputs(infos)

	if len(result) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(result))
	}
	if result[0].Filename != "a.txt" || result[1].Filename != "b.pdf" {
		t.Fatalf("unexpected filenames: %v", result)
	}
}

func TestAttachmentOutputsFromDownloads_Empty(t *testing.T) {
	result := attachmentOutputsFromDownloads(nil)
	if result != nil {
		t.Fatalf("expected nil for empty input, got %v", result)
	}

	result = attachmentOutputsFromDownloads([]attachmentDownloadOutput{})
	if result != nil {
		t.Fatalf("expected nil for empty slice, got %v", result)
	}
}

func TestAttachmentOutputsFromDownloads_Multiple(t *testing.T) {
	downloads := []attachmentDownloadOutput{
		{
			MessageID: "m1",
			attachmentOutput: attachmentOutput{
				Filename:     "a.txt",
				Size:         100,
				SizeHuman:    "100 B",
				MimeType:     "text/plain",
				AttachmentID: "a1",
			},
			Path:   "/tmp/a.txt",
			Cached: false,
		},
		{
			MessageID: "m2",
			attachmentOutput: attachmentOutput{
				Filename:     "b.pdf",
				Size:         2048,
				SizeHuman:    "2.0 KB",
				MimeType:     "application/pdf",
				AttachmentID: "b2",
			},
			Path:   "/tmp/b.pdf",
			Cached: true,
		},
	}
	result := attachmentOutputsFromDownloads(downloads)

	if len(result) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(result))
	}
	if result[0].Filename != "a.txt" || result[1].Filename != "b.pdf" {
		t.Fatalf("unexpected filenames: %v", result)
	}
	// Verify it extracts the embedded attachmentOutput correctly
	if result[0].Size != 100 || result[1].Size != 2048 {
		t.Fatalf("unexpected sizes: %v", result)
	}
}

func TestAttachmentDownloadOutputsFromInfo_Empty(t *testing.T) {
	result := attachmentDownloadOutputsFromInfo("m1", nil)
	if result != nil {
		t.Fatalf("expected nil for empty input, got %v", result)
	}

	result = attachmentDownloadOutputsFromInfo("m1", []attachmentInfo{})
	if result != nil {
		t.Fatalf("expected nil for empty slice, got %v", result)
	}
}

func TestAttachmentDownloadOutputsFromInfo_Multiple(t *testing.T) {
	infos := []attachmentInfo{
		{Filename: "a.txt", Size: 100, MimeType: "text/plain", AttachmentID: "a1"},
		{Filename: "b.pdf", Size: 2048, MimeType: "application/pdf", AttachmentID: "b2"},
	}
	result := attachmentDownloadOutputsFromInfo("msg123", infos)

	if len(result) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(result))
	}
	if result[0].MessageID != "msg123" || result[1].MessageID != "msg123" {
		t.Fatalf("expected message ID msg123, got %v", result)
	}
	if result[0].Filename != "a.txt" || result[1].Filename != "b.pdf" {
		t.Fatalf("unexpected filenames: %v", result)
	}
}

func TestAttachmentDownloadSummaries_Empty(t *testing.T) {
	result := attachmentDownloadSummaries(nil)
	if result != nil {
		t.Fatalf("expected nil for empty input, got %v", result)
	}

	result = attachmentDownloadSummaries([]attachmentDownloadOutput{})
	if result != nil {
		t.Fatalf("expected nil for empty slice, got %v", result)
	}
}

func TestAttachmentDownloadSummaries_Multiple(t *testing.T) {
	downloads := []attachmentDownloadOutput{
		{
			MessageID: "m1",
			attachmentOutput: attachmentOutput{
				Filename:     "a.txt",
				Size:         100,
				SizeHuman:    "100 B",
				MimeType:     "text/plain",
				AttachmentID: "a1",
			},
			Path:   "/tmp/a.txt",
			Cached: false,
		},
		{
			MessageID: "m2",
			attachmentOutput: attachmentOutput{
				Filename:     "b.pdf",
				Size:         2048,
				SizeHuman:    "2.0 KB",
				MimeType:     "application/pdf",
				AttachmentID: "b2",
			},
			Path:   "/tmp/b.pdf",
			Cached: true,
		},
	}
	result := attachmentDownloadSummaries(downloads)

	if len(result) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(result))
	}
	if result[0].MessageID != "m1" || result[1].MessageID != "m2" {
		t.Fatalf("unexpected message IDs: %v", result)
	}
	if result[0].Path != "/tmp/a.txt" || result[1].Path != "/tmp/b.pdf" {
		t.Fatalf("unexpected paths: %v", result)
	}
	if result[0].Cached != false || result[1].Cached != true {
		t.Fatalf("unexpected cached flags: %v", result)
	}
}

func TestAttachmentDownloadDraftOutputs_Empty(t *testing.T) {
	result := attachmentDownloadDraftOutputs(nil)
	if result != nil {
		t.Fatalf("expected nil for empty input, got %v", result)
	}

	result = attachmentDownloadDraftOutputs([]attachmentDownloadOutput{})
	if result != nil {
		t.Fatalf("expected nil for empty slice, got %v", result)
	}
}

func TestAttachmentDownloadDraftOutputs_Multiple(t *testing.T) {
	downloads := []attachmentDownloadOutput{
		{
			MessageID: "m1",
			attachmentOutput: attachmentOutput{
				Filename:     "a.txt",
				Size:         100,
				SizeHuman:    "100 B",
				MimeType:     "text/plain",
				AttachmentID: "a1",
			},
			Path:   "/tmp/a.txt",
			Cached: false,
		},
		{
			MessageID: "m2",
			attachmentOutput: attachmentOutput{
				Filename:     "b.pdf",
				Size:         2048,
				SizeHuman:    "2.0 KB",
				MimeType:     "application/pdf",
				AttachmentID: "b2",
			},
			Path:   "/tmp/b.pdf",
			Cached: true,
		},
	}
	result := attachmentDownloadDraftOutputs(downloads)

	if len(result) != 2 {
		t.Fatalf("expected 2 draft outputs, got %d", len(result))
	}
	if result[0].MessageID != "m1" || result[1].MessageID != "m2" {
		t.Fatalf("unexpected message IDs: %v", result)
	}
	if result[0].Filename != "a.txt" || result[1].Filename != "b.pdf" {
		t.Fatalf("unexpected filenames: %v", result)
	}
}

func TestAttachmentLine_Advanced(t *testing.T) {
	out := attachmentOutput{
		Filename:     "test.pdf",
		SizeHuman:    "1.0 KB",
		MimeType:     "application/pdf",
		AttachmentID: "att123",
	}
	line := attachmentLine(out)
	expected := "attachment\ttest.pdf\t1.0 KB\tapplication/pdf\tatt123"
	if line != expected {
		t.Fatalf("expected %q, got %q", expected, line)
	}
}

func TestFormatBytes_EdgeCases(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1536, "1.5 KB"},
		{1024*1024*1024 + 512*1024*1024, "1.5 GB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, result, tt.expected)
		}
	}
}

func TestCollectAttachments_Nil(t *testing.T) {
	result := collectAttachments(nil)
	if result != nil {
		t.Fatalf("expected nil for nil input, got %v", result)
	}
}

func TestCollectAttachments_NoAttachments(t *testing.T) {
	part := &gmail.MessagePart{
		MimeType: "text/plain",
		Body:     &gmail.MessagePartBody{Data: "SGVsbG8="},
	}
	result := collectAttachments(part)
	if len(result) != 0 {
		t.Fatalf("expected 0 attachments, got %d", len(result))
	}
}

func TestCollectAttachments_SingleAttachment(t *testing.T) {
	part := &gmail.MessagePart{
		MimeType: "application/pdf",
		Filename: "document.pdf",
		Body: &gmail.MessagePartBody{
			AttachmentId: "att123",
			Size:         1024,
		},
	}
	result := collectAttachments(part)
	if len(result) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(result))
	}
	if result[0].Filename != "document.pdf" {
		t.Fatalf("expected document.pdf, got %s", result[0].Filename)
	}
	if result[0].AttachmentID != "att123" {
		t.Fatalf("expected att123, got %s", result[0].AttachmentID)
	}
}

func TestCollectAttachments_EmptyFilename(t *testing.T) {
	part := &gmail.MessagePart{
		MimeType: "application/pdf",
		Filename: "",
		Body: &gmail.MessagePartBody{
			AttachmentId: "att123",
			Size:         1024,
		},
	}
	result := collectAttachments(part)
	if len(result) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(result))
	}
	if result[0].Filename != "attachment" {
		t.Fatalf("expected default 'attachment', got %s", result[0].Filename)
	}
}

func TestCollectAttachments_WhitespaceFilename(t *testing.T) {
	part := &gmail.MessagePart{
		MimeType: "application/pdf",
		Filename: "   ",
		Body: &gmail.MessagePartBody{
			AttachmentId: "att123",
			Size:         1024,
		},
	}
	result := collectAttachments(part)
	if len(result) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(result))
	}
	if result[0].Filename != "attachment" {
		t.Fatalf("expected default 'attachment', got %s", result[0].Filename)
	}
}

func TestCollectAttachments_NestedParts(t *testing.T) {
	part := &gmail.MessagePart{
		MimeType: "multipart/mixed",
		Parts: []*gmail.MessagePart{
			{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: "SGVsbG8="},
			},
			{
				MimeType: "application/pdf",
				Filename: "doc1.pdf",
				Body: &gmail.MessagePartBody{
					AttachmentId: "att1",
					Size:         1024,
				},
			},
			{
				MimeType: "multipart/alternative",
				Parts: []*gmail.MessagePart{
					{
						MimeType: "image/png",
						Filename: "image.png",
						Body: &gmail.MessagePartBody{
							AttachmentId: "att2",
							Size:         2048,
						},
					},
				},
			},
		},
	}
	result := collectAttachments(part)
	if len(result) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(result))
	}
	filenames := make(map[string]bool)
	for _, a := range result {
		filenames[a.Filename] = true
	}
	if !filenames["doc1.pdf"] || !filenames["image.png"] {
		t.Fatalf("expected doc1.pdf and image.png, got %v", result)
	}
}

// ==================== gmail.go helper tests ====================

func TestHasHeaderName(t *testing.T) {
	headers := []string{"From", "To", "Subject"}

	if !hasHeaderName(headers, "from") {
		t.Fatalf("expected to find 'from'")
	}
	if !hasHeaderName(headers, "FROM") {
		t.Fatalf("expected to find 'FROM' (case insensitive)")
	}
	if !hasHeaderName(headers, "To") {
		t.Fatalf("expected to find 'To'")
	}
	if hasHeaderName(headers, "Bcc") {
		t.Fatalf("expected not to find 'Bcc'")
	}
	if hasHeaderName(nil, "From") {
		t.Fatalf("expected not to find in nil slice")
	}
	if hasHeaderName([]string{}, "From") {
		t.Fatalf("expected not to find in empty slice")
	}
	// Test with whitespace
	if !hasHeaderName([]string{" From "}, "From") {
		t.Fatalf("expected to find 'From' with trimmed whitespace")
	}
}

func TestParseListUnsubscribe_Empty(t *testing.T) {
	result := parseListUnsubscribe("")
	if result != nil {
		t.Fatalf("expected nil for empty string, got %v", result)
	}

	result = parseListUnsubscribe("   ")
	if result != nil {
		t.Fatalf("expected nil for whitespace-only string, got %v", result)
	}
}

func TestParseListUnsubscribe_SingleHTTPS(t *testing.T) {
	result := parseListUnsubscribe("<https://example.com/unsub>")
	if len(result) != 1 || result[0] != "https://example.com/unsub" {
		t.Fatalf("expected single HTTPS link, got %v", result)
	}
}

func TestParseListUnsubscribe_SingleMailto(t *testing.T) {
	result := parseListUnsubscribe("<mailto:unsub@example.com>")
	if len(result) != 1 || result[0] != "mailto:unsub@example.com" {
		t.Fatalf("expected single mailto link, got %v", result)
	}
}

func TestParseListUnsubscribe_Multiple(t *testing.T) {
	result := parseListUnsubscribe("<mailto:unsub@example.com>, <https://example.com/unsub>")
	if len(result) != 2 {
		t.Fatalf("expected 2 links, got %d", len(result))
	}
}

func TestParseListUnsubscribe_NoBrackets(t *testing.T) {
	result := parseListUnsubscribe("https://example.com/unsub, mailto:unsub@example.com")
	if len(result) != 2 {
		t.Fatalf("expected 2 links, got %d: %v", len(result), result)
	}
}

func TestParseListUnsubscribe_InvalidLinks(t *testing.T) {
	result := parseListUnsubscribe("not a link, also not a link")
	if result != nil && len(result) != 0 {
		t.Fatalf("expected no valid links, got %v", result)
	}
}

func TestParseListUnsubscribe_Deduplication(t *testing.T) {
	result := parseListUnsubscribe("<https://example.com/unsub>, <https://example.com/unsub>")
	if len(result) != 1 {
		t.Fatalf("expected 1 deduplicated link, got %d", len(result))
	}
}

func TestIsUnsubscribeLink(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"https://example.com/unsub", true},
		{"http://example.com/unsub", true},
		{"mailto:unsub@example.com", true},
		{"HTTPS://EXAMPLE.COM/UNSUB", true},
		{"HTTP://EXAMPLE.COM/UNSUB", true},
		{"MAILTO:UNSUB@EXAMPLE.COM", true},
		{"ftp://example.com/unsub", false},
		{"not a link", false},
		{"", false},
		{"   ", false},
	}

	for _, tt := range tests {
		result := isUnsubscribeLink(tt.input)
		if result != tt.expected {
			t.Errorf("isUnsubscribeLink(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

// ==================== gmail_thread.go helper tests ====================

func TestStripHTMLTags_Advanced(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>Hello</p>", "Hello"},
		{"<div><p>Hello</p></div>", "Hello"},
		{"<script>alert('x')</script>Hello", "Hello"},
		{"<style>.class{color:red}</style>Hello", "Hello"},
		{"Hello   World", "Hello World"},
		{"<a href='test'>Link</a>", "Link"},
		{"", ""},
	}

	for _, tt := range tests {
		result := stripHTMLTags(tt.input)
		if result != tt.expected {
			t.Errorf("stripHTMLTags(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestBestBodyText_Nil(t *testing.T) {
	result := bestBodyText(nil)
	if result != "" {
		t.Fatalf("expected empty string for nil, got %q", result)
	}
}

func TestBestBodyForDisplay_Nil(t *testing.T) {
	body, isHTML := bestBodyForDisplay(nil)
	if body != "" || isHTML {
		t.Fatalf("expected empty string and false for nil, got %q, %v", body, isHTML)
	}
}

func TestMimeTypeMatches_Advanced(t *testing.T) {
	tests := []struct {
		partType string
		want     string
		expected bool
	}{
		{"text/plain", "text/plain", true},
		{"TEXT/PLAIN", "text/plain", true},
		{"text/plain; charset=utf-8", "text/plain", true},
		{"text/html", "text/plain", false},
		{"", "", true},
		{"", "text/plain", false},
	}

	for _, tt := range tests {
		result := mimeTypeMatches(tt.partType, tt.want)
		if result != tt.expected {
			t.Errorf("mimeTypeMatches(%q, %q) = %v, want %v", tt.partType, tt.want, result, tt.expected)
		}
	}
}

func TestNormalizeMimeType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"text/plain", "text/plain"},
		{"TEXT/PLAIN", "text/plain"},
		{"text/plain; charset=utf-8", "text/plain"},
		{"text/html;charset=ISO-8859-1", "text/html"},
		{"", ""},
		{"  text/plain  ", "text/plain"},
	}

	for _, tt := range tests {
		result := normalizeMimeType(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeMimeType(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestLooksLikeHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"<!DOCTYPE html>", true},
		{"<!doctype html>", true},
		{"<html>", true},
		{"<HTML>", true},
		{"<head>", true},
		{"<body>", true},
		{"<meta>", true},
		{"Hello World", false},
		{"<p>Hello</p>", false}, // Not a clear HTML document marker
		{"", false},
		{"   ", false},
		{"Some text with <html> embedded", true},
	}

	for _, tt := range tests {
		result := looksLikeHTML(tt.input)
		if result != tt.expected {
			t.Errorf("looksLikeHTML(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestLooksLikeBase64(t *testing.T) {
	tests := []struct {
		input    []byte
		expected bool
	}{
		{[]byte("SGVsbG8gV29ybGQ="), true},                 // "Hello World" base64 encoded
		{[]byte("SGVsbG8gV29ybGQ"), true},                  // Without padding
		{[]byte("SGVs bG8g V29y bGQ="), true},              // With spaces
		{[]byte("SGVs\nbG8g\nV29y\nbGQ="), true},           // With newlines
		{[]byte("!@#$%^&*()"), false},                      // Special characters
		{[]byte("Hello World with special chars!"), false}, // Regular text
		{[]byte(""), false},
		{[]byte("   "), false},
	}

	for _, tt := range tests {
		result := looksLikeBase64(tt.input)
		if result != tt.expected {
			t.Errorf("looksLikeBase64(%q) = %v, want %v", string(tt.input), result, tt.expected)
		}
	}
}

func TestDecodeAnyBase64(t *testing.T) {
	input := "SGVsbG8gV29ybGQ=" // "Hello World" in standard base64
	result, err := decodeAnyBase64([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != "Hello World" {
		t.Fatalf("expected 'Hello World', got %q", string(result))
	}

	// Test raw URL encoding
	inputURLRaw := "SGVsbG8gV29ybGQ" // Without padding
	result, err = decodeAnyBase64([]byte(inputURLRaw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != "Hello World" {
		t.Fatalf("expected 'Hello World', got %q", string(result))
	}
}

func TestStripBase64Whitespace(t *testing.T) {
	input := []byte("SGVs bG8g\nV29y\tbGQ=")
	expected := []byte("SGVsbG8gV29ybGQ=")
	result := stripBase64Whitespace(input)
	if string(result) != string(expected) {
		t.Fatalf("expected %q, got %q", string(expected), string(result))
	}
}

func TestDecodeBase64URL_Advanced(t *testing.T) {
	// Test standard base64 URL encoding
	input := base64.RawURLEncoding.EncodeToString([]byte("Hello World"))
	result, err := decodeBase64URL(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello World" {
		t.Fatalf("expected 'Hello World', got %q", result)
	}
}

func TestDecodeBase64URLBytes(t *testing.T) {
	// Test with padding
	input := base64.URLEncoding.EncodeToString([]byte("Hello World"))
	result, err := decodeBase64URLBytes(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != "Hello World" {
		t.Fatalf("expected 'Hello World', got %q", string(result))
	}

	// Test without padding
	inputRaw := base64.RawURLEncoding.EncodeToString([]byte("Hello World"))
	result, err = decodeBase64URLBytes(inputRaw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != "Hello World" {
		t.Fatalf("expected 'Hello World', got %q", string(result))
	}
}

// ==================== Command execution tests ====================

func TestGmailThreadGetCmd_EmptyThreadID(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	// No server needed since validation happens before API call
	newGmailService = func(context.Context, string) (*gmail.Service, error) {
		t.Fatalf("should not create service for empty thread ID")
		return nil, nil
	}

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailThreadGetCmd{ThreadID: ""}
	err := runKong(t, cmd, []string{""}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty threadId") {
		t.Fatalf("expected empty threadId error, got: %v", err)
	}
}

func TestGmailThreadModifyCmd_EmptyThreadID(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	newGmailService = func(context.Context, string) (*gmail.Service, error) {
		t.Fatalf("should not create service for empty thread ID")
		return nil, nil
	}

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailThreadModifyCmd{ThreadID: "", Add: "INBOX"}
	err := runKong(t, cmd, []string{"", "--add", "INBOX"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty threadId") {
		t.Fatalf("expected empty threadId error, got: %v", err)
	}
}

func TestGmailThreadModifyCmd_NoLabels(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	newGmailService = func(context.Context, string) (*gmail.Service, error) {
		t.Fatalf("should not create service when no labels specified")
		return nil, nil
	}

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailThreadModifyCmd{ThreadID: "t1"}
	err := runKong(t, cmd, []string{"t1"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "--add and/or --remove") {
		t.Fatalf("expected add/remove error, got: %v", err)
	}
}

func TestGmailThreadAttachmentsCmd_EmptyThreadID(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	newGmailService = func(context.Context, string) (*gmail.Service, error) {
		t.Fatalf("should not create service for empty thread ID")
		return nil, nil
	}

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailThreadAttachmentsCmd{ThreadID: ""}
	err := runKong(t, cmd, []string{""}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "empty threadId") {
		t.Fatalf("expected empty threadId error, got: %v", err)
	}
}

func TestGmailAttachmentCmd_MissingArgs(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	newGmailService = func(context.Context, string) (*gmail.Service, error) {
		t.Fatalf("should not create service for missing args")
		return nil, nil
	}

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailAttachmentCmd{MessageID: "", AttachmentID: ""}
	err := runKong(t, cmd, []string{"", ""}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "messageId/attachmentId required") {
		t.Fatalf("expected required args error, got: %v", err)
	}
}

func TestGmailURLCmd_JSON_MultipleThreads(t *testing.T) {
	flags := &RootFlags{Account: "test@example.com"}

	out := captureStdout(t, func() {
		ctx := testContextJSON(t)
		cmd := &GmailURLCmd{ThreadIDs: []string{"t1", "t2"}}
		if err := runKong(t, cmd, []string{"t1", "t2"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		URLs []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"urls"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.URLs) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(parsed.URLs))
	}
	if parsed.URLs[0].ID != "t1" || parsed.URLs[1].ID != "t2" {
		t.Fatalf("unexpected IDs: %v", parsed.URLs)
	}
	if !strings.Contains(parsed.URLs[0].URL, "test%40example.com") {
		t.Fatalf("expected URL-encoded account in URL, got %s", parsed.URLs[0].URL)
	}
}

func TestGmailURLCmd_Text_Encoded(t *testing.T) {
	flags := &RootFlags{Account: "test@example.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &GmailURLCmd{ThreadIDs: []string{"t1"}}
		if err := runKong(t, cmd, []string{"t1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "t1") || !strings.Contains(out, "mail.google.com") {
		t.Fatalf("unexpected text output: %q", out)
	}
}

func TestGmailThreadAttachmentsCmd_EmptyThread_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "t1",
				"messages": []any{},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		ctx := testContextJSON(t)
		cmd := &GmailThreadAttachmentsCmd{}
		if err := runKong(t, cmd, []string{"t1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		ThreadID    string `json:"threadId"`
		Attachments []any  `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.ThreadID != "t1" {
		t.Fatalf("expected threadId t1, got %s", parsed.ThreadID)
	}
	if len(parsed.Attachments) != 0 {
		t.Fatalf("expected 0 attachments, got %d", len(parsed.Attachments))
	}
}

func TestGmailThreadAttachmentsCmd_WithAttachments_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":       "m1",
						"threadId": "t1",
						"payload": map[string]any{
							"mimeType": "multipart/mixed",
							"parts": []map[string]any{
								{
									"mimeType": "text/plain",
									"body":     map[string]any{"data": "SGVsbG8="},
								},
								{
									"mimeType": "application/pdf",
									"filename": "test.pdf",
									"body": map[string]any{
										"attachmentId": "att1",
										"size":         1024,
									},
								},
							},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		ctx := testContextJSON(t)
		cmd := &GmailThreadAttachmentsCmd{}
		if err := runKong(t, cmd, []string{"t1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		ThreadID    string `json:"threadId"`
		Attachments []struct {
			MessageID    string `json:"messageId"`
			Filename     string `json:"filename"`
			Size         int64  `json:"size"`
			AttachmentID string `json:"attachmentId"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.ThreadID != "t1" {
		t.Fatalf("expected threadId t1, got %s", parsed.ThreadID)
	}
	if len(parsed.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(parsed.Attachments))
	}
	if parsed.Attachments[0].Filename != "test.pdf" {
		t.Fatalf("expected filename test.pdf, got %s", parsed.Attachments[0].Filename)
	}
}

func TestGmailThreadAttachmentsCmd_Download_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	attachmentData := []byte("test data")
	attachmentEncoded := base64.RawURLEncoding.EncodeToString(attachmentData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":       "m1",
						"threadId": "t1",
						"payload": map[string]any{
							"mimeType": "multipart/mixed",
							"parts": []map[string]any{
								{
									"mimeType": "application/pdf",
									"filename": "download.pdf",
									"body": map[string]any{
										"attachmentId": "att1",
										"size":         int64(len(attachmentData)),
									},
								},
							},
						},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/att1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attachmentEncoded})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	outDir := t.TempDir()
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		ctx := testContextJSON(t)
		cmd := &GmailThreadAttachmentsCmd{Download: true, OutputDir: OutputDirFlag{Dir: outDir}}
		if err := runKong(t, cmd, []string{"t1", "--download", "--out-dir", outDir}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		ThreadID    string `json:"threadId"`
		Attachments []struct {
			Path   string `json:"path"`
			Cached bool   `json:"cached"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(parsed.Attachments))
	}
	if !strings.Contains(parsed.Attachments[0].Path, outDir) {
		t.Fatalf("expected path in %s, got %s", outDir, parsed.Attachments[0].Path)
	}

	// Verify file was written
	data, err := os.ReadFile(parsed.Attachments[0].Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(attachmentData) {
		t.Fatalf("expected %q, got %q", string(attachmentData), string(data))
	}
}

func TestGmailThreadGetCmd_JSON_EmptyThread(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "t1",
				"messages": []any{},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		ctx := testContextJSON(t)
		cmd := &GmailThreadGetCmd{}
		if err := runKong(t, cmd, []string{"t1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
		Downloaded []any `json:"downloaded"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.Thread.ID != "t1" {
		t.Fatalf("expected thread id t1, got %s", parsed.Thread.ID)
	}
}

func TestGmailThreadGetCmd_Text_WithMessages(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	bodyData := base64.RawURLEncoding.EncodeToString([]byte("Hello World"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":       "m1",
						"threadId": "t1",
						"payload": map[string]any{
							"mimeType": "text/plain",
							"headers": []map[string]any{
								{"name": "From", "value": "sender@example.com"},
								{"name": "To", "value": "recipient@example.com"},
								{"name": "Subject", "value": "Test Subject"},
								{"name": "Date", "value": "Mon, 02 Jan 2006 15:04:05 -0700"},
							},
							"body": map[string]any{"data": bodyData},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := &GmailThreadGetCmd{}
		if err := runKong(t, cmd, []string{"t1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "sender@example.com") {
		t.Fatalf("expected From in output: %q", out)
	}
	if !strings.Contains(out, "Test Subject") {
		t.Fatalf("expected Subject in output: %q", out)
	}
	if !strings.Contains(out, "Hello World") {
		t.Fatalf("expected body in output: %q", out)
	}
}

func TestGmailThreadModifyCmd_Success_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX"},
					{"id": "Label_123", "name": "MyLabel"},
					{"id": "Label_456", "name": "OldLabel"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/t1/modify") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		ctx := testContextJSON(t)
		cmd := &GmailThreadModifyCmd{}
		if err := runKong(t, cmd, []string{"t1", "--add", "MyLabel", "--remove", "OldLabel"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Modified      string   `json:"modified"`
		AddedLabels   []string `json:"addedLabels"`
		RemovedLabels []string `json:"removedLabels"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.Modified != "t1" {
		t.Fatalf("expected modified t1, got %s", parsed.Modified)
	}
}

// ==================== downloadAttachment tests ====================

func TestDownloadAttachment_MissingMessageID(t *testing.T) {
	_, _, err := downloadAttachment(context.Background(), nil, "", attachmentInfo{AttachmentID: "a1"}, ".")
	if err == nil || !strings.Contains(err.Error(), "missing messageID/attachmentID") {
		t.Fatalf("expected missing messageID error, got: %v", err)
	}
}

func TestDownloadAttachment_MissingAttachmentID(t *testing.T) {
	_, _, err := downloadAttachment(context.Background(), nil, "m1", attachmentInfo{AttachmentID: ""}, ".")
	if err == nil || !strings.Contains(err.Error(), "missing messageID/attachmentID") {
		t.Fatalf("expected missing attachmentID error, got: %v", err)
	}
}

func TestDownloadAttachment_DefaultDir(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	attachmentData := []byte("test attachment data")
	attachmentEncoded := base64.RawURLEncoding.EncodeToString(attachmentData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/att1") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attachmentEncoded})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Change to temp dir so "." works correctly
	origDir, _ := os.Getwd()
	tempDir := t.TempDir()
	_ = os.Chdir(tempDir)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	info := attachmentInfo{
		Filename:     "test.txt",
		AttachmentID: "att1",
		Size:         int64(len(attachmentData)),
	}
	path, cached, err := downloadAttachment(context.Background(), svc, "m1", info, "")
	if err != nil {
		t.Fatalf("downloadAttachment: %v", err)
	}
	if cached {
		t.Fatalf("expected not cached")
	}
	if !strings.Contains(path, "test.txt") {
		t.Fatalf("expected test.txt in path, got %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(attachmentData) {
		t.Fatalf("expected %q, got %q", string(attachmentData), string(data))
	}
}

func TestDownloadAttachment_PathTraversalPrevention(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	attachmentData := []byte("test")
	attachmentEncoded := base64.RawURLEncoding.EncodeToString(attachmentData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/att1") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attachmentEncoded})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	tempDir := t.TempDir()

	// Test path traversal attack filename
	info := attachmentInfo{
		Filename:     "../../../etc/passwd",
		AttachmentID: "att1",
		Size:         int64(len(attachmentData)),
	}
	path, _, err := downloadAttachment(context.Background(), svc, "m1", info, tempDir)
	if err != nil {
		t.Fatalf("downloadAttachment: %v", err)
	}
	// Verify the file was saved in the expected directory, not traversed
	if !strings.HasPrefix(path, tempDir) {
		t.Fatalf("expected path to be within %s, got %s", tempDir, path)
	}
	if strings.Contains(path, "..") {
		t.Fatalf("path should not contain path traversal: %s", path)
	}
}

func TestDownloadAttachment_InvalidFilenames(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	attachmentData := []byte("test")
	attachmentEncoded := base64.RawURLEncoding.EncodeToString(attachmentData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/att1") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": attachmentEncoded})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	tempDir := t.TempDir()

	tests := []struct {
		filename       string
		expectedInPath string
	}{
		{".", "attachment"},
		{"..", "attachment"},
		{"", "attachment"},
	}

	for _, tt := range tests {
		info := attachmentInfo{
			Filename:     tt.filename,
			AttachmentID: "att1",
			Size:         int64(len(attachmentData)),
		}
		path, _, err := downloadAttachment(context.Background(), svc, "m1", info, tempDir)
		if err != nil {
			t.Fatalf("downloadAttachment for %q: %v", tt.filename, err)
		}
		if !strings.Contains(filepath.Base(path), tt.expectedInPath) {
			t.Fatalf("expected %q in path for filename %q, got %s", tt.expectedInPath, tt.filename, path)
		}
	}
}
