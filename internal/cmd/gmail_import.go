package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/mail"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"
	gapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type GmailImportCmd struct {
	File               string   `arg:"" name:"file" help:"RFC822/EML file path, or '-' for stdin"`
	Labels             []string `name:"label" help:"Label ID or name to apply (repeatable)"`
	InternalDateSource string   `name:"internal-date-source" enum:"dateHeader,receivedTime" default:"dateHeader" help:"Gmail internal date source: dateHeader or receivedTime"`
	NeverMarkSpam      bool     `name:"never-mark-spam" help:"Never classify the imported message as spam"`
	ProcessForCalendar bool     `name:"process-for-calendar" help:"Process calendar invitations in the imported message"`
}

type gmailImportPlan struct {
	Source             string   `json:"source"`
	Bytes              int      `json:"bytes"`
	Subject            string   `json:"subject,omitempty"`
	From               string   `json:"from,omitempty"`
	To                 string   `json:"to,omitempty"`
	Date               string   `json:"date,omitempty"`
	MessageID          string   `json:"message_id,omitempty"`
	Labels             []string `json:"labels,omitempty"`
	InternalDateSource string   `json:"internal_date_source"`
	NeverMarkSpam      bool     `json:"never_mark_spam"`
	ProcessForCalendar bool     `json:"process_for_calendar"`
}

func (c *GmailImportCmd) Run(ctx context.Context, flags *RootFlags) error {
	raw, plan, err := c.readAndPlan(ctx)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.import", plan); dryRunErr != nil {
		return dryRunErr
	}
	if googleapi.ReadOnly(ctx) {
		return fmt.Errorf("%w: Gmail message import is disabled", googleapi.ErrReadOnly)
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	labelIDs := []string(nil)
	if len(c.Labels) > 0 {
		nameToID, labelsErr := fetchLabelNameToID(svc)
		if labelsErr != nil {
			return labelsErr
		}
		labelIDs = resolveLabelIDs(c.Labels, nameToID)
	}

	call := svc.Users.Messages.Import("me", &gmail.Message{LabelIds: labelIDs}).
		InternalDateSource(c.InternalDateSource).
		Media(bytes.NewReader(raw), gapi.ContentType("message/rfc822")).
		Context(ctx)
	if c.NeverMarkSpam {
		call = call.NeverMarkSpam(true)
	}
	if c.ProcessForCalendar {
		call = call.ProcessForCalendar(true)
	}

	message, err := call.Do()
	if err != nil {
		return err
	}
	return writeGmailImportResult(ctx, message)
}

func (c *GmailImportCmd) readAndPlan(ctx context.Context) ([]byte, gmailImportPlan, error) {
	source := strings.TrimSpace(c.File)
	raw, message, err := readRFC822Input(ctx, source)
	if err != nil {
		return nil, gmailImportPlan{}, err
	}

	plan := gmailImportPlan{
		Source:             source,
		Bytes:              len(raw),
		Subject:            message.Header.Get("Subject"),
		From:               message.Header.Get("From"),
		To:                 message.Header.Get("To"),
		Date:               message.Header.Get("Date"),
		MessageID:          message.Header.Get("Message-ID"),
		Labels:             normalizedImportLabels(c.Labels),
		InternalDateSource: c.InternalDateSource,
		NeverMarkSpam:      c.NeverMarkSpam,
		ProcessForCalendar: c.ProcessForCalendar,
	}
	return raw, plan, nil
}

func readRFC822Input(ctx context.Context, source string) ([]byte, *mail.Message, error) {
	return readRFC822InputWithLimit(ctx, source, 0)
}

// A zero limit retains the import path's existing behavior. Raw send and draft
// inputs use a bounded reader, including for stdin and non-regular files.
func readRFC822InputWithLimit(ctx context.Context, source string, limit int64) ([]byte, *mail.Message, error) {
	if source == "" {
		return nil, nil, usage("RFC822/EML file is required")
	}

	reader := stdinReader(ctx)
	if source != "-" {
		path, expandErr := config.ExpandPath(source)
		if expandErr != nil {
			return nil, nil, expandErr
		}
		file, err := os.Open(path) //nolint:gosec // user-provided RFC822 message path
		if err != nil {
			return nil, nil, err
		}
		defer file.Close()
		reader = file
	}
	if limit > 0 {
		reader = io.LimitReader(reader, limit+1)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, nil, err
	}
	if limit > 0 && int64(len(raw)) > limit {
		return nil, nil, usagef("RFC822/EML input exceeds maximum size of %d bytes (35 MiB)", limit)
	}
	if len(raw) == 0 {
		return nil, nil, usage("RFC822/EML input is empty")
	}

	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		// Parser errors may quote entire header lines. Never expose message
		// content through diagnostics, including failed offline previews.
		return nil, nil, usage("invalid RFC822/EML input: malformed message headers")
	}

	return raw, message, nil
}

func normalizedImportLabels(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		if label = strings.TrimSpace(label); label != "" {
			out = append(out, label)
		}
	}
	return out
}

func writeGmailImportResult(ctx context.Context, message *gmail.Message) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"messageId":    message.Id,
			"threadId":     message.ThreadId,
			"labelIds":     message.LabelIds,
			"internalDate": message.InternalDate,
		})
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("message_id\t%s", message.Id)
	if message.ThreadId != "" {
		u.Out().Linef("thread_id\t%s", message.ThreadId)
	}
	return nil
}
