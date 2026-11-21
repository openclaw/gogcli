package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"google.golang.org/api/chat/v1"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type ChatAttachmentsCmd struct {
	Download ChatAttachmentsDownloadCmd `cmd:"" name:"download" help:"Download a message's attachments read-only (spaces.messages.get + media.download)"`
}

// chatAttachmentItem is the attachment metadata of a Chat message, as carried by
// `chat messages list --json` and `chat attachments download`.
type chatAttachmentItem struct {
	Name              string                 `json:"name"`
	ContentName       string                 `json:"contentName,omitempty"`
	ContentType       string                 `json:"contentType,omitempty"`
	Source            string                 `json:"source,omitempty"`
	AttachmentDataRef *chatAttachmentDataRef `json:"attachmentDataRef,omitempty"`
	DriveDataRef      *chatDriveDataRef      `json:"driveDataRef,omitempty"`
}

type chatAttachmentDataRef struct {
	ResourceName string `json:"resourceName,omitempty"`
}

type chatDriveDataRef struct {
	DriveFileID string `json:"driveFileId,omitempty"`
}

const defaultChatAttachmentFilename = "attachment.bin"

func compactChatAttachments(attachments []*chat.Attachment) []*chatAttachmentItem {
	var items []*chatAttachmentItem
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		item := &chatAttachmentItem{
			Name:        attachment.Name,
			ContentName: attachment.ContentName,
			ContentType: attachment.ContentType,
			Source:      attachment.Source,
		}
		if ref := attachment.AttachmentDataRef; ref != nil && ref.ResourceName != "" {
			item.AttachmentDataRef = &chatAttachmentDataRef{ResourceName: ref.ResourceName}
		}
		if ref := attachment.DriveDataRef; ref != nil && ref.DriveFileId != "" {
			item.DriveDataRef = &chatDriveDataRef{DriveFileID: ref.DriveFileId}
		}
		items = append(items, item)
	}
	return items
}

type ChatAttachmentsDownloadCmd struct {
	Name      string `arg:"" name:"name" help:"Message (spaces/.../messages/...) or one attachment (spaces/.../messages/.../attachments/...)"`
	Out       string `name:"out" aliases:"output" help:"Output file (one attachment) or directory (default: the current directory)"`
	Overwrite bool   `name:"overwrite" help:"Overwrite an existing file"`
}

// splitChatAttachmentName accepts a message name or an attachment name and returns
// (message, attachment); attachment is empty when the whole message was named.
func splitChatAttachmentName(raw string) (string, string, error) {
	name := strings.TrimSpace(raw)
	parts := strings.Split(name, "/")
	valid := func(n int) bool {
		if len(parts) != n || parts[0] != "spaces" || parts[2] != "messages" {
			return false
		}
		if n == 6 && parts[4] != "attachments" {
			return false
		}
		for _, part := range parts {
			if part == "" {
				return false
			}
		}
		return true
	}
	switch {
	case valid(4):
		return name, "", nil
	case valid(6):
		return strings.Join(parts[:4], "/"), name, nil
	}
	return "", "", usagef("invalid name %q: expected spaces/<space>/messages/<message>[/attachments/<attachment>]", raw)
}

func chatAttachmentFallbackName(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 && i+1 < len(name) {
		return name[i+1:]
	}
	return defaultChatAttachmentFilename
}

func (c *ChatAttachmentsDownloadCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	messageName, attachmentName, err := splitChatAttachmentName(c.Name)
	if err != nil {
		return err
	}
	outFlag := strings.TrimSpace(c.Out)
	outPath := ""
	if outFlag != "" {
		outPath, err = config.ExpandPath(outFlag)
		if err != nil {
			return err
		}
	}
	if dryRunErr := dryRunExit(ctx, flags, "chat.attachments.download", map[string]any{
		"message":    messageName,
		"attachment": attachmentName,
		"out":        outPath,
		"overwrite":  c.Overwrite,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}
	svc, err := chatService(ctx, account)
	if err != nil {
		return err
	}

	msg, err := svc.Spaces.Messages.Get(messageName).Context(ctx).Do()
	if err != nil {
		return err
	}
	attachments := compactChatAttachments(msg.Attachment)
	if attachmentName != "" {
		var picked []*chatAttachmentItem
		for _, item := range attachments {
			if item.Name == attachmentName {
				picked = append(picked, item)
			}
		}
		if len(picked) == 0 {
			return usagef("attachment %q not found on %s (%d attachment(s))", attachmentName, messageName, len(attachments))
		}
		attachments = picked
	}
	toDir := outFlag == "" || isDirIntent(outFlag, outPath)
	if !toDir && len(attachments) > 1 {
		return usagef("--out %q names a file but %s has %d attachments: name one attachment or pass a directory (trailing slash)", outFlag, messageName, len(attachments))
	}

	results := make([]map[string]any, 0, len(attachments))
	for _, item := range attachments {
		row := map[string]any{
			"name":        item.Name,
			"contentName": item.ContentName,
			"contentType": item.ContentType,
			"source":      item.Source,
		}
		switch {
		case item.DriveDataRef != nil:
			row["driveFileId"] = item.DriveDataRef.DriveFileID
			row["skipped"] = "DRIVE_FILE: read it with gog drive download " + item.DriveDataRef.DriveFileID
		case item.AttachmentDataRef == nil:
			row["skipped"] = "no attachmentDataRef to download"
		default:
			dest := outPath
			if toDir {
				dir := outPath
				if dir == "" {
					dir = "."
				}
				dest = filepath.Join(dir, sanitizeAttachmentFilename(item.ContentName, chatAttachmentFallbackName(item.Name)))
			}
			path, size, sum, dlErr := downloadChatAttachment(ctx, svc, item.AttachmentDataRef.ResourceName, dest, c.Overwrite)
			if dlErr != nil {
				return fmt.Errorf("download %s: %w", item.Name, dlErr)
			}
			row["path"] = path
			row["bytes"] = size
			row["sha256"] = sum
		}
		results = append(results, row)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"message":     messageName,
			"attachments": results,
		})
	}
	if len(results) == 0 {
		u.Err().Println("No attachments")
		return nil
	}
	for _, row := range results {
		u.Out().Linef("name\t%s", row["name"])
		for _, key := range []string{"contentName", "contentType", "source", "path", "bytes", "sha256", "driveFileId", "skipped"} {
			if v, ok := row[key]; ok && fmt.Sprint(v) != "" {
				u.Out().Linef("%s\t%s", key, tsvSafeValue(v))
			}
		}
	}
	return nil
}

// downloadChatAttachment streams media.download (a GET; allowed under --readonly) to
// dest and returns the absolute path, the byte count and the SHA-256 of what was written.
func downloadChatAttachment(ctx context.Context, svc *chat.Service, resourceName, dest string, overwrite bool) (string, int64, string, error) {
	resp, err := svc.Media.Download(resourceName).Context(ctx).Download()
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()
	f, path, err := openUserOutputFile(dest, outputFileOptions{Overwrite: overwrite, FileMode: 0o600, DirMode: 0o700})
	if err != nil {
		return "", 0, "", err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, hash), resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return "", 0, "", copyErr
	}
	if closeErr != nil {
		return "", 0, "", closeErr
	}
	if abs, absErr := filepath.Abs(path); absErr == nil {
		path = abs
	}
	return path, n, hex.EncodeToString(hash.Sum(nil)), nil
}
