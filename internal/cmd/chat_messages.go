package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ChatMessagesCmd struct {
	List      ChatMessagesListCmd      `cmd:"" name:"list" aliases:"ls" help:"List messages"`
	Send      ChatMessagesSendCmd      `cmd:"" name:"send" aliases:"create,post" help:"Send a message"`
	React     ChatMessagesReactCmd     `cmd:"" name:"react" help:"Add an emoji reaction to a message"`
	Reactions ChatMessagesReactionsCmd `cmd:"" name:"reactions" aliases:"reaction" help:"Manage emoji reactions on a message"`
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
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
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

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactChatRows(messages),
		chatMessageColumns(),
	); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type ChatMessagesSendCmd struct {
	Space  string   `arg:"" name:"space" help:"Space name (spaces/...)"`
	Text   string   `name:"text" help:"Message text (required unless --attach is provided)"`
	Thread string   `name:"thread" help:"Reply to thread (spaces/.../threads/...)"`
	Attach []string `name:"attach" help:"Attachment file path, e.g. an image (repeatable)"`
}

func (c *ChatMessagesSendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := newChatMessageSendPlan(chatMessageSendInput{
		Space:       c.Space,
		Text:        c.Text,
		Thread:      c.Thread,
		Attachments: c.Attach,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "chat.messages.send", plan.dryRunPayload()); dryRunErr != nil {
		return dryRunErr
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

	var attachments []*chat.Attachment
	if len(plan.Attachments) > 0 {
		attachments, err = uploadChatAttachments(ctx, svc, plan.Space, plan.Attachments)
		if err != nil {
			return err
		}
	}
	message := plan.message(attachments)

	call := svc.Spaces.Messages.Create(plan.Space, message)
	if replyOption := plan.replyOption(); replyOption != "" {
		call = call.MessageReplyOption(replyOption)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"message": resp})
	}

	if resp == nil {
		u.Out().Linef("space\t%s", plan.Space)
		return nil
	}
	if resp.Name != "" {
		u.Out().Linef("resource\t%s", resp.Name)
	}
	if resp.Thread != nil && resp.Thread.Name != "" {
		u.Out().Linef("thread\t%s", resp.Thread.Name)
	}
	return nil
}
