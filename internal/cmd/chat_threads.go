package cmd

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ChatThreadsCmd struct {
	List ChatThreadsListCmd `cmd:"" name:"list" help:"List threads in a space"`
}

type ChatThreadsListCmd struct {
	Space string `arg:"" name:"space" help:"Space name (spaces/...)"`
	Max   int64  `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page  string `name:"page" help:"Page token"`
}

func (c *ChatThreadsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	space, err := normalizeSpace(c.Space)
	if err != nil {
		return usage("required: space")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spaces.Messages.List(space).
		PageSize(c.Max).
		PageToken(c.Page).
		OrderBy("createTime desc").
		Do()
	if err != nil {
		return err
	}

	threads := make([]*chatMessageThreadItem, 0, len(resp.Messages))
	seen := make(map[string]bool)
	for _, msg := range resp.Messages {
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"threads":       items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(threads) == 0 {
		u.Err().Println("No threads")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "THREAD\tMESSAGE\tSENDER\tTIME\tTEXT")
	for _, item := range threads {
		if item == nil || item.message == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			item.thread,
			item.message.Name,
			sanitizeTab(chatMessageSender(item.message)),
			sanitizeTab(item.message.CreateTime),
			sanitizeChatText(chatMessageText(item.message)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type chatMessageThreadItem struct {
	thread  string
	message *chat.Message
}
