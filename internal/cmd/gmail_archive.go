package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// GmailArchiveCmd archives messages (removes INBOX label).
type GmailArchiveCmd struct {
	MessageIDs []string `arg:"" optional:"" name:"messageId" help:"Message IDs to archive"`
	Query      string   `name:"query" short:"q" help:"Archive all messages matching this Gmail search query"`
	Max        int64    `name:"max" aliases:"limit" help:"Max messages to archive (with --query)" default:"100"`
}

func (c *GmailArchiveCmd) Run(ctx context.Context, flags *RootFlags) error {
	return gmailBulkLabelOp(ctx, flags, c.MessageIDs, c.Query, c.Max, nil, []string{"INBOX"}, "archived", "gmail.archive")
}

// GmailTrashMsgCmd moves messages to trash.
type GmailTrashMsgCmd struct {
	MessageIDs []string `arg:"" optional:"" name:"messageId" help:"Message IDs to trash"`
	Query      string   `name:"query" short:"q" help:"Trash all messages matching this Gmail search query"`
	Max        int64    `name:"max" aliases:"limit" help:"Max messages to trash (with --query)" default:"100"`
}

func (c *GmailTrashMsgCmd) Run(ctx context.Context, flags *RootFlags) error {
	return gmailBulkLabelOp(ctx, flags, c.MessageIDs, c.Query, c.Max, []string{"TRASH"}, []string{"INBOX"}, "trashed", "gmail.trash")
}

// GmailReadCmd marks messages as read.
type GmailReadCmd struct {
	MessageIDs []string `arg:"" optional:"" name:"messageId" help:"Message IDs to mark as read"`
	Query      string   `name:"query" short:"q" help:"Mark all messages matching this query as read"`
	Max        int64    `name:"max" aliases:"limit" help:"Max messages (with --query)" default:"100"`
}

func (c *GmailReadCmd) Run(ctx context.Context, flags *RootFlags) error {
	return gmailBulkLabelOp(ctx, flags, c.MessageIDs, c.Query, c.Max, nil, []string{"UNREAD"}, "marked as read", "gmail.read")
}

// GmailUnreadCmd marks messages as unread.
type GmailUnreadCmd struct {
	MessageIDs []string `arg:"" optional:"" name:"messageId" help:"Message IDs to mark as unread"`
	Query      string   `name:"query" short:"q" help:"Mark all messages matching this query as unread"`
	Max        int64    `name:"max" aliases:"limit" help:"Max messages (with --query)" default:"100"`
}

func (c *GmailUnreadCmd) Run(ctx context.Context, flags *RootFlags) error {
	return gmailBulkLabelOp(ctx, flags, c.MessageIDs, c.Query, c.Max, []string{"UNREAD"}, nil, "marked as unread", "gmail.unread")
}

// gmailBulkLabelOp handles the common pattern: resolve IDs (from args or query), then batch modify labels.
func gmailBulkLabelOp(ctx context.Context, flags *RootFlags, messageIDs []string, query string, limit int64, addLabels, removeLabels []string, verb string, dryRunOp string) error {
	u := ui.FromContext(ctx)

	idsFromArgs := make([]string, 0, len(messageIDs))
	for _, id := range messageIDs {
		id = normalizeGmailMessageID(id)
		if id != "" {
			idsFromArgs = append(idsFromArgs, id)
		}
	}

	if len(idsFromArgs) == 0 && query == "" {
		return usage("provide message IDs or --query")
	}

	if err := dryRunExit(ctx, flags, dryRunOp, map[string]any{
		"message_ids":    idsFromArgs,
		"query":          strings.TrimSpace(query),
		"max":            limit,
		"added_labels":   nonNilStrings(addLabels),
		"removed_labels": nonNilStrings(removeLabels),
		"action":         verb,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Collect IDs: either from args or by searching
	ids := make([]string, 0, len(idsFromArgs))
	if query != "" {
		ids, err = searchMessageIDs(ctx, svc, query, limit)
		if err != nil {
			return err
		}
	}
	ids = append(ids, idsFromArgs...)

	if len(ids) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
				"action": verb,
				"count":  0,
			})
		}
		u.Err().Println("No messages found")
		return nil
	}

	// Resolve label names to IDs
	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}
	addIDs := resolveLabelIDs(addLabels, idMap)
	removeIDs := resolveLabelIDs(removeLabels, idMap)

	// Batch modify in chunks of 1000 (API limit)
	total := 0
	for i := 0; i < len(ids); i += 1000 {
		end := i + 1000
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]

		req := &gmail.BatchModifyMessagesRequest{
			Ids: chunk,
		}
		if len(addIDs) > 0 {
			req.AddLabelIds = addIDs
		}
		if len(removeIDs) > 0 {
			req.RemoveLabelIds = removeIDs
		}

		if err := svc.Users.Messages.BatchModify("me", req).Do(); err != nil {
			return fmt.Errorf("batch modify failed at offset %d: %w", i, err)
		}
		total += len(chunk)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"action":        verb,
			"count":         total,
			"addedLabels":   addLabels,
			"removedLabels": removeLabels,
		})
	}

	u.Out().Printf("%s %d message%s", capitalizeFirst(verb), total, pluralS(total))
	return nil
}

// searchMessageIDs returns message IDs matching a Gmail query.
func searchMessageIDs(ctx context.Context, svc *gmail.Service, query string, limit int64) ([]string, error) {
	var ids []string
	pageToken := ""
	remaining := limit

	for {
		batchSize := remaining
		if batchSize > 500 {
			batchSize = 500
		}

		call := svc.Users.Messages.List("me").
			Q(query).
			MaxResults(batchSize).
			Fields("messages(id),nextPageToken").
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, err
		}

		for _, m := range resp.Messages {
			if m != nil && m.Id != "" {
				ids = append(ids, m.Id)
			}
		}

		remaining -= int64(len(resp.Messages))
		if resp.NextPageToken == "" || remaining <= 0 {
			break
		}
		pageToken = resp.NextPageToken
	}

	return ids, nil
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
