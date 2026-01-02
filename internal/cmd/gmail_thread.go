package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// HTML stripping patterns for cleaner text output.
var (
	// Remove script blocks entirely (including content)
	scriptPattern = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	// Remove style blocks entirely (including content)
	stylePattern = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	// Remove all HTML tags
	htmlTagPattern = regexp.MustCompile(`<[^>]*>`)
	// Collapse multiple whitespace/newlines
	whitespacePattern = regexp.MustCompile(`\s+`)
)

func stripHTMLTags(s string) string {
	// First remove script and style blocks entirely
	s = scriptPattern.ReplaceAllString(s, "")
	s = stylePattern.ReplaceAllString(s, "")
	// Then remove remaining HTML tags
	s = htmlTagPattern.ReplaceAllString(s, " ")
	// Collapse whitespace
	s = whitespacePattern.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

type GmailThreadCmd struct {
	Get         GmailThreadGetCmd         `cmd:"" name:"get" help:"Get a thread with all messages (optionally download attachments)"`
	Modify      GmailThreadModifyCmd      `cmd:"" name:"modify" help:"Modify labels on all messages in a thread"`
	Attachments GmailThreadAttachmentsCmd `cmd:"" name:"attachments" help:"List all attachments in a thread"`
}

type GmailThreadGetCmd struct {
	ThreadID string `arg:"" name:"threadId" help:"Thread ID"`
	Download bool   `name:"download" help:"Download attachments"`
	OutDir   string `name:"out-dir" help:"Directory to write attachments to (default: current directory)"`
	Full     bool   `name:"full" help:"Show complete message bodies without truncation"`
}

func (c *GmailThreadGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	threadID := strings.TrimSpace(c.ThreadID)
	if threadID == "" {
		return usage("empty threadId")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	thread, err := svc.Users.Threads.Get("me", threadID).Format("full").Context(ctx).Do()
	if err != nil {
		return err
	}

	var attachDir string
	if c.Download {
		if strings.TrimSpace(c.OutDir) == "" {
			// Default: current directory, not gogcli config dir.
			attachDir = "."
		} else {
			attachDir = filepath.Clean(c.OutDir)
		}
	}

	if outfmt.IsJSON(ctx) {
		type downloaded struct {
			MessageID     string `json:"messageId"`
			AttachmentID  string `json:"attachmentId"`
			Filename      string `json:"filename"`
			MimeType      string `json:"mimeType,omitempty"`
			Size          int64  `json:"size,omitempty"`
			Path          string `json:"path"`
			Cached        bool   `json:"cached"`
			DownloadError string `json:"error,omitempty"`
		}
		downloadedFiles := make([]downloaded, 0)
		if c.Download && thread != nil {
			for _, msg := range thread.Messages {
				if msg == nil || msg.Id == "" {
					continue
				}
				for _, a := range collectAttachments(msg.Payload) {
					outPath, cached, err := downloadAttachment(ctx, svc, msg.Id, a, attachDir)
					if err != nil {
						return err
					}
					df := downloaded{
						MessageID:    msg.Id,
						AttachmentID: a.AttachmentID,
						Filename:     a.Filename,
						MimeType:     a.MimeType,
						Size:         a.Size,
						Path:         outPath,
						Cached:       cached,
					}
					downloadedFiles = append(downloadedFiles, df)
				}
			}
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"thread":     thread,
			"downloaded": downloadedFiles,
		})
	}
	if thread == nil || len(thread.Messages) == 0 {
		u.Err().Println("Empty thread")
		return nil
	}

	// Show message count upfront so users know how many messages to expect
	u.Out().Printf("Thread contains %d message(s)", len(thread.Messages))
	u.Out().Println("")

	for i, msg := range thread.Messages {
		if msg == nil {
			continue
		}
		u.Out().Printf("=== Message %d/%d: %s ===", i+1, len(thread.Messages), msg.Id)
		u.Out().Printf("From: %s", headerValue(msg.Payload, "From"))
		u.Out().Printf("To: %s", headerValue(msg.Payload, "To"))
		u.Out().Printf("Subject: %s", headerValue(msg.Payload, "Subject"))
		u.Out().Printf("Date: %s", headerValue(msg.Payload, "Date"))
		u.Out().Println("")

		body, isHTML := bestBodyForDisplay(msg.Payload)
		if body != "" {
			cleanBody := body
			if isHTML {
				// Strip HTML tags for cleaner text output
				cleanBody = stripHTMLTags(body)
			}
			if !c.Full {
				// Limit body preview to avoid overwhelming output.
				// Use runes to avoid breaking multi-byte UTF-8 characters.
				runes := []rune(cleanBody)
				if len(runes) > 500 {
					cleanBody = string(runes[:500]) + "... [truncated; use --full]"
				}
			}
			u.Out().Println(cleanBody)
			u.Out().Println("")
		}

		attachments := collectAttachments(msg.Payload)
		if len(attachments) > 0 {
			u.Out().Println("Attachments:")
			for _, a := range attachments {
				u.Out().Printf("  - %s (%d bytes)", a.Filename, a.Size)
			}
			u.Out().Println("")
		}

		if c.Download && len(attachments) > 0 {
			for _, a := range attachments {
				outPath, cached, err := downloadAttachment(ctx, svc, msg.Id, a, attachDir)
				if err != nil {
					return err
				}
				if cached {
					u.Out().Printf("Cached: %s", outPath)
				} else {
					u.Out().Successf("Saved: %s", outPath)
				}
			}
			u.Out().Println("")
		}
	}

	return nil
}

type GmailThreadModifyCmd struct {
	ThreadID string `arg:"" name:"threadId" help:"Thread ID"`
	Add      string `name:"add" help:"Labels to add (comma-separated, name or ID)"`
	Remove   string `name:"remove" help:"Labels to remove (comma-separated, name or ID)"`
}

func (c *GmailThreadModifyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	threadID := strings.TrimSpace(c.ThreadID)
	if threadID == "" {
		return usage("empty threadId")
	}

	addLabels := splitCSV(c.Add)
	removeLabels := splitCSV(c.Remove)
	if len(addLabels) == 0 && len(removeLabels) == 0 {
		return usage("must specify --add and/or --remove")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Resolve label names to IDs
	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}

	addIDs := resolveLabelIDs(addLabels, idMap)
	removeIDs := resolveLabelIDs(removeLabels, idMap)

	// Use Gmail's Threads.Modify API
	_, err = svc.Users.Threads.Modify("me", threadID, &gmail.ModifyThreadRequest{
		AddLabelIds:    addIDs,
		RemoveLabelIds: removeIDs,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"modified":      threadID,
			"addedLabels":   addIDs,
			"removedLabels": removeIDs,
		})
	}

	u.Out().Printf("Modified thread %s", threadID)
	return nil
}

// GmailThreadAttachmentsCmd lists all attachments in a thread.
type GmailThreadAttachmentsCmd struct {
	ThreadID string `arg:"" name:"threadId" help:"Thread ID"`
	Download bool   `name:"download" help:"Download all attachments"`
	OutDir   string `name:"out-dir" help:"Directory to write attachments to (default: current directory)"`
}

func (c *GmailThreadAttachmentsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	threadID := strings.TrimSpace(c.ThreadID)
	if threadID == "" {
		return usage("empty threadId")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	thread, err := svc.Users.Threads.Get("me", threadID).Format("full").Context(ctx).Do()
	if err != nil {
		return err
	}

	if thread == nil || len(thread.Messages) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(os.Stdout, map[string]any{
				"threadId":    threadID,
				"attachments": []any{},
			})
		}
		u.Err().Println("Empty thread")
		return nil
	}

	var attachDir string
	if c.Download {
		if strings.TrimSpace(c.OutDir) == "" {
			attachDir = "."
		} else {
			attachDir = filepath.Clean(c.OutDir)
		}
	}

	type attachmentOutput struct {
		MessageID    string `json:"messageId"`
		AttachmentID string `json:"attachmentId"`
		Filename     string `json:"filename"`
		Size         int64  `json:"size"`
		SizeHuman    string `json:"sizeHuman"`
		MimeType     string `json:"mimeType"`
		Path         string `json:"path,omitempty"`
		Cached       bool   `json:"cached,omitempty"`
	}

	var allAttachments []attachmentOutput
	for _, msg := range thread.Messages {
		if msg == nil {
			continue
		}
		for _, a := range collectAttachments(msg.Payload) {
			att := attachmentOutput{
				MessageID:    msg.Id,
				AttachmentID: a.AttachmentID,
				Filename:     a.Filename,
				Size:         a.Size,
				SizeHuman:    formatBytes(a.Size),
				MimeType:     a.MimeType,
			}
			if c.Download {
				outPath, cached, err := downloadAttachment(ctx, svc, msg.Id, a, attachDir)
				if err != nil {
					return err
				}
				att.Path = outPath
				att.Cached = cached
			}
			allAttachments = append(allAttachments, att)
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"threadId":    threadID,
			"attachments": allAttachments,
		})
	}

	if len(allAttachments) == 0 {
		u.Out().Println("No attachments found")
		return nil
	}

	u.Out().Printf("Found %d attachment(s):\n", len(allAttachments))
	for _, a := range allAttachments {
		if c.Download {
			status := "Saved"
			if a.Cached {
				status = "Cached"
			}
			u.Out().Printf("  %s: %s (%s) - %s", status, a.Filename, a.SizeHuman, a.Path)
		} else {
			u.Out().Printf("  - %s (%s) [%s]", a.Filename, a.SizeHuman, a.MimeType)
		}
	}
	return nil
}

// formatBytes formats bytes into human-readable format.
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

type GmailURLCmd struct {
	ThreadIDs []string `arg:"" name:"threadId" help:"Thread IDs"`
}

func (c *GmailURLCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		urls := make([]map[string]string, 0, len(c.ThreadIDs))
		for _, id := range c.ThreadIDs {
			urls = append(urls, map[string]string{
				"id":  id,
				"url": fmt.Sprintf("https://mail.google.com/mail/?authuser=%s#all/%s", url.QueryEscape(account), id),
			})
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{"urls": urls})
	}
	for _, id := range c.ThreadIDs {
		threadURL := fmt.Sprintf("https://mail.google.com/mail/?authuser=%s#all/%s", url.QueryEscape(account), id)
		u.Out().Printf("%s\t%s", id, threadURL)
	}
	return nil
}

type attachmentInfo struct {
	Filename     string
	Size         int64
	MimeType     string
	AttachmentID string
}

func collectAttachments(p *gmail.MessagePart) []attachmentInfo {
	if p == nil {
		return nil
	}
	var out []attachmentInfo
	if p.Body != nil && p.Body.AttachmentId != "" {
		filename := p.Filename
		if strings.TrimSpace(filename) == "" {
			filename = "attachment"
		}
		out = append(out, attachmentInfo{
			Filename:     filename,
			Size:         p.Body.Size,
			MimeType:     p.MimeType,
			AttachmentID: p.Body.AttachmentId,
		})
	}
	for _, part := range p.Parts {
		out = append(out, collectAttachments(part)...)
	}
	return out
}

func bestBodyText(p *gmail.MessagePart) string {
	if p == nil {
		return ""
	}
	plain := findPartBody(p, "text/plain")
	if plain != "" {
		return plain
	}
	html := findPartBody(p, "text/html")
	return html
}

func bestBodyForDisplay(p *gmail.MessagePart) (string, bool) {
	if p == nil {
		return "", false
	}
	plain := findPartBody(p, "text/plain")
	if plain != "" {
		return plain, false
	}
	html := findPartBody(p, "text/html")
	if html == "" {
		return "", false
	}
	return html, true
}

func findPartBody(p *gmail.MessagePart, mimeType string) string {
	if p == nil {
		return ""
	}
	if p.MimeType == mimeType && p.Body != nil && p.Body.Data != "" {
		s, err := decodeBase64URL(p.Body.Data)
		if err == nil {
			return s
		}
	}
	for _, part := range p.Parts {
		if s := findPartBody(part, mimeType); s != "" {
			return s
		}
	}
	return ""
}

func decodeBase64URL(s string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func downloadAttachment(ctx context.Context, svc *gmail.Service, messageID string, a attachmentInfo, dir string) (string, bool, error) {
	if strings.TrimSpace(messageID) == "" || strings.TrimSpace(a.AttachmentID) == "" {
		return "", false, errors.New("missing messageID/attachmentID")
	}
	if strings.TrimSpace(dir) == "" {
		dir = "."
	}
	shortID := a.AttachmentID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	// Sanitize filename to prevent path traversal attacks
	safeFilename := filepath.Base(a.Filename)
	if safeFilename == "" || safeFilename == "." || safeFilename == ".." {
		safeFilename = "attachment"
	}
	filename := fmt.Sprintf("%s_%s_%s", messageID, shortID, safeFilename)
	outPath := filepath.Join(dir, filename)
	path, cached, _, err := downloadAttachmentToPath(ctx, svc, messageID, a.AttachmentID, outPath, a.Size)
	if err != nil {
		return "", false, err
	}
	return path, cached, nil
}
