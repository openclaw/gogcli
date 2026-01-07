package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailHistoryCmd struct {
	Since string `name:"since" help:"Start history ID"`
	Max   int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page  string `name:"page" help:"Page token"`
}

func (c *GmailHistoryCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.Since) == "" {
		return usage("--since is required")
	}
	startID, err := parseHistoryID(c.Since)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Users.History.List("me").StartHistoryId(startID).MaxResults(c.Max)
	call.HistoryTypes("messageAdded")
	if strings.TrimSpace(c.Page) != "" {
		call.PageToken(c.Page)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	ids := collectHistoryMessageIDs(resp)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"historyId":     formatHistoryID(resp.HistoryId),
			"messages":      ids,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(ids) == 0 {
		u.Err().Println("No history")
		return nil
	}
	u.Out().Println("MESSAGE_ID")
	for _, id := range ids {
		u.Out().Println(id)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}
