package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ChatThreadsCmd struct {
	List ChatThreadsListCmd `cmd:"" name:"list" help:"List threads in a space"`
}

type ChatThreadsListCmd struct {
	Space     string `arg:"" name:"space" help:"Space name (spaces/...)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *ChatThreadsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	space, err := normalizeSpace(c.Space)
	if err != nil {
		return usage("required: space")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	svc, err := chatService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*chat.Message, string, error) {
		call := svc.Spaces.Messages.List(space).
			PageSize(c.Max).
			OrderBy("createTime desc").
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Messages, resp.NextPageToken, nil
	}

	messages, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}

	threads := make([]*chatMessageThreadItem, 0, len(messages))
	seen := make(map[string]bool)
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		threadName := chatMessageThread(msg)
		if threadName == "" {
			continue
		}
		if seen[threadName] {
			continue
		}
		seen[threadName] = true
		threads = append(threads, &chatMessageThreadItem{message: msg, thread: threadName})
	}

	if outfmt.IsJSON(ctx) {
		items := make([]map[string]any, 0, len(threads))
		for _, item := range threads {
			if item == nil || item.message == nil {
				continue
			}
			items = append(items, map[string]any{
				"thread":     item.thread,
				"message":    item.message.Name,
				"sender":     chatMessageSender(item.message),
				"text":       chatMessageText(item.message),
				"createTime": item.message.CreateTime,
			})
		}
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"threads":       items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(threads) == 0 {
		u.Err().Println("No threads")
		return failEmptyExit(c.FailEmpty)
	}

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactChatThreadRows(threads),
		chatThreadColumns(),
	); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type chatMessageThreadItem struct {
	thread  string
	message *chat.Message
}
