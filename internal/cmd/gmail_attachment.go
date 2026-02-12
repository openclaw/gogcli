package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailAttachmentCmd struct {
	MessageID    string         `arg:"" name:"messageId" help:"Message ID"`
	AttachmentID string         `arg:"" name:"attachmentId" help:"Attachment ID"`
	Output       OutputPathFlag `embed:""`
	Name         string         `name:"name" help:"Filename (used when --out is empty or points to a directory)"`
}

func (c *GmailAttachmentCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	messageID := strings.TrimSpace(c.MessageID)
	attachmentID := strings.TrimSpace(c.AttachmentID)
	if messageID == "" || attachmentID == "" {
		return usage("messageId/attachmentId required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	destPath, err := resolveAttachmentOutputPath(messageID, attachmentID, c.Output.Path, c.Name)
	if err != nil {
		return err
	}
	expectedSize := lookupAttachmentSizeEstimate(ctx, svc, messageID, attachmentID)
	path, cached, bytes, err := downloadAttachmentToPath(ctx, svc, messageID, attachmentID, destPath, expectedSize)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"path": path, "cached": cached, "bytes": bytes})
	}
	u.Out().Printf("path\t%s", path)
	u.Out().Printf("cached\t%t", cached)
	u.Out().Printf("bytes\t%d", bytes)
	return nil
}

func resolveAttachmentOutputPath(messageID, attachmentID, outPathFlag, name string) (string, error) {
	shortID := attachmentID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	safeFilename := sanitizeAttachmentFilename(name, "attachment.bin")

	if strings.TrimSpace(outPathFlag) == "" {
		dir, err := config.EnsureGmailAttachmentsDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, fmt.Sprintf("%s_%s_%s", messageID, shortID, safeFilename)), nil
	}

	outPath, err := config.ExpandPath(outPathFlag)
	if err != nil {
		return "", err
	}

	if st, statErr := os.Stat(outPath); statErr == nil && st.IsDir() {
		filename := safeFilename
		if strings.TrimSpace(name) == "" {
			filename = fmt.Sprintf("%s_%s_attachment.bin", messageID, shortID)
		}
		return filepath.Join(outPath, filename), nil
	}

	// Treat paths ending with a separator as directory targets even if they don't exist yet.
	if outPath != "" && os.IsPathSeparator(outPath[len(outPath)-1]) {
		return filepath.Join(outPath, safeFilename), nil
	}

	return outPath, nil
}

func sanitizeAttachmentFilename(name, fallback string) string {
	safeFilename := filepath.Base(strings.TrimSpace(name))
	if safeFilename == "" || safeFilename == "." || safeFilename == ".." {
		return fallback
	}
	return safeFilename
}

func lookupAttachmentSizeEstimate(ctx context.Context, svc *gmail.Service, messageID, attachmentID string) int64 {
	if svc == nil {
		return -1
	}
	msg, err := svc.Users.Messages.Get("me", messageID).Format("full").Fields("payload").Context(ctx).Do()
	if err != nil || msg == nil {
		return -1
	}
	for _, a := range collectAttachments(msg.Payload) {
		if a.AttachmentID == attachmentID && a.Size > 0 {
			return a.Size
		}
	}
	return -1
}

func downloadAttachmentToPath(
	ctx context.Context,
	svc *gmail.Service,
	messageID string,
	attachmentID string,
	outPath string,
	expectedSize int64,
) (string, bool, int64, error) {
	if strings.TrimSpace(outPath) == "" {
		return "", false, 0, errors.New("missing outPath")
	}

	if st, err := os.Stat(outPath); err == nil && st.Mode().IsRegular() {
		if expectedSize > 0 && st.Size() == expectedSize {
			return outPath, true, st.Size(), nil
		}
	}

	if svc == nil {
		return "", false, 0, errors.New("missing gmail service")
	}

	body, err := svc.Users.Messages.Attachments.Get("me", messageID, attachmentID).Context(ctx).Do()
	if err != nil {
		return "", false, 0, err
	}
	if body == nil || body.Data == "" {
		return "", false, 0, errors.New("empty attachment data")
	}
	data, err := base64.RawURLEncoding.DecodeString(body.Data)
	if err != nil {
		// Gmail can return padded base64url; accept both.
		data, err = base64.URLEncoding.DecodeString(body.Data)
		if err != nil {
			return "", false, 0, err
		}
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o700); err != nil {
		return "", false, 0, err
	}
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		return "", false, 0, err
	}
	return outPath, false, int64(len(data)), nil
}
