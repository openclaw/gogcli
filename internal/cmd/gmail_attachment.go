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

const defaultGmailAttachmentFilename = "attachment.bin"

func printAttachmentDownloadResult(ctx context.Context, u *ui.UI, path string, cached bool, bytes int64) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"path": path, "cached": cached, "bytes": bytes})
	}
	u.Out().Printf("path\t%s", path)
	u.Out().Printf("cached\t%t", cached)
	u.Out().Printf("bytes\t%d", bytes)
	return nil
}

func (c *GmailAttachmentCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	messageID := normalizeGmailMessageID(c.MessageID)
	attachmentID := strings.TrimSpace(c.AttachmentID)
	if messageID == "" || attachmentID == "" {
		return usage("messageId/attachmentId required")
	}

	destPath, err := resolveAttachmentOutputPath(messageID, attachmentID, c.Output.Path, c.Name, false)
	if err != nil {
		return err
	}

	// Avoid touching auth/keyring and avoid writing files in dry-run mode.
	if dryRunErr := dryRunExit(ctx, flags, "gmail.attachment.download", map[string]any{
		"message_id":    messageID,
		"attachment_id": attachmentID,
		"path":          destPath,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	destPath, err = resolveAttachmentOutputPath(messageID, attachmentID, c.Output.Path, c.Name, true)
	if err != nil {
		return err
	}

	expectedSize := int64(-1)
	if st, statErr := os.Stat(destPath); statErr == nil && st.Mode().IsRegular() {
		// Only hit messages.get when we might have a cache-hit candidate.
		expectedSize = lookupAttachmentSizeEstimate(ctx, svc, messageID, attachmentID)
	}
	path, cached, bytes, err := downloadAttachmentToPath(ctx, svc, messageID, attachmentID, destPath, expectedSize)
	if err != nil {
		return err
	}
	return printAttachmentDownloadResult(ctx, u, path, cached, bytes)
}

func resolveAttachmentOutputPath(messageID, attachmentID, outPathFlag, name string, ensureDefaultDir bool) (string, error) {
	shortID := attachmentID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	safeFilename := sanitizeAttachmentFilename(name, defaultGmailAttachmentFilename)

	if strings.TrimSpace(outPathFlag) == "" {
		var dir string
		var err error
		if ensureDefaultDir {
			dir, err = config.EnsureGmailAttachmentsDir()
		} else {
			dir, err = config.GmailAttachmentsDir()
		}
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, fmt.Sprintf("%s_%s_%s", messageID, shortID, safeFilename)), nil
	}

	outPath, err := config.ExpandPath(outPathFlag)
	if err != nil {
		return "", err
	}

	// Directory intent:
	// - existing directory path
	// - or explicit trailing slash for a (possibly non-existent) directory
	isDir := strings.HasSuffix(strings.TrimSpace(outPathFlag), string(os.PathSeparator)) ||
		strings.HasSuffix(outPathFlag, "/") ||
		strings.HasSuffix(outPathFlag, "\\")
	if !isDir {
		if st, statErr := os.Stat(outPath); statErr == nil && st.IsDir() {
			isDir = true
		}
	}
	if isDir {
		filename := safeFilename
		if strings.TrimSpace(name) == "" {
			filename = fmt.Sprintf("%s_%s_attachment.bin", messageID, shortID)
		}
		return filepath.Join(outPath, filename), nil
	}

	return outPath, nil
}

func sanitizeAttachmentFilename(name, fallback string) string {
	// Normalize Windows-style separators too; prevents "..\\..\\x" escapes when treating `--name` as a filename.
	clean := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	safeFilename := filepath.Base(clean)
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

	if st, err := os.Stat(outPath); err == nil {
		if st.IsDir() {
			return "", false, 0, fmt.Errorf("outPath is a directory: %s", outPath)
		}
		if st.Mode().IsRegular() && expectedSize > 0 && st.Size() == expectedSize {
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
