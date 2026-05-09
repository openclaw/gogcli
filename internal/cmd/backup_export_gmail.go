package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/backup"
)

type gmailExportIndexEntry struct {
	ID           string   `json:"id"`
	ThreadID     string   `json:"threadId,omitempty"`
	HistoryID    string   `json:"historyId,omitempty"`
	InternalDate int64    `json:"internalDate,omitempty"`
	LabelIDs     []string `json:"labelIds,omitempty"`
	SizeEstimate int64    `json:"sizeEstimate,omitempty"`
	Subject      string   `json:"subject,omitempty"`
	From         string   `json:"from,omitempty"`
	To           []string `json:"to,omitempty"`
	Cc           []string `json:"cc,omitempty"`
	Date         string   `json:"date,omitempty"`
	EML          string   `json:"eml,omitempty"`
	Markdown     string   `json:"markdown,omitempty"`
	Attachments  []string `json:"attachments,omitempty"`
}

type backupEmail struct {
	Subject     string
	From        string
	To          []string
	Cc          []string
	Date        string
	TextBody    string
	HTMLBody    string
	ParseError  string
	Attachments []backupEmailAttachment
}

type backupEmailAttachment struct {
	Filename string
	Data     []byte
}

func exportGmailLabels(outDir string, shard backup.PlainShard) (int, int, error) {
	var labels []gmailBackupLabel
	if err := backup.DecodeJSONL(shard.Plaintext, &labels); err != nil {
		return 0, 0, err
	}
	path := filepath.Join(outDir, backupServiceGmail, sanitizeFilePart(shard.Account), "labels.json")
	if err := writeJSONFile(path, labels); err != nil {
		return 0, 0, err
	}
	return 1, len(labels), nil
}

func exportGmailMessages(outDir string, shard backup.PlainShard, opts backupExportOptions) (int, int, error) {
	var messages []gmailBackupMessage
	if err := backup.DecodeJSONL(shard.Plaintext, &messages); err != nil {
		return 0, 0, err
	}
	gmailFormat := strings.ToLower(strings.TrimSpace(opts.GmailFormat))
	if gmailFormat == "" {
		gmailFormat = "eml"
	}
	attachmentsMode := strings.ToLower(strings.TrimSpace(opts.GmailAttachments))
	if attachmentsMode == "" {
		attachmentsMode = "extract"
	}
	account := sanitizeFilePart(shard.Account)
	indexPath := filepath.Join(outDir, backupServiceGmail, account, "messages", "index.jsonl")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o700); err != nil {
		return 0, 0, err
	}
	indexFile, err := os.OpenFile(indexPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- path is confined to caller-selected export dir and sanitized account.
	if err != nil {
		return 0, 0, err
	}
	defer indexFile.Close()
	enc := json.NewEncoder(indexFile)
	enc.SetEscapeHTML(false)
	files := 0
	for _, message := range messages {
		rawMIME, err := decodeGmailRaw(message.Raw)
		if err != nil {
			return files, 0, fmt.Errorf("decode Gmail raw %s: %w", message.ID, err)
		}
		parsed, parseErr := parseBackupEmail(rawMIME)
		if parseErr != nil && gmailFormat != "eml" {
			parsed.ParseError = parseErr.Error()
		}
		entry := gmailExportIndexEntry{
			ID:           message.ID,
			ThreadID:     message.ThreadID,
			HistoryID:    message.HistoryID,
			InternalDate: message.InternalDate,
			LabelIDs:     message.LabelIDs,
			SizeEstimate: message.SizeEstimate,
			Subject:      parsed.Subject,
			From:         parsed.From,
			To:           parsed.To,
			Cc:           parsed.Cc,
			Date:         parsed.Date,
		}
		if gmailFormat == "eml" || gmailFormat == "both" {
			rel := backupExportMessageEMLPath(account, message)
			path := filepath.Join(outDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return files, 0, err
			}
			if err := os.WriteFile(path, rawMIME, 0o600); err != nil {
				return files, 0, err
			}
			files++
			entry.EML = rel
		}
		if gmailFormat == "markdown" || gmailFormat == "both" {
			rel, attachmentRels, written, err := exportGmailMarkdownMessage(outDir, account, message, parsed, attachmentsMode == "extract")
			if err != nil {
				return files, 0, err
			}
			files += written
			entry.Markdown = rel
			entry.Attachments = attachmentRels
		}
		if err := enc.Encode(entry); err != nil {
			return files, 0, err
		}
	}
	return files + 1, len(messages), nil
}

func decodeGmailRaw(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty raw payload")
	}
	if data, err := base64.RawURLEncoding.DecodeString(raw); err == nil {
		return data, nil
	}
	return base64.URLEncoding.DecodeString(raw)
}

func backupExportMessageEMLPath(account string, message gmailBackupMessage) string {
	timestamp := trackingUnknown
	yearMonth := trackingUnknown
	if message.InternalDate > 0 {
		t := time.UnixMilli(message.InternalDate).UTC()
		timestamp = t.Format("20060102T150405Z")
		yearMonth = filepath.Join(fmt.Sprintf("%04d", t.Year()), fmt.Sprintf("%02d", int(t.Month())))
	}
	name := timestamp + "-" + sanitizeFilePart(message.ID) + ".eml"
	return filepath.ToSlash(filepath.Join(backupServiceGmail, account, "messages", yearMonth, name))
}

func backupExportMessageDir(account string, message gmailBackupMessage, subject string) string {
	timestamp := trackingUnknown
	yearMonth := trackingUnknown
	if message.InternalDate > 0 {
		t := time.UnixMilli(message.InternalDate).UTC()
		timestamp = t.Format("20060102T150405Z")
		yearMonth = filepath.Join(fmt.Sprintf("%04d", t.Year()), fmt.Sprintf("%02d", int(t.Month())))
	}
	subjectPart := truncateFilePart(sanitizeFilePart(subject), 72)
	if subjectPart == trackingUnknown {
		subjectPart = "no-subject"
	}
	name := timestamp + "-" + subjectPart + "-" + sanitizeFilePart(message.ID)
	return filepath.ToSlash(filepath.Join(backupServiceGmail, account, "messages", yearMonth, name))
}

func exportGmailMarkdownMessage(outDir, account string, message gmailBackupMessage, parsed backupEmail, extractAttachments bool) (string, []string, int, error) {
	messageDirRel := backupExportMessageDir(account, message, parsed.Subject)
	messageDir := filepath.Join(outDir, filepath.FromSlash(messageDirRel))
	if err := os.MkdirAll(messageDir, 0o700); err != nil {
		return "", nil, 0, err
	}
	var attachmentRels []string
	files := 0
	if extractAttachments {
		seen := map[string]int{}
		for i, attachment := range parsed.Attachments {
			filename := sanitizeBackupAttachmentFilename(attachment.Filename, i+1)
			filename = uniqueExportFilename(seen, filename)
			rel := filepath.ToSlash(filepath.Join(messageDirRel, "attachments", filename))
			path := filepath.Join(outDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return "", nil, files, err
			}
			if err := os.WriteFile(path, attachment.Data, 0o600); err != nil {
				return "", nil, files, err
			}
			attachmentRels = append(attachmentRels, rel)
			files++
		}
	}
	body := backupEmailMarkdownBody(parsed)
	md := renderGmailMessageMarkdown(message, parsed, body, attachmentRels)
	rel := filepath.ToSlash(filepath.Join(messageDirRel, "message.md"))
	path := filepath.Join(outDir, filepath.FromSlash(rel))
	if err := os.WriteFile(path, []byte(md), 0o600); err != nil {
		return "", nil, files, err
	}
	files++
	return rel, attachmentRels, files, nil
}

func backupEmailMarkdownBody(parsed backupEmail) string {
	if strings.TrimSpace(parsed.TextBody) != "" {
		return backupEmailMarkdownText(parsed.TextBody)
	}
	if strings.TrimSpace(parsed.HTMLBody) != "" {
		return cleanBackupHTMLBody(parsed.HTMLBody)
	}
	return ""
}

func backupEmailMarkdownText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if looksLikeHTML(value) || looksLikeHTMLFragment(value) {
		return cleanBackupHTMLBody(value)
	}
	return value
}

func cleanBackupHTMLBody(value string) string {
	cleaned := stdhtml.UnescapeString(stripHTMLTags(value))
	return strings.Join(strings.Fields(cleaned), " ")
}

func looksLikeHTMLFragment(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return false
	}
	for _, marker := range []string{
		"<p", "</p", "<br", "<div", "</div", "<span", "</span", "<table", "</table",
		"<tr", "</tr", "<td", "</td", "<section", "</section", "<blockquote",
		"</blockquote", "<a ", "</a", "<img", "<font", "</font", "<style", "<!--",
	} {
		if strings.Contains(trimmed, marker) {
			return true
		}
	}
	return false
}

func renderGmailMessageMarkdown(message gmailBackupMessage, parsed backupEmail, body string, attachmentRels []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	writeYAMLScalar(&b, "gmail_id", message.ID)
	writeYAMLScalar(&b, "thread_id", message.ThreadID)
	writeYAMLScalar(&b, "history_id", message.HistoryID)
	if message.InternalDate > 0 {
		writeYAMLScalar(&b, "internal_date", time.UnixMilli(message.InternalDate).UTC().Format(time.RFC3339))
	}
	writeYAMLScalar(&b, "date", parsed.Date)
	writeYAMLScalar(&b, "from", parsed.From)
	writeYAMLList(&b, "to", parsed.To)
	writeYAMLList(&b, "cc", parsed.Cc)
	writeYAMLScalar(&b, "subject", parsed.Subject)
	writeYAMLList(&b, "labels", message.LabelIDs)
	writeYAMLScalar(&b, "parse_error", parsed.ParseError)
	if message.SizeEstimate > 0 {
		fmt.Fprintf(&b, "size_estimate: %d\n", message.SizeEstimate)
	}
	writeYAMLList(&b, "attachments", attachmentRels)
	b.WriteString("---\n\n")
	if strings.TrimSpace(parsed.Subject) != "" {
		b.WriteString("# ")
		b.WriteString(markdownHeadingText(parsed.Subject))
		b.WriteString("\n\n")
	}
	trimmedBody := strings.TrimSpace(body)
	parseError := strings.TrimSpace(parsed.ParseError)
	switch {
	case trimmedBody != "":
		b.WriteString(trimmedBody)
		b.WriteString("\n")
	case parseError != "":
		b.WriteString("_MIME parse failed: ")
		b.WriteString(markdownHeadingText(parseError))
		b.WriteString("._\n\n")
		b.WriteString("_Raw MIME remains available in the encrypted backup._\n")
	default:
		b.WriteString("_No text body found._\n")
	}
	if len(attachmentRels) > 0 {
		b.WriteString("\n## Attachments\n\n")
		for _, rel := range attachmentRels {
			name := filepath.Base(rel)
			b.WriteString("- [")
			b.WriteString(markdownLinkText(name))
			b.WriteString("](")
			b.WriteString("attachments/")
			b.WriteString(markdownLinkTarget(name))
			b.WriteString(")\n")
		}
	}
	return b.String()
}

func parseBackupEmail(rawMIME []byte) (backupEmail, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(rawMIME))
	if err != nil {
		return backupEmail{}, err
	}
	out := backupEmail{
		Subject: decodeMIMEHeader(msg.Header.Get("Subject")),
		From:    decodeMIMEHeader(msg.Header.Get("From")),
		Date:    decodeMIMEHeader(msg.Header.Get("Date")),
		To:      parseAddressHeader(msg.Header.Get("To")),
		Cc:      parseAddressHeader(msg.Header.Get("Cc")),
	}
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return out, err
	}
	if err := parseBackupEmailEntity(body, string(msg.Header.Get("Content-Type")), string(msg.Header.Get("Content-Transfer-Encoding")), &out); err != nil {
		return out, err
	}
	return out, nil
}

func parseBackupEmailEntity(body []byte, contentType, transferEncoding string, out *backupEmail) error {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || strings.TrimSpace(mediaType) == "" {
		mediaType = mimeTextPlain
	}
	mediaType = strings.ToLower(mediaType)
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if strings.TrimSpace(boundary) == "" {
			return nil
		}
		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		for {
			part, partErr := reader.NextPart()
			if partErr == io.EOF {
				break
			}
			if partErr != nil {
				return partErr
			}
			partBody, readErr := io.ReadAll(part)
			_ = part.Close()
			if readErr != nil {
				return readErr
			}
			partContentType := part.Header.Get("Content-Type")
			partEncoding := part.Header.Get("Content-Transfer-Encoding")
			if isBackupEmailAttachment(part.Header.Get("Content-Disposition"), partContentType) {
				decoded := decodeTransferEncoding(partBody, partEncoding)
				filename := backupAttachmentFilename(part.Header.Get("Content-Disposition"), partContentType)
				out.Attachments = append(out.Attachments, backupEmailAttachment{
					Filename: filename,
					Data:     decoded,
				})
				continue
			}
			if err := parseBackupEmailEntity(partBody, partContentType, partEncoding, out); err != nil {
				return err
			}
		}
		return nil
	}
	decoded := decodeTransferEncoding(body, transferEncoding)
	decoded = decodeBodyCharset(decoded, contentType)
	switch mediaType {
	case mimeTextPlain:
		if strings.TrimSpace(out.TextBody) == "" {
			out.TextBody = string(decoded)
		}
	case mimeHTML:
		if strings.TrimSpace(out.HTMLBody) == "" {
			out.HTMLBody = string(decoded)
		}
	}
	return nil
}

func isBackupEmailAttachment(contentDisposition, contentType string) bool {
	disposition, dispParams, _ := mime.ParseMediaType(contentDisposition)
	if strings.EqualFold(disposition, mimeDispositionAttachment) {
		return true
	}
	if strings.EqualFold(disposition, "inline") && strings.TrimSpace(dispParams["filename"]) != "" {
		return true
	}
	_, typeParams, _ := mime.ParseMediaType(contentType)
	return strings.TrimSpace(typeParams["name"]) != ""
}

func backupAttachmentFilename(contentDisposition, contentType string) string {
	_, dispParams, _ := mime.ParseMediaType(contentDisposition)
	if filename := decodeMIMEHeader(dispParams["filename"]); strings.TrimSpace(filename) != "" {
		return filename
	}
	_, typeParams, _ := mime.ParseMediaType(contentType)
	if filename := decodeMIMEHeader(typeParams["name"]); strings.TrimSpace(filename) != "" {
		return filename
	}
	return defaultAttachmentFilename
}

func decodeMIMEHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(value)
	if err == nil {
		return strings.TrimSpace(decoded)
	}
	return value
}

func parseAddressHeader(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	addrs, err := mail.ParseAddressList(value)
	if err != nil {
		return []string{decodeMIMEHeader(value)}
	}
	out := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		out = append(out, addr.String())
	}
	return out
}

func writeYAMLScalar(b *strings.Builder, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "%s: %q\n", key, value)
}

func writeYAMLList(b *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", key)
	for _, value := range values {
		fmt.Fprintf(b, "  - %q\n", value)
	}
}

func markdownHeadingText(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func markdownLinkText(value string) string {
	value = strings.ReplaceAll(value, "[", "\\[")
	value = strings.ReplaceAll(value, "]", "\\]")
	return value
}

func markdownLinkTarget(value string) string {
	value = strings.ReplaceAll(value, " ", "%20")
	value = strings.ReplaceAll(value, "(", "%28")
	value = strings.ReplaceAll(value, ")", "%29")
	return value
}

func sanitizeBackupAttachmentFilename(value string, fallbackIndex int) string {
	value = filepath.Base(strings.TrimSpace(value))
	if value == "" || value == "." || value == ".." {
		value = fmt.Sprintf("attachment-%03d", fallbackIndex)
	}
	return sanitizeFilePart(value)
}

func uniqueExportFilename(seen map[string]int, filename string) string {
	if filename == "" {
		filename = defaultAttachmentFilename
	}
	count := seen[filename]
	seen[filename] = count + 1
	if count == 0 {
		return filename
	}
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	return fmt.Sprintf("%s-%d%s", base, count+1, ext)
}

func truncateFilePart(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.Trim(value[:limit], "._-")
}
