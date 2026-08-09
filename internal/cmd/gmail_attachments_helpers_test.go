package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestCollectAttachments(t *testing.T) {
	part := &gmail.MessagePart{
		Parts: []*gmail.MessagePart{
			{
				Filename: "a.txt",
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{AttachmentId: "att1", Size: 123},
			},
			{
				MimeType: "image/png",
				Body:     &gmail.MessagePartBody{AttachmentId: "att-inline", Size: 42},
			},
			{
				Parts: []*gmail.MessagePart{
					{
						Filename: "b.pdf",
						MimeType: "application/pdf",
						Body:     &gmail.MessagePartBody{AttachmentId: "att2", Size: 456},
					},
				},
			},
		},
	}

	attachments := collectAttachments(part)
	if len(attachments) != 3 {
		t.Fatalf("unexpected: %#v", attachments)
	}
	if attachments[0].AttachmentID == "" || attachments[1].AttachmentID == "" {
		t.Fatalf("missing attachment ids: %#v", attachments)
	}
	if attachments[1].Filename != "attachment" {
		t.Fatalf("expected fallback filename, got: %#v", attachments[1])
	}
}

func TestCollectAttachmentsMore(t *testing.T) {
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

func TestAttachmentLine(t *testing.T) {
	attachment := attachmentOutput{
		Filename:     "file.txt",
		Size:         12,
		SizeHuman:    formatBytes(12),
		MimeType:     "text/plain",
		AttachmentID: "a1",
	}
	if got := attachmentLine(attachment); got != "attachment\tfile.txt\t12 B\ttext/plain\ta1" {
		t.Fatalf("unexpected attachment line: %q", got)
	}
}

func TestDownloadAttachmentCached(t *testing.T) {
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
	gotPath, cached, err := downloadAttachment(context.Background(), nil, messageID, info, dir, false)
	if err != nil {
		t.Fatalf("downloadAttachment: %v", err)
	}
	if !cached || gotPath != outPath {
		t.Fatalf("expected cached path %q, got %q cached=%v", outPath, gotPath, cached)
	}
}
