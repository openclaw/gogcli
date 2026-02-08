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
	Name         string         `name:"name" help:"Filename (only used when --out is empty)"`
}

const defaultGmailAttachmentFilename = "attachment.bin"

func (c *GmailAttachmentCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	messageID := normalizeGmailMessageID(c.MessageID)
	attachmentID := strings.TrimSpace(c.AttachmentID)
	if messageID == "" || attachmentID == "" {
		return usage("messageId/attachmentId required")
	}

	outPathFlag := strings.TrimSpace(c.Output.Path)
	var destPath string
	if outPathFlag == "" {
		dir, dirErr := config.GmailAttachmentsDir()
		if dirErr != nil {
			return dirErr
		}
		filename := strings.TrimSpace(c.Name)
		if filename == "" {
			filename = defaultGmailAttachmentFilename
		}
		safeFilename := filepath.Base(filename)
		if safeFilename == "" || safeFilename == "." || safeFilename == ".." {
			safeFilename = defaultGmailAttachmentFilename
		}
		shortID := attachmentID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		destPath = filepath.Join(dir, fmt.Sprintf("%s_%s_%s", messageID, shortID, safeFilename))
	} else {
		outPath, err := config.ExpandPath(outPathFlag)
		if err != nil {
			return err
		}
		destPath = outPath
	}

	// Avoid touching auth/keyring and avoid writing files in dry-run mode.
	if err := dryRunExit(ctx, flags, "gmail.attachment.download", map[string]any{
		"message_id":    messageID,
		"attachment_id": attachmentID,
		"path":          destPath,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.Output.Path) == "" {
		dir, dirErr := config.EnsureGmailAttachmentsDir()
		if dirErr != nil {
			return dirErr
		}
		filename := strings.TrimSpace(c.Name)
		if filename == "" {
			filename = defaultGmailAttachmentFilename
		}
		safeFilename := filepath.Base(filename)
		if safeFilename == "" || safeFilename == "." || safeFilename == ".." {
			safeFilename = defaultGmailAttachmentFilename
		}
		shortID := attachmentID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		destPath := filepath.Join(dir, fmt.Sprintf("%s_%s_%s", messageID, shortID, safeFilename))
		path, cached, bytes, dlErr := downloadAttachmentToPath(ctx, svc, messageID, attachmentID, destPath, -1)
		if dlErr != nil {
			return dlErr
		}
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"path": path, "cached": cached, "bytes": bytes})
		}
		u.Out().Printf("path\t%s", path)
		u.Out().Printf("cached\t%t", cached)
		u.Out().Printf("bytes\t%d", bytes)
		return nil
	}

	outPath, err := config.ExpandPath(c.Output.Path)
	if err != nil {
		return err
	}
	path, cached, bytes, err := downloadAttachmentToPath(ctx, svc, messageID, attachmentID, outPath, -1)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"path": path, "cached": cached, "bytes": bytes})
	}
	u.Out().Printf("path\t%s", path)
	u.Out().Printf("cached\t%t", cached)
	u.Out().Printf("bytes\t%d", bytes)
	return nil
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

	if expectedSize > 0 {
		if st, err := os.Stat(outPath); err == nil && st.Size() == expectedSize {
			return outPath, true, st.Size(), nil
		}
	} else if expectedSize == -1 {
		if st, err := os.Stat(outPath); err == nil && st.Size() > 0 {
			return outPath, true, st.Size(), nil
		}
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
