package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/gmailcontent"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailThreadCmd struct {
	Get         GmailThreadGetCmd         `cmd:"" name:"get" aliases:"info,show" default:"withargs" help:"Get a thread with all messages (optionally download attachments)"`
	Modify      GmailThreadModifyCmd      `cmd:"" name:"modify" aliases:"update,edit,set" help:"Modify labels on all messages in a thread"`
	Attachments GmailThreadAttachmentsCmd `cmd:"" name:"attachments" aliases:"files" help:"List all attachments in a thread"`
}

type GmailThreadGetCmd struct {
	ThreadID        string        `arg:"" name:"threadId" help:"Thread ID"`
	Download        bool          `name:"download" help:"Download attachments"`
	Full            bool          `name:"full" help:"Show full message bodies without truncation"`
	SanitizeContent bool          `name:"sanitize-content" aliases:"sanitize,safe" help:"Emit agent-oriented sanitized content: strip HTML, remove HTTP(S) URLs, and omit raw Gmail payloads from JSON"`
	OutputDir       OutputDirFlag `embed:""`
}

func (c *GmailThreadGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	threadID := strings.TrimSpace(c.ThreadID)
	threadID = normalizeGmailThreadID(threadID)
	if threadID == "" {
		return usage("empty threadId")
	}

	svc, err := gmailService(ctx, account)
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
		if c.SanitizeContent {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
				"thread":     sanitizedGmailThread(thread, true),
				"downloaded": downloadedFiles,
			})
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"thread":     thread,
			"downloaded": downloadedFiles,
		})
	}
	if thread == nil || len(thread.Messages) == 0 {
		u.Err().Println("Empty thread")
		return nil
	}

	// Show message count upfront so users know how many messages to expect
	u.Out().Linef("Thread contains %d message(s)", len(thread.Messages))
	u.Out().Println("")

	for i, msg := range thread.Messages {
		if msg == nil {
			continue
		}
		u.Out().Linef("=== Message %d/%d: %s ===", i+1, len(thread.Messages), msg.Id)
		header := func(name string) string {
			value := headerValue(msg.Payload, name)
			if c.SanitizeContent {
				return sanitizeGmailText(value)
			}
			return value
		}
		u.Out().Linef("From: %s", header("From"))
		u.Out().Linef("To: %s", header("To"))
		u.Out().Linef("Subject: %s", header("Subject"))
		u.Out().Linef("Date: %s", header("Date"))
		u.Out().Println("")

		body, isHTML := gmailcontent.BestBodyForDisplay(msg.Payload)
		if body != "" {
			cleanBody := body
			if c.SanitizeContent {
				cleanBody = sanitizeGmailBody(body, isHTML)
			} else if isHTML {
				cleanBody = gmailcontent.StripHTMLTags(body)
			}
			// Limit body preview to avoid overwhelming output
			// Use runes to avoid breaking multi-byte UTF-8 characters
			runes := []rune(cleanBody)
			if len(runes) > 500 && !c.Full {
				cleanBody = string(runes[:500]) + gmailTextTruncationMarker
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
					u.Out().Linef("Cached: %s", a.Path)
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
	threadID := strings.TrimSpace(c.ThreadID)
	threadID = normalizeGmailThreadID(threadID)
	if threadID == "" {
		return usage("empty threadId")
	}

	addLabels := splitCSV(c.Add)
	removeLabels := splitCSV(c.Remove)
	if len(addLabels) == 0 && len(removeLabels) == 0 {
		return usage("must specify --add and/or --remove")
	}

	if err := dryRunExit(ctx, flags, "gmail.thread.modify", map[string]any{
		"thread_id": threadID,
		"add":       addLabels,
		"remove":    removeLabels,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	addIDs, removeIDs, err := resolveModifyLabelIDs(svc, addLabels, removeLabels)
	if err != nil {
		return err
	}

	// Use Gmail's Threads.Modify API
	_, err = svc.Users.Threads.Modify("me", threadID, &gmail.ModifyThreadRequest{
		AddLabelIds:    addIDs,
		RemoveLabelIds: removeIDs,
	}).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"modified":      threadID,
			"addedLabels":   addIDs,
			"removedLabels": removeIDs,
		})
	}

	u.Out().Linef("Modified thread %s", threadID)
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
	threadID = normalizeGmailThreadID(threadID)
	if threadID == "" {
		return usage("empty threadId")
	}

	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	thread, err := svc.Users.Threads.Get("me", threadID).Format("full").Context(ctx).Do()
	if err != nil {
		return err
	}

	if thread == nil || len(thread.Messages) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
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

	allAttachments := make([]attachmentDownloadOutput, 0)
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"threadId":    threadID,
			"attachments": allAttachments,
		})
	}

	if len(allAttachments) == 0 {
		u.Out().Println("No attachments found")
		return nil
	}

	u.Out().Linef("Found %d attachment(s):", len(allAttachments))
	if c.Download {
		for _, a := range allAttachments {
			status := "Saved"
			if a.Cached {
				status = "Cached"
			}
			u.Out().Linef("  %s: %s (%s) - %s", status, a.Filename, a.SizeHuman, a.Path)
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
			id = normalizeGmailThreadID(id)
			urls = append(urls, map[string]string{
				"id":  id,
				"url": fmt.Sprintf("https://mail.google.com/mail/?authuser=%s#all/%s", url.QueryEscape(account), id),
			})
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"urls": urls})
	}
	for _, id := range c.ThreadIDs {
		id = normalizeGmailThreadID(id)
		threadURL := fmt.Sprintf("https://mail.google.com/mail/?authuser=%s#all/%s", url.QueryEscape(account), id)
		u.Out().Linef("%s\t%s", id, threadURL)
	}
	return nil
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
