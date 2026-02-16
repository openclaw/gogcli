package cmd

import (
	"context"
	"errors"
	"os"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailBatchCmd struct {
	Delete GmailBatchDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Permanently delete multiple messages"`
	Modify GmailBatchModifyCmd `cmd:"" name:"modify" aliases:"update,edit,set" help:"Modify labels on multiple messages"`
}

type GmailBatchDeleteCmd struct {
	MessageIDs []string `arg:"" name:"messageId" help:"Message IDs"`
}

func (c *GmailBatchDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	ids := make([]string, 0, len(c.MessageIDs))
	for _, id := range c.MessageIDs {
		id = normalizeGmailMessageID(id)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return usage("missing messageId")
	}

	if confirmErr := confirmDestructive(ctx, flags, "permanently delete gmail messages"); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	err = svc.Users.Messages.BatchDelete("me", &gmail.BatchDeleteMessagesRequest{
		Ids: ids,
	}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"deleted": ids,
			"count":   len(ids),
		})
	}

	u.Out().Printf("Deleted %d messages", len(ids))
	return nil
}

type GmailBatchModifyCmd struct {
	MessageIDs []string `arg:"" name:"messageId" help:"Message IDs"`
	Add        string   `name:"add" help:"Labels to add (comma-separated, name or ID)"`
	Remove     string   `name:"remove" help:"Labels to remove (comma-separated, name or ID)"`
}

func (c *GmailBatchModifyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	ids := make([]string, 0, len(c.MessageIDs))
	for _, id := range c.MessageIDs {
		id = normalizeGmailMessageID(id)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return usage("missing messageId")
	}
	addLabels := splitCSV(c.Add)
	removeLabels := splitCSV(c.Remove)
	if len(addLabels) == 0 && len(removeLabels) == 0 {
		return errors.New("must specify --add and/or --remove")
	}

	if err := dryRunExit(ctx, flags, "gmail.batch.modify", map[string]any{
		"message_ids": ids,
		"add":         addLabels,
		"remove":      removeLabels,
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

	idMap, err := fetchLabelNameToID(svc)
	if err != nil {
		return err
	}

	addIDs := resolveLabelIDs(addLabels, idMap)
	removeIDs := resolveLabelIDs(removeLabels, idMap)

	err = svc.Users.Messages.BatchModify("me", &gmail.BatchModifyMessagesRequest{
		Ids:            ids,
		AddLabelIds:    addIDs,
		RemoveLabelIds: removeIDs,
	}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"modified":      ids,
			"count":         len(ids),
			"addedLabels":   addIDs,
			"removedLabels": removeIDs,
		})
	}

	u.Out().Printf("Modified %d messages", len(ids))
	return nil
}
