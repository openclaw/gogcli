package cmd

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestStripHTMLTags_More(t *testing.T) {
	input := "<div>Hello <b>World</b><script>bad()</script><style>.x{}</style></div>"
	out := stripHTMLTags(input)
	if out != "Hello World" {
		t.Fatalf("unexpected stripped output: %q", out)
	}
}

func TestFormatBytes(t *testing.T) {
	if got := formatBytes(500); got != "500 B" {
		t.Fatalf("unexpected bytes format: %q", got)
	}
	if got := formatBytes(2048); got != "2.0 KB" {
		t.Fatalf("unexpected KB format: %q", got)
	}
	if got := formatBytes(5 * 1024 * 1024); got != "5.0 MB" {
		t.Fatalf("unexpected MB format: %q", got)
	}
	if got := formatBytes(3 * 1024 * 1024 * 1024); got != "3.0 GB" {
		t.Fatalf("unexpected GB format: %q", got)
	}
}

func TestCollectAttachments_More(t *testing.T) {
	part := &gmail.MessagePart{
		Parts: []*gmail.MessagePart{
			{
				Filename: "file.txt",
				MimeType: "text/plain",
				Body: &gmail.MessagePartBody{
					AttachmentId: "a1",
					Size:         12,
				},
			},
			{
				Parts: []*gmail.MessagePart{
					{
						MimeType: "image/png",
						Body: &gmail.MessagePartBody{
							AttachmentId: "a2",
							Size:         34,
						},
					},
				},
			},
		},
	}
	attachments := collectAttachments(part)
	if len(attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(attachments))
	}
	if attachments[0].Filename != "file.txt" || attachments[1].AttachmentID != "a2" {
		t.Fatalf("unexpected attachments: %#v", attachments)
	}
}

func TestAttachmentLine(t *testing.T) {
	att := attachmentOutput{
		Filename:     "file.txt",
		Size:         12,
		SizeHuman:    formatBytes(12),
		MimeType:     "text/plain",
		AttachmentID: "a1",
	}
	if got := attachmentLine(att); got != "attachment\tfile.txt\t12 B\ttext/plain\ta1" {
		t.Fatalf("unexpected attachment line: %q", got)
	}
}

func TestBestBodySelection(t *testing.T) {
	plain := base64.RawURLEncoding.EncodeToString([]byte("plain"))
	html := base64.RawURLEncoding.EncodeToString([]byte("<b>html</b>"))
	part := &gmail.MessagePart{
		Parts: []*gmail.MessagePart{
			{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: plain},
			},
			{
				MimeType: "text/html",
				Body:     &gmail.MessagePartBody{Data: html},
			},
		},
	}
	if got := bestBodyText(part); got != "plain" {
		t.Fatalf("unexpected best body text: %q", got)
	}
	body, isHTML := bestBodyForDisplay(part)
	if body != "plain" || isHTML {
		t.Fatalf("unexpected body display: %q html=%v", body, isHTML)
	}
}

func TestFindPartBodyHTML(t *testing.T) {
	html := base64.RawURLEncoding.EncodeToString([]byte("<p>hi</p>"))
	part := &gmail.MessagePart{
		MimeType: "multipart/alternative",
		Parts: []*gmail.MessagePart{
			{
				MimeType: "text/html; charset=UTF-8",
				Body:     &gmail.MessagePartBody{Data: html},
			},
		},
	}
	got := findPartBody(part, "text/html")
	if got != "<p>hi</p>" {
		t.Fatalf("unexpected html body: %q", got)
	}
}

func TestBestBodyForDisplay_DetectsHTMLInPlainPart(t *testing.T) {
	html := base64.RawURLEncoding.EncodeToString([]byte("<html><body>hi</body></html>"))
	part := &gmail.MessagePart{
		Parts: []*gmail.MessagePart{
			{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: html},
			},
		},
	}
	body, isHTML := bestBodyForDisplay(part)
	if body == "" || !isHTML {
		t.Fatalf("expected HTML detection, got body=%q html=%v", body, isHTML)
	}
}

func TestFindPartBody_DecodesQuotedPrintable(t *testing.T) {
	qp := "Precio =E2=82=AC99.99"
	encoded := base64.RawURLEncoding.EncodeToString([]byte(qp))
	part := &gmail.MessagePart{
		MimeType: "text/plain",
		Headers: []*gmail.MessagePartHeader{
			{Name: "Content-Transfer-Encoding", Value: "quoted-printable"},
			{Name: "Content-Type", Value: "text/plain; charset=utf-8"},
		},
		Body: &gmail.MessagePartBody{Data: encoded},
	}
	got := findPartBody(part, "text/plain")
	if got != "Precio €99.99" {
		t.Fatalf("unexpected decoded body: %q", got)
	}
}

func TestFindPartBody_PreservesURLsWhenAlreadyDecoded(t *testing.T) {
	// Gmail API sometimes returns already-decoded content even when
	// Content-Transfer-Encoding header says quoted-printable.
	// URLs with = should be preserved, not corrupted to U+FFFD.
	// See: https://github.com/steipete/gogcli/issues/159
	url := "https://example.com/auth?token_hash=abc123&type=magiclink"
	encoded := base64.RawURLEncoding.EncodeToString([]byte(url))
	part := &gmail.MessagePart{
		MimeType: "text/plain",
		Headers: []*gmail.MessagePartHeader{
			{Name: "Content-Transfer-Encoding", Value: "quoted-printable"},
			{Name: "Content-Type", Value: "text/plain; charset=utf-8"},
		},
		Body: &gmail.MessagePartBody{Data: encoded},
	}
	got := findPartBody(part, "text/plain")
	if got != url {
		t.Fatalf("URL corrupted: expected %q, got %q", url, got)
	}
}

func TestLooksLikeQuotedPrintable(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"actual QP uppercase hex", "Price =E2=82=AC99", true},
		{"soft line break CRLF", "line=\r\ncontinued", true},
		{"soft line break LF", "line=\ncontinued", true},
		{"plain URL lowercase", "https://example.com?foo=bar", false},
		{"URL with multiple params", "https://example.com?a=b1&c=d2", false},
		{"lowercase hex sequence", "test=ab", false}, // lowercase not matched
		{"uppercase hex sequence", "test=AB", true},
		{"mixed case hex", "test=Ab", false}, // requires both uppercase
		{"plain text", "Hello World", false},
		{"equals at end", "foo=", false},
		{"short input", "=", false},
		{"QP encoded equals", "foo=3Dbar", true}, // =3D is QP for =
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeQuotedPrintable([]byte(tt.input))
			if got != tt.want {
				t.Errorf("looksLikeQuotedPrintable(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindPartBody_DecodesBase64Transfer(t *testing.T) {
	inner := base64.StdEncoding.EncodeToString([]byte("plain body"))
	encoded := base64.RawURLEncoding.EncodeToString([]byte(inner))
	part := &gmail.MessagePart{
		MimeType: "text/plain",
		Headers: []*gmail.MessagePartHeader{
			{Name: "Content-Transfer-Encoding", Value: "base64"},
			{Name: "Content-Type", Value: "text/plain; charset=utf-8"},
		},
		Body: &gmail.MessagePartBody{Data: encoded},
	}
	got := findPartBody(part, "text/plain")
	if got != "plain body" {
		t.Fatalf("unexpected decoded body: %q", got)
	}
}

func TestDecodeTransferEncoding_Base64Whitespace(t *testing.T) {
	encoded := []byte("cGxhaW4gYm9keQ==\n")
	got := decodeTransferEncoding(encoded, "base64")
	if string(got) != "plain body" {
		t.Fatalf("unexpected decoded body: %q", got)
	}
}

func TestDecodeBodyCharset_ISO88591(t *testing.T) {
	input := []byte{0x63, 0x61, 0x66, 0xe9} // "café" in ISO-8859-1
	got := decodeBodyCharset(input, "text/plain; charset=iso-8859-1")
	if string(got) != "café" {
		t.Fatalf("unexpected decoded charset: %q", string(got))
	}
}

func TestMimeTypeMatches(t *testing.T) {
	if !mimeTypeMatches("Text/Plain; charset=UTF-8", "text/plain") {
		t.Fatalf("expected mime match")
	}
	if mimeTypeMatches("application/json", "text/plain") {
		t.Fatalf("unexpected mime match")
	}
	if normalizeMimeType("text/plain; charset=utf-8") != "text/plain" {
		t.Fatalf("unexpected normalized mime type")
	}
	if normalizeMimeType("") != "" {
		t.Fatalf("expected empty normalized mime type")
	}
}

func TestDecodeBase64URL_Padded(t *testing.T) {
	encoded := base64.URLEncoding.EncodeToString([]byte("hello"))
	decoded, err := decodeBase64URL(encoded)
	if err != nil {
		t.Fatalf("decodeBase64URL: %v", err)
	}
	if decoded != "hello" {
		t.Fatalf("unexpected decode: %q", decoded)
	}
}

func TestDownloadAttachment_Cached(t *testing.T) {
	dir := t.TempDir()
	messageID := "msg1"
	attachmentID := "att123456"
	filename := "file.txt"
	shortID := attachmentID[:8]
	outPath := filepath.Join(dir, messageID+"_"+shortID+"_"+filename)

	if err := os.WriteFile(outPath, []byte("abc"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	info := attachmentInfo{
		Filename:     filename,
		AttachmentID: attachmentID,
		Size:         3,
	}
	gotPath, cached, err := downloadAttachment(context.Background(), nil, messageID, info, dir)
	if err != nil {
		t.Fatalf("downloadAttachment: %v", err)
	}
	if !cached || gotPath != outPath {
		t.Fatalf("expected cached path %q, got %q cached=%v", outPath, gotPath, cached)
	}
}
