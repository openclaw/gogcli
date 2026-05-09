package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/ui"
)

type attachmentInfo struct {
	Filename     string
	Size         int64
	MimeType     string
	AttachmentID string
}

const (
	defaultAttachmentFilename = "attachment"
	mimeDispositionAttachment = defaultAttachmentFilename
)

type attachmentOutput struct {
	Filename     string `json:"filename"`
	Size         int64  `json:"size"`
	SizeHuman    string `json:"sizeHuman"`
	MimeType     string `json:"mimeType"`
	AttachmentID string `json:"attachmentId"`
}

type attachmentDownloadOutput struct {
	MessageID string `json:"messageId"`
	attachmentOutput
	Path   string `json:"path,omitempty"`
	Cached bool   `json:"cached,omitempty"`
}

type attachmentDownloadSummary struct {
	MessageID     string `json:"messageId"`
	AttachmentID  string `json:"attachmentId"`
	Filename      string `json:"filename"`
	MimeType      string `json:"mimeType,omitempty"`
	Size          int64  `json:"size,omitempty"`
	Path          string `json:"path"`
	Cached        bool   `json:"cached"`
	DownloadError string `json:"error,omitempty"`
}

type attachmentDownloadDraftOutput struct {
	MessageID    string `json:"messageId"`
	AttachmentID string `json:"attachmentId"`
	Filename     string `json:"filename"`
	Path         string `json:"path"`
	Cached       bool   `json:"cached"`
}

func attachmentOutputFromInfo(a attachmentInfo) attachmentOutput {
	return attachmentOutput{
		Filename:     a.Filename,
		Size:         a.Size,
		SizeHuman:    formatBytes(a.Size),
		MimeType:     a.MimeType,
		AttachmentID: a.AttachmentID,
	}
}

func attachmentOutputs(attachments []attachmentInfo) []attachmentOutput {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]attachmentOutput, len(attachments))
	for i, a := range attachments {
		out[i] = attachmentOutputFromInfo(a)
	}
	return out
}

func attachmentOutputsFromDownloads(attachments []attachmentDownloadOutput) []attachmentOutput {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]attachmentOutput, len(attachments))
	for i, a := range attachments {
		out[i] = a.attachmentOutput
	}
	return out
}

func attachmentDownloadOutputsFromInfo(messageID string, attachments []attachmentInfo) []attachmentDownloadOutput {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]attachmentDownloadOutput, len(attachments))
	for i, a := range attachments {
		out[i] = attachmentDownloadOutput{
			MessageID:        messageID,
			attachmentOutput: attachmentOutputFromInfo(a),
		}
	}
	return out
}

func attachmentDownloadSummaries(attachments []attachmentDownloadOutput) []attachmentDownloadSummary {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]attachmentDownloadSummary, len(attachments))
	for i, a := range attachments {
		out[i] = attachmentDownloadSummary{
			MessageID:    a.MessageID,
			AttachmentID: a.AttachmentID,
			Filename:     a.Filename,
			MimeType:     a.MimeType,
			Size:         a.Size,
			Path:         a.Path,
			Cached:       a.Cached,
		}
	}
	return out
}

func attachmentDownloadDraftOutputs(attachments []attachmentDownloadOutput) []attachmentDownloadDraftOutput {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]attachmentDownloadDraftOutput, len(attachments))
	for i, a := range attachments {
		out[i] = attachmentDownloadDraftOutput{
			MessageID:    a.MessageID,
			AttachmentID: a.AttachmentID,
			Filename:     a.Filename,
			Path:         a.Path,
			Cached:       a.Cached,
		}
	}
	return out
}

func attachmentLine(a attachmentOutput) string {
	return fmt.Sprintf("attachment\t%s\t%s\t%s\t%s", a.Filename, a.SizeHuman, a.MimeType, a.AttachmentID)
}

func printAttachmentLines(p *ui.Printer, attachments []attachmentOutput) {
	for _, a := range attachments {
		p.Println(attachmentLine(a))
	}
}

func printAttachmentSection(p *ui.Printer, attachments []attachmentInfo) {
	out := attachmentOutputs(attachments)
	if len(out) == 0 {
		return
	}
	p.Println("Attachments:")
	printAttachmentLines(p, out)
	p.Println("")
}

func downloadAttachmentOutputs(ctx context.Context, svc *gmail.Service, messageID string, attachments []attachmentInfo, dir string) ([]attachmentDownloadOutput, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	out := make([]attachmentDownloadOutput, 0, len(attachments))
	for _, a := range attachments {
		outPath, cached, err := downloadAttachment(ctx, svc, messageID, a, dir)
		if err != nil {
			return nil, err
		}
		out = append(out, attachmentDownloadOutput{
			MessageID:        messageID,
			attachmentOutput: attachmentOutputFromInfo(a),
			Path:             outPath,
			Cached:           cached,
		})
	}
	return out, nil
}

func collectAttachments(p *gmail.MessagePart) []attachmentInfo {
	if p == nil {
		return nil
	}
	var out []attachmentInfo
	if p.Body != nil && p.Body.AttachmentId != "" {
		filename := p.Filename
		if strings.TrimSpace(filename) == "" {
			filename = defaultAttachmentFilename
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
