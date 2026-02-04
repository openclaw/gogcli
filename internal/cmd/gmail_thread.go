package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html/charset"
	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/config"
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
	Get         GmailThreadGetCmd         `cmd:"" name:"get" default:"withargs" help:"Get a thread with all messages (optionally download attachments)"`
	Modify      GmailThreadModifyCmd      `cmd:"" name:"modify" help:"Modify labels on all messages in a thread"`
	Attachments GmailThreadAttachmentsCmd `cmd:"" name:"attachments" help:"List all attachments in a thread"`
}

type GmailThreadGetCmd struct {
	ThreadID  string        `arg:"" name:"threadId" help:"Thread ID"`
	Download  bool          `name:"download" help:"Download attachments"`
	Full      bool          `name:"full" help:"Show full message bodies"`
	OutputDir OutputDirFlag `embed:""`
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
		if strings.TrimSpace(c.OutputDir.Dir) == "" {
			// Default: current directory, not gogcli config dir.
			attachDir = "."
		} else {
			expanded, err := config.ExpandPath(c.OutputDir.Dir)
			if err != nil {
				return err
			}
			attachDir = filepath.Clean(expanded)
		}
	}

	if outfmt.IsJSON(ctx) {
		var downloadedFiles []attachmentDownloadSummary
		if c.Download && thread != nil {
			for _, msg := range thread.Messages {
				if msg == nil || msg.Id == "" {
					continue
				}
				downloads, err := downloadAttachmentOutputs(ctx, svc, msg.Id, collectAttachments(msg.Payload), attachDir)
				if err != nil {
					return err
				}
				downloadedFiles = append(downloadedFiles, attachmentDownloadSummaries(downloads)...)
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
			// Limit body preview to avoid overwhelming output
			// Use runes to avoid breaking multi-byte UTF-8 characters
			runes := []rune(cleanBody)
			if len(runes) > 500 && !c.Full {
				cleanBody = string(runes[:500]) + "... [truncated]"
			}
			u.Out().Println(cleanBody)
			u.Out().Println("")
		}

		attachments := collectAttachments(msg.Payload)
		printAttachmentSection(u.Out(), attachments)

		if c.Download && len(attachments) > 0 {
			downloads, err := downloadAttachmentOutputs(ctx, svc, msg.Id, attachments, attachDir)
			if err != nil {
				return err
			}
			for _, a := range downloads {
				if a.Cached {
					u.Out().Printf("Cached: %s", a.Path)
				} else {
					u.Out().Successf("Saved: %s", a.Path)
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
	ThreadID  string        `arg:"" name:"threadId" help:"Thread ID"`
	Download  bool          `name:"download" help:"Download all attachments"`
	OutputDir OutputDirFlag `embed:""`
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
		if strings.TrimSpace(c.OutputDir.Dir) == "" {
			attachDir = "."
		} else {
			expanded, err := config.ExpandPath(c.OutputDir.Dir)
			if err != nil {
				return err
			}
			attachDir = filepath.Clean(expanded)
		}
	}

	var allAttachments []attachmentDownloadOutput
	for _, msg := range thread.Messages {
		if msg == nil {
			continue
		}
		attachments := collectAttachments(msg.Payload)
		if c.Download {
			downloads, err := downloadAttachmentOutputs(ctx, svc, msg.Id, attachments, attachDir)
			if err != nil {
				return err
			}
			allAttachments = append(allAttachments, downloads...)
			continue
		}
		allAttachments = append(allAttachments, attachmentDownloadOutputsFromInfo(msg.Id, attachments)...)
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

	u.Out().Printf("Found %d attachment(s):", len(allAttachments))
	if c.Download {
		for _, a := range allAttachments {
			status := "Saved"
			if a.Cached {
				status = "Cached"
			}
			u.Out().Printf("  %s: %s (%s) - %s", status, a.Filename, a.SizeHuman, a.Path)
		}
		return nil
	}
	printAttachmentLines(u.Out(), attachmentOutputsFromDownloads(allAttachments))
	return nil
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
		if looksLikeHTML(plain) {
			return plain, true
		}
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
	if mimeTypeMatches(p.MimeType, mimeType) && p.Body != nil && p.Body.Data != "" {
		s, err := decodePartBody(p)
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

func mimeTypeMatches(partType string, want string) bool {
	return normalizeMimeType(partType) == normalizeMimeType(want)
}

func normalizeMimeType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err == nil && mediaType != "" {
		return strings.ToLower(mediaType)
	}
	if idx := strings.Index(value, ";"); idx != -1 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func looksLikeHTML(value string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "<!doctype") ||
		strings.HasPrefix(trimmed, "<html") ||
		strings.HasPrefix(trimmed, "<head") ||
		strings.HasPrefix(trimmed, "<body") ||
		strings.HasPrefix(trimmed, "<meta") ||
		strings.Contains(trimmed, "<html")
}

func decodePartBody(p *gmail.MessagePart) (string, error) {
	if p == nil || p.Body == nil || p.Body.Data == "" {
		return "", nil
	}
	raw, err := decodeBase64URLBytes(p.Body.Data)
	if err != nil {
		return "", err
	}

	decoded := raw
	if cte := strings.TrimSpace(headerValue(p, "Content-Transfer-Encoding")); cte != "" {
		decoded = decodeTransferEncoding(decoded, cte)
	}

	if contentType := strings.TrimSpace(headerValue(p, "Content-Type")); contentType != "" {
		decoded = decodeBodyCharset(decoded, contentType)
	}

	return string(decoded), nil
}

func decodeTransferEncoding(data []byte, encoding string) []byte {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		if !looksLikeBase64(data) {
			return data
		}
		if decoded, err := decodeAnyBase64(data); err == nil {
			return decoded
		}
	case "quoted-printable":
		if !looksLikeQuotedPrintable(data) {
			return data
		}
		if decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(data))); err == nil {
			return decoded
		}
	}
	return data
}

func decodeBodyCharset(data []byte, contentType string) []byte {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return data
	}
	charsetLabel := strings.TrimSpace(params["charset"])
	if charsetLabel == "" || strings.EqualFold(charsetLabel, "utf-8") {
		return data
	}
	reader, err := charset.NewReaderLabel(charsetLabel, bytes.NewReader(data))
	if err != nil {
		return data
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return data
	}
	return decoded
}

func looksLikeBase64(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}
	for _, b := range trimmed {
		switch {
		case b >= 'A' && b <= 'Z':
		case b >= 'a' && b <= 'z':
		case b >= '0' && b <= '9':
		case b == '+', b == '/', b == '=', b == '-', b == '_':
		case b == '\n', b == '\r', b == '\t', b == ' ':
		default:
			return false
		}
	}
	return true
}

// looksLikeQuotedPrintable checks if data appears to contain quoted-printable
// encoded sequences. This prevents double-decoding when the Gmail API has
// already decoded the content.
//
// Detection strategy:
// 1. Soft line breaks (=\r\n or =\n) are definitive QP markers
// 2. For =XX hex sequences, we only match UPPERCASE hex (0-9, A-F) because:
//    - Most QP encoders use uppercase per RFC 2045 recommendation
//    - This avoids false positives from URLs like "?foo=bar" where "=ba"
//      would otherwise match as lowercase hex
func looksLikeQuotedPrintable(data []byte) bool {
	for i := 0; i < len(data)-2; i++ {
		if data[i] == '=' {
			// Check for soft line break (=\r\n or =\n) - definitive QP marker
			if data[i+1] == '\r' || data[i+1] == '\n' {
				return true
			}
			// Check for hex-encoded byte with UPPERCASE hex only (=XX where X is 0-9 or A-F)
			// This reduces false positives from URLs containing lowercase letters after =
			if isUpperHexDigit(data[i+1]) && isUpperHexDigit(data[i+2]) {
				return true
			}
		}
	}
	return false
}

func isUpperHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'F')
}

func decodeAnyBase64(data []byte) ([]byte, error) {
	cleaned := stripBase64Whitespace(data)
	str := string(cleaned)
	if decoded, err := base64.StdEncoding.DecodeString(str); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(str); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(str); err == nil {
		return decoded, nil
	}
	return base64.RawURLEncoding.DecodeString(str)
}

func stripBase64Whitespace(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		switch b {
		case '\n', '\r', '\t', ' ':
			continue
		default:
			out = append(out, b)
		}
	}
	return out
}

func decodeBase64URLBytes(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

func decodeBase64URL(s string) (string, error) {
	b, err := decodeBase64URLBytes(s)
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
