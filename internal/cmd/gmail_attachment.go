package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailAttachmentCmd struct {
	MessageID      string         `arg:"" name:"messageId" help:"Message ID"`
	AttachmentID   string         `arg:"" name:"attachmentId" help:"Attachment ID"`
	Output         OutputPathFlag `embed:""`
	Name           string         `name:"name" help:"Filename (used when --out is empty or points to a directory)"`
	Inline         bool           `name:"inline" help:"Also return the attachment content base64-encoded (contentBase64) in the response; attachments over the inline size limit fall back to the file path with an explanatory reason"`
	InlineMaxBytes int            `name:"inline-max-bytes" default:"3145728" help:"Maximum attachment size --inline embeds (bytes)" env:"GOG_GMAIL_INLINE_MAX_BYTES"`
}

const defaultGmailAttachmentFilename = "attachment.bin"

func printAttachmentDownloadResult(ctx context.Context, u *ui.UI, payload map[string]any) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}
	u.Out().Linef("path\t%s", payload["path"])
	u.Out().Linef("cached\t%t", payload["cached"])
	u.Out().Linef("bytes\t%d", payload["bytes"])
	for _, key := range []string{"filename", "mimeType", "contentBase64", "reason"} {
		if v, ok := payload[key]; ok {
			u.Out().Linef("%s\t%s", key, tsvSafeValue(v))
		}
	}
	return nil
}

func tsvSafeValue(v any) string {
	s := fmt.Sprintf("%v", v)
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return strconv.Quote(s)
	}
	return s
}

func (c *GmailAttachmentCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	messageID := normalizeGmailMessageID(c.MessageID)
	attachmentID := strings.TrimSpace(c.AttachmentID)
	if messageID == "" || attachmentID == "" {
		return usage("messageId/attachmentId required")
	}
	if c.InlineMaxBytes < 0 {
		return usage("--inline-max-bytes must be non-negative")
	}
	defaultDir := ""
	if strings.TrimSpace(c.Output.Path) == "" {
		layout, err := commandLayout(ctx, config.PathKindConfig)
		if err != nil {
			return err
		}
		defaultDir = layout.GmailAttachmentsDir()
	}
	dest, err := resolveAttachmentDest(messageID, attachmentID, c.Output.Path, c.Name, defaultDir)
	if err != nil {
		return err
	}

	// Avoid touching auth/keyring and avoid writing files in dry-run mode.
	if dryRunErr := dryRunExit(ctx, flags, "gmail.attachment.download", map[string]any{
		"message_id":       messageID,
		"attachment_id":    attachmentID,
		"path":             dest,
		"inline":           c.Inline,
		"inline_max_bytes": c.InlineMaxBytes,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	expectedSize := int64(-1)
	var info *attachmentInfo
	if c.Inline {
		// Inline output needs the part metadata, but always fetches the attachment
		// so contentBase64 cannot expose a stale same-size local cache entry.
		info = lookupAttachmentPartInfo(ctx, svc, messageID, attachmentID, true)
	} else if st, statErr := os.Stat(dest); statErr == nil && st.Mode().IsRegular() {
		// Only hit messages.get when we might have a cache-hit candidate.
		expectedSize = lookupAttachmentSizeEstimate(ctx, svc, messageID, attachmentID)
	}
	var path string
	var cached bool
	var bytes int64
	var data []byte
	if c.Inline {
		path, bytes, data, err = downloadAttachmentFreshToPath(ctx, svc, messageID, attachmentID, dest)
	} else {
		path, cached, bytes, err = downloadAttachmentToPath(ctx, svc, messageID, attachmentID, dest, expectedSize)
	}
	if err != nil {
		return err
	}
	payload := map[string]any{"path": path, "cached": cached, "bytes": bytes}
	if info != nil {
		if info.Filename != "" {
			payload["filename"] = info.Filename
		}
		if info.MimeType != "" {
			payload["mimeType"] = info.MimeType
		}
	}
	if c.Inline {
		addInlineContent(payload, data, c.InlineMaxBytes)
	}
	return printAttachmentDownloadResult(ctx, u, payload)
}

// addInlineContent embeds the fetched bytes, avoiding a second read of the
// caller-controlled destination path.
func addInlineContent(payload map[string]any, data []byte, maxBytes int) {
	if len(data) > maxBytes {
		payload["reason"] = fmt.Sprintf("attachment size %d bytes exceeds inline size limit (%d bytes); content written to path only", len(data), maxBytes)
		return
	}
	payload["contentBase64"] = base64.StdEncoding.EncodeToString(data)
}

func resolveAttachmentDest(messageID, attachmentID, outPathFlag, name, defaultDir string) (string, error) {
	shortID := attachmentID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	safeFilename := sanitizeAttachmentFilename(name, defaultGmailAttachmentFilename)

	if strings.TrimSpace(outPathFlag) == "" {
		if strings.TrimSpace(defaultDir) == "" {
			return "", errors.New("missing default attachment directory")
		}
		return filepath.Join(defaultDir, fmt.Sprintf("%s_%s_%s", messageID, shortID, safeFilename)), nil
	}

	outPath, err := config.ExpandPath(outPathFlag)
	if err != nil {
		return "", err
	}

	isDir := isDirIntent(outPathFlag, outPath)
	if !isDir {
		// file path; keep as-is
		return outPath, nil
	}

	filename := safeFilename
	if strings.TrimSpace(name) == "" {
		filename = fmt.Sprintf("%s_%s_attachment.bin", messageID, shortID)
	}

	return filepath.Join(outPath, filename), nil
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
	info := lookupAttachmentPartInfo(ctx, svc, messageID, attachmentID, false)
	if info == nil || info.Size <= 0 {
		return -1
	}
	return info.Size
}

// lookupAttachmentPartInfo fetches the message payload to resolve the part
// metadata (filename, mimeType, size) for an attachment ID. Best-effort: nil
// when the lookup fails or the attachment is not found.
func lookupAttachmentPartInfo(ctx context.Context, svc *gmail.Service, messageID, attachmentID string, allowSingleFallback bool) *attachmentInfo {
	if svc == nil {
		return nil
	}
	msg, err := svc.Users.Messages.Get("me", messageID).Format("full").Fields("payload").Context(ctx).Do()
	if err != nil || msg == nil {
		return nil
	}
	attachments := collectAttachments(msg.Payload)
	for _, a := range attachments {
		if a.AttachmentID == attachmentID {
			return &a
		}
	}
	// Gmail attachment IDs are not stable across API responses, so an exact
	// match can miss; with a single attachment it is unambiguous anyway.
	if allowSingleFallback && len(attachments) == 1 {
		return &attachments[0]
	}
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

	cached, cachedSize, err := cachedRegularFile(outPath, expectedSize)
	if err != nil {
		return "", false, 0, err
	}
	if cached {
		return outPath, true, cachedSize, nil
	}

	path, size, _, err := downloadAttachmentFreshToPath(ctx, svc, messageID, attachmentID, outPath)
	return path, false, size, err
}

func downloadAttachmentFreshToPath(
	ctx context.Context,
	svc *gmail.Service,
	messageID string,
	attachmentID string,
	outPath string,
) (string, int64, []byte, error) {
	if strings.TrimSpace(outPath) == "" {
		return "", 0, nil, errors.New("missing outPath")
	}
	data, err := fetchAttachmentBytes(ctx, svc, messageID, attachmentID)
	if err != nil {
		return "", 0, nil, err
	}
	if err := writeFileAtomic(outPath, data); err != nil {
		return "", 0, nil, err
	}
	return outPath, int64(len(data)), data, nil
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
