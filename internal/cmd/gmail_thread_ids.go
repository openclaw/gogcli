package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"

	api "github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
)

type GmailThreadIDsCmd struct {
	ThreadID string `arg:"" name:"threadId" help:"Thread ID"`
}
type gmailThreadIDsSnapshot struct {
	ThreadID string             `json:"thread_id"`
	Messages []gmailThreadIDRow `json:"messages"`
}
type gmailThreadIDRow struct {
	ID              string   `json:"id"`
	ThreadID        string   `json:"thread_id"`
	InternalDateISO string   `json:"internalDateIso,omitempty"`
	Subject         string   `json:"subject,omitempty"`
	From            string   `json:"from,omitempty"`
	To              string   `json:"to,omitempty"`
	Cc              string   `json:"cc,omitempty"`
	Date            string   `json:"date,omitempty"`
	Snippet         string   `json:"snippet,omitempty"`
	TruncatedFields []string `json:"truncated_fields,omitempty"`
}

func (c *GmailThreadIDsCmd) Run(ctx context.Context, flags *RootFlags) error {
	if !gmailExportIDValid(c.ThreadID) {
		return usage("invalid thread ID")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	ctx = api.WithResponseByteLimit(ctx, 12<<20)
	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}
	snapshot, err := fetchGmailThreadIDs(ctx, svc, c.ThreadID)
	if err != nil {
		return err
	}
	if _, enabled := outfmt.UntrustedWrapperFromContext(ctx); enabled {
		for i := range snapshot.Messages {
			snapshot.Messages[i] = wrapMCPThreadRow(snapshot.Messages[i])
		}
	}
	return outfmt.WriteJSON(ctx, stdoutWriter(ctx), snapshot)
}

func fetchGmailThreadIDs(ctx context.Context, svc *gmail.Service, threadID string) (gmailThreadIDsSnapshot, error) {
	snapshot := gmailThreadIDsSnapshot{ThreadID: threadID, Messages: []gmailThreadIDRow{}}
	thread, err := svc.Users.Threads.Get("me", threadID).Format("metadata").MetadataHeaders("Date", "Subject", "From", "To", "Cc").Fields(googleapi.Field("id,messages(id,threadId,internalDate,snippet,payload/headers)")).Context(ctx).Do()
	if err != nil {
		return snapshot, err
	}
	if thread == nil || thread.Id != threadID || len(thread.Messages) > 10000 {
		return snapshot, fmt.Errorf("thread metadata identity or row limit violated")
	}
	seen := map[string]bool{}
	for _, msg := range thread.Messages {
		if msg == nil || !gmailExportIDValid(msg.Id) || msg.ThreadId != threadID || seen[msg.Id] {
			return snapshot, fmt.Errorf("missing, duplicate or mismatched message identity")
		}
		seen[msg.Id] = true
		row := gmailThreadIDRow{ID: msg.Id, ThreadID: threadID, InternalDateISO: formatGmailDateISO(msg.InternalDate, time.UTC)}
		for _, field := range []struct {
			name string
			dest *string
		}{{"Subject", &row.Subject}, {"From", &row.From}, {"To", &row.To}, {"Cc", &row.Cc}, {"Date", &row.Date}, {"Snippet", &row.Snippet}} {
			value := msg.Snippet
			if field.name != "Snippet" {
				value = headerValue(msg.Payload, field.name)
			}
			var cut bool
			*field.dest, cut = truncateMCPText(value, 4096)
			if cut {
				row.TruncatedFields = append(row.TruncatedFields, field.name)
			}
		}
		snapshot.Messages = append(snapshot.Messages, row)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil || len(encoded) > 8<<20 {
		return gmailThreadIDsSnapshot{}, fmt.Errorf("thread metadata exceeds byte limit")
	}
	return snapshot, nil
}

func truncateMCPText(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit], true
}

func wrapMCPThreadRow(row gmailThreadIDRow) gmailThreadIDRow {
	for _, field := range []*string{&row.Subject, &row.From, &row.To, &row.Cc, &row.Date, &row.Snippet} {
		if *field != "" {
			*field = outfmt.WrapUntrustedContent(*field, outfmt.UntrustedWrapOptions{Enabled: true, Source: "google_api"})
		}
	}
	return row
}
