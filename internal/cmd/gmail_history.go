package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailHistoryCmd struct {
	Since     string `name:"since" help:"Start history ID"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
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

	historyID := ""
	fetch := func(pageToken string) ([]string, string, error) {
		call := svc.Users.History.List("me").StartHistoryId(startID).MaxResults(c.Max)
		call.HistoryTypes("messageAdded")
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Context(ctx).Do()
		if err != nil {
			return nil, "", err
		}
		historyID = formatHistoryID(resp.HistoryId)
		return collectHistoryMessageIDs(resp), resp.NextPageToken, nil
	}

	var ids []string
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		ids = all
	} else {
		var err error
		ids, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"historyId":     historyID,
			"messages":      ids,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(ids) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}
	if len(ids) == 0 {
		u.Err().Println("No history")
		return failEmptyExit(c.FailEmpty)
	}
	u.Out().Println("MESSAGE_ID")
	for _, id := range ids {
		u.Out().Println(id)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}
