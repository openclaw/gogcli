package cmd

import (
	"context"
	"os"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// ── archive ──────────────────────────────────────────────────────────────────

type GmailArchiveCmd struct {
	MessageIDs []string `arg:"" name:"messageId" help:"Message IDs to archive"`
}

func (c *GmailArchiveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if len(c.MessageIDs) == 0 {
		return usage("at least one message ID is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	err = svc.Users.Messages.BatchModify("me", &gmail.BatchModifyMessagesRequest{
		Ids:            c.MessageIDs,
		RemoveLabelIds: []string{"INBOX"},
	}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"archived": c.MessageIDs,
			"count":    len(c.MessageIDs),
		})
	}

	u.Out().Printf("Archived %d messages", len(c.MessageIDs))
	return nil
}

// ── delete (trash) ───────────────────────────────────────────────────────────

type GmailDeleteCmd struct {
	MessageIDs []string `arg:"" name:"messageId" help:"Message IDs to move to Trash"`
}

func (c *GmailDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if len(c.MessageIDs) == 0 {
		return usage("at least one message ID is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	err = svc.Users.Messages.BatchModify("me", &gmail.BatchModifyMessagesRequest{
		Ids:         c.MessageIDs,
		AddLabelIds: []string{"TRASH"},
	}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"trashed": c.MessageIDs,
			"count":   len(c.MessageIDs),
		})
	}

	u.Out().Printf("Moved %d messages to Trash", len(c.MessageIDs))
	return nil
}

// ── label ────────────────────────────────────────────────────────────────────

type GmailLabelCmd struct {
	MessageIDs []string `arg:"" name:"messageId" help:"Message IDs to modify"`
	Add        string   `name:"add" help:"Labels to add (comma-separated, name or ID)"`
	Remove     string   `name:"remove" help:"Labels to remove (comma-separated, name or ID)"`
}

func (c *GmailLabelCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if len(c.MessageIDs) == 0 {
		return usage("at least one message ID is required")
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

	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}

	addIDs := resolveLabelIDs(addLabels, idMap)
	removeIDs := resolveLabelIDs(removeLabels, idMap)

	err = svc.Users.Messages.BatchModify("me", &gmail.BatchModifyMessagesRequest{
		Ids:            c.MessageIDs,
		AddLabelIds:    addIDs,
		RemoveLabelIds: removeIDs,
	}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"modified":      c.MessageIDs,
			"count":         len(c.MessageIDs),
			"addedLabels":   addIDs,
			"removedLabels": removeIDs,
		})
	}

	u.Out().Printf("Modified %d messages", len(c.MessageIDs))
	return nil
}

// ── mark-read / mark-unread ──────────────────────────────────────────────────

type GmailMarkReadCmd struct {
	MessageIDs []string `arg:"" name:"messageId" help:"Message IDs to mark as read or unread"`
	Unread     bool     `name:"unread" help:"Mark as unread instead of read"`
}

func (c *GmailMarkReadCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if len(c.MessageIDs) == 0 {
		return usage("at least one message ID is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	req := &gmail.BatchModifyMessagesRequest{Ids: c.MessageIDs}
	if c.Unread {
		req.AddLabelIds = []string{"UNREAD"}
	} else {
		req.RemoveLabelIds = []string{"UNREAD"}
	}

	err = svc.Users.Messages.BatchModify("me", req).Do()
	if err != nil {
		return err
	}

	action := "read"
	if c.Unread {
		action = "unread"
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"marked": c.MessageIDs,
			"count":  len(c.MessageIDs),
			"status": action,
		})
	}

	u.Out().Printf("Marked %d messages as %s", len(c.MessageIDs), action)
	return nil
}
