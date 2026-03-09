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

type GmailSearchCmd struct {
	Query     []string `arg:"" name:"query" help:"Search query"`
	Max       int64    `name:"max" aliases:"limit" help:"Max results" default:"10"`
	Page      string   `name:"page" aliases:"cursor" help:"Page token"`
	All       bool     `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	Oldest    bool     `name:"oldest" help:"Show first message date instead of last"`
	Timezone  string   `name:"timezone" short:"z" help:"Output timezone (IANA name, e.g. America/New_York, UTC). Default: local"`
	Local     bool     `name:"local" help:"Use local timezone (default behavior, useful to override --timezone)"`
}

func (c *GmailSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(c.Query, " "))
	if query == "" {
		return usage("missing query")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*gmail.Thread, string, error) {
		call := svc.Users.Threads.List("me").
			Q(query).
			MaxResults(c.Max).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Threads, resp.NextPageToken, nil
	}

	var threads []*gmail.Thread
	nextPageToken := ""
	if c.All {
		all, collectErr := collectAllPages(c.Page, fetch)
		if collectErr != nil {
			return collectErr
		}
		threads = all
	} else {
		threadsPage, pageToken, fetchErr := fetch(c.Page)
		if fetchErr != nil {
			return fetchErr
		}
		threads = threadsPage
		nextPageToken = pageToken
	}

	if len(threads) == 0 {
		if outfmt.IsJSON(ctx) {
			if writeErr := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
				"threads":       []threadItem{},
				"nextPageToken": nextPageToken,
			}); writeErr != nil {
				return writeErr
			}
			return failEmptyExit(c.FailEmpty)
		}
		u.Err().Println("No results")
		return failEmptyExit(c.FailEmpty)
	}

	idToName, err := fetchLabelIDToName(svc)
	if err != nil {
		return err
	}

	loc, err := resolveOutputLocation(c.Timezone, c.Local)
	if err != nil {
		return err
	}

	items, err := fetchThreadDetails(ctx, svc, threads, idToName, c.Oldest, loc)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		if writeErr := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"threads":       items,
			"nextPageToken": nextPageToken,
		}); writeErr != nil {
			return writeErr
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(items) == 0 {
		u.Err().Println("No results")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintln(w, "ID\tDATE\tFROM\tSUBJECT\tLABELS\tTHREAD")
	for _, it := range items {
		threadInfo := "-"
		if it.MessageCount > 1 {
			threadInfo = fmt.Sprintf("[%d msgs]", it.MessageCount)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", it.ID, it.Date, it.From, it.Subject, strings.Join(it.Labels, ","), threadInfo)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}
