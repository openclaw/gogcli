package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newYouTubeService = googleapi.NewYouTube

type YouTubeCmd struct {
	Accounts YouTubeAccountsCmd `cmd:"" name:"accounts" help:"List YouTube accounts (managed channels)"`
	Channels YouTubeChannelsCmd `cmd:"" name:"channels" help:"List YouTube channels (owned by user)"`
}

type YouTubeAccountsCmd struct {
	User string `name:"user" help:"User email to list accounts for"`
	Max  int64  `name:"max" aliases:"limit" default:"50" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *YouTubeAccountsCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	svc, err := newYouTubeService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Channels.List([]string{"snippet"}).ManagedByMe(true)
	if c.Max > 0 {
		call = call.MaxResults(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list youtube accounts: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Items) == 0 {
		ui.FromContext(ctx).Err().Println("No YouTube accounts found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "CHANNEL ID\tTITLE\tCUSTOM URL")
	for _, ch := range resp.Items {
		if ch == nil {
			continue
		}
		title := ""
		customURL := ""
		if ch.Snippet != nil {
			title = ch.Snippet.Title
			customURL = ch.Snippet.CustomUrl
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", sanitizeTab(ch.Id), sanitizeTab(title), sanitizeTab(customURL))
	}
	printNextPageHint(ui.FromContext(ctx), resp.NextPageToken)
	return nil
}

type YouTubeChannelsCmd struct {
	User string `name:"user" help:"User email to list channels for"`
	Max  int64  `name:"max" aliases:"limit" default:"50" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *YouTubeChannelsCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	svc, err := newYouTubeService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Channels.List([]string{"snippet"}).Mine(true)
	if c.Max > 0 {
		call = call.MaxResults(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list youtube channels: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Items) == 0 {
		ui.FromContext(ctx).Err().Println("No YouTube channels found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "CHANNEL ID\tTITLE\tCUSTOM URL")
	for _, ch := range resp.Items {
		if ch == nil {
			continue
		}
		title := ""
		customURL := ""
		if ch.Snippet != nil {
			title = ch.Snippet.Title
			customURL = ch.Snippet.CustomUrl
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", sanitizeTab(ch.Id), sanitizeTab(title), sanitizeTab(customURL))
	}
	printNextPageHint(ui.FromContext(ctx), resp.NextPageToken)
	return nil
}
