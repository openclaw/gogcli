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

	dest, err := resolveAttachmentDest(messageID, attachmentID, c.Output.Path, c.Name, false)
	if err != nil {
		return err
	}

	// Avoid touching auth/keyring and avoid writing files in dry-run mode.
	if dryRunErr := dryRunExit(ctx, flags, "gmail.attachment.download", map[string]any{
		"message_id":    messageID,
		"attachment_id": attachmentID,
		"path":          dest.Path,
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

	dest, err = resolveAttachmentDest(messageID, attachmentID, c.Output.Path, c.Name, true)
	if err != nil {
		return err
	}
	if dest.EnsureDefaultDir {
		// Ensure the config dir exists (so permissions are correct) before we write under it.
		if _, ensureErr := config.EnsureGmailAttachmentsDir(); ensureErr != nil {
			return ensureErr
		}
	}

	expectedSize := int64(-1)
	if st, statErr := os.Stat(dest.Path); statErr == nil && st.Mode().IsRegular() {
		// Only hit messages.get when we might have a cache-hit candidate.
		expectedSize = lookupAttachmentSizeEstimate(ctx, svc, messageID, attachmentID)
	}
	path, cached, bytes, err := downloadAttachmentToPath(ctx, svc, messageID, attachmentID, dest.Path, expectedSize)
	if err != nil {
		return err
	}
	return printAttachmentDownloadResult(ctx, u, path, cached, bytes)
}

type attachmentDest struct {
	Path             string
	EnsureDefaultDir bool
}

func resolveAttachmentDest(messageID, attachmentID, outPathFlag, name string, allowEnsureDefaultDir bool) (attachmentDest, error) {
	shortID := attachmentID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	safeFilename := sanitizeAttachmentFilename(name, defaultGmailAttachmentFilename)

	if strings.TrimSpace(outPathFlag) == "" {
		dir, err := config.GmailAttachmentsDir()
		if err != nil {
			return attachmentDest{}, err
		}
		return attachmentDest{
			Path:             filepath.Join(dir, fmt.Sprintf("%s_%s_%s", messageID, shortID, safeFilename)),
			EnsureDefaultDir: allowEnsureDefaultDir,
		}, nil
	}

	outPath, err := config.ExpandPath(outPathFlag)
	if err != nil {
		return attachmentDest{}, err
	}

	isDir := isDirIntent(outPathFlag, outPath)
	if !isDir {
		// file path; keep as-is
		return attachmentDest{Path: outPath}, nil
	}

	filename := safeFilename
	if strings.TrimSpace(name) == "" {
		filename = fmt.Sprintf("%s_%s_attachment.bin", messageID, shortID)
	}

	return attachmentDest{Path: filepath.Join(outPath, filename)}, nil
}

func isDirIntent(outPathFlag, expandedOutPath string) bool {
	// Directory intent:
	// - existing directory path
	// - or explicit trailing slash for a (possibly non-existent) directory
	flag := strings.TrimSpace(outPathFlag)
	if strings.HasSuffix(flag, string(os.PathSeparator)) || strings.HasSuffix(flag, "/") || strings.HasSuffix(flag, "\\") {
		return true
	}
	if st, statErr := os.Stat(expandedOutPath); statErr == nil && st.IsDir() {
		return true
	}
	return false
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

	cached, cachedSize, err := cachedRegularFile(outPath, expectedSize)
	if err != nil {
		return "", false, 0, err
	}
	if cached {
		return outPath, true, cachedSize, nil
	}

	data, err := fetchAttachmentBytes(ctx, svc, messageID, attachmentID)
	if err != nil {
		return "", false, 0, err
	}
	if err := writeFileAtomic(outPath, data); err != nil {
		return "", false, 0, err
	}
	return outPath, false, int64(len(data)), nil
}

func cachedRegularFile(outPath string, expectedSize int64) (cached bool, size int64, err error) {
	if expectedSize <= 0 {
		return false, 0, nil
	}
	st, statErr := os.Stat(outPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return false, 0, nil
		}
		return false, 0, statErr
	}
	if st.IsDir() {
		return false, 0, fmt.Errorf("outPath is a directory: %s", outPath)
	}
	if st.Mode().IsRegular() && st.Size() == expectedSize {
		return true, st.Size(), nil
	}
	return false, 0, nil
}

func fetchAttachmentBytes(ctx context.Context, svc *gmail.Service, messageID, attachmentID string) ([]byte, error) {
	if svc == nil {
		return nil, errors.New("missing gmail service")
	}

	body, err := svc.Users.Messages.Attachments.Get("me", messageID, attachmentID).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if body == nil || body.Data == "" {
		return nil, errors.New("empty attachment data")
	}

	data, err := base64.RawURLEncoding.DecodeString(body.Data)
	if err != nil {
		// Gmail can return padded base64url; accept both.
		data, err = base64.URLEncoding.DecodeString(body.Data)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func writeFileAtomic(outPath string, data []byte) error {
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, ".gog-attachment-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()

	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, outPath)
}
