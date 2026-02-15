package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ChatMessagesCmd struct {
	List ChatMessagesListCmd `cmd:"" name:"list" aliases:"ls" help:"List messages"`
	Send ChatMessagesSendCmd `cmd:"" name:"send" aliases:"create,post" help:"Send a message"`
}

type ChatMessagesListCmd struct {
	Space     string `arg:"" name:"space" help:"Space name (spaces/...)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	Order     string `name:"order" help:"Order by (e.g. createTime desc)"`
	Thread    string `name:"thread" help:"Filter by thread (spaces/.../threads/...)"`
	Unread    bool   `name:"unread" help:"Only messages after last read time"`
}

func (c *ChatMessagesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
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

	filters := make([]string, 0, 2)
	thread := strings.TrimSpace(c.Thread)
	if thread != "" {
		threadName, threadErr := normalizeThread(space, thread)
		if threadErr != nil {
			return usage(fmt.Sprintf("invalid thread: %v", threadErr))
		}
		filters = append(filters, fmt.Sprintf("thread.name = \"%s\"", threadName))
	}
	if c.Unread {
		readState, readErr := svc.Users.Spaces.GetSpaceReadState(fmt.Sprintf("users/me/spaces/%s/spaceReadState", spaceID(space))).Do()
		if readErr != nil {
			return readErr
		}
		if readState.LastReadTime != "" {
			filters = append(filters, fmt.Sprintf("createTime > \"%s\"", readState.LastReadTime))
		}
	}
	filter := strings.Join(filters, " AND ")

	fetch := func(pageToken string) ([]*chat.Message, string, error) {
		call := svc.Spaces.Messages.List(space).
			PageSize(c.Max).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		if strings.TrimSpace(c.Order) != "" {
			call = call.OrderBy(c.Order)
		}
		if filter != "" {
			call = call.Filter(filter)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Messages, resp.NextPageToken, nil
	}

	var messages []*chat.Message
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		messages = all
	} else {
		var err error
		messages, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource   string `json:"resource"`
			Sender     string `json:"sender,omitempty"`
			Text       string `json:"text,omitempty"`
			CreateTime string `json:"createTime,omitempty"`
			Thread     string `json:"thread,omitempty"`
		}
		items := make([]item, 0, len(messages))
		for _, msg := range messages {
			if msg == nil {
				continue
			}
			items = append(items, item{
				Resource:   msg.Name,
				Sender:     chatMessageSender(msg),
				Text:       chatMessageText(msg),
				CreateTime: msg.CreateTime,
				Thread:     chatMessageThread(msg),
			})
		}
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"messages":      items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(messages) == 0 {
		u.Err().Println("No messages")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tSENDER\tTIME\tTEXT")
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			msg.Name,
			sanitizeTab(chatMessageSender(msg)),
			sanitizeTab(msg.CreateTime),
			sanitizeChatText(chatMessageText(msg)),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type ChatMessagesSendCmd struct {
	Space  string `arg:"" name:"space" help:"Space name (spaces/...)"`
	Text   string `name:"text" help:"Message text (required)"`
	Thread string `name:"thread" help:"Reply to thread (spaces/.../threads/...)"`
}

func (c *ChatMessagesSendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	space, err := normalizeSpace(c.Space)
	if err != nil {
		return usage("required: space")
	}

	text := strings.TrimSpace(c.Text)
	if text == "" {
		return usage("required: --text")
	}

	message := &chat.Message{Text: text}
	thread := strings.TrimSpace(c.Thread)
	threadName := ""
	if thread != "" {
		tn, threadErr := normalizeThread(space, thread)
		if threadErr != nil {
			return usage(fmt.Sprintf("invalid thread: %v", threadErr))
		}
		threadName = tn
		message.Thread = &chat.Thread{Name: tn}
	}

	if dryRunErr := dryRunExit(ctx, flags, "chat.messages.send", map[string]any{
		"space":                        space,
		"text":                         text,
		"thread":                       threadName,
		"thread_raw":                   thread,
		"reply_fallback_to_new_thread": thread != "",
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Spaces.Messages.Create(space, message)
	if thread != "" {
		call = call.MessageReplyOption("REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD")
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"message": resp})
	}

	if resp == nil {
		u.Out().Printf("space\t%s", space)
		return nil
	}
	if resp.Name != "" {
		u.Out().Printf("resource\t%s", resp.Name)
	}
	if resp.Thread != nil && resp.Thread.Name != "" {
		u.Out().Printf("thread\t%s", resp.Thread.Name)
	}
	return nil
}
