package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/chat/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type ChatMessagesCmd struct {
	List      ChatMessagesListCmd      `cmd:"" name:"list" aliases:"ls" help:"List messages"`
	Search    ChatMessagesSearchCmd    `cmd:"" name:"search" aliases:"find,query" help:"Search messages across Chat"`
	Send      ChatMessagesSendCmd      `cmd:"" name:"send" aliases:"create,post" help:"Send a message"`
	React     ChatMessagesReactCmd     `cmd:"" name:"react" help:"Add an emoji reaction to a message"`
	Reactions ChatMessagesReactionsCmd `cmd:"" name:"reactions" aliases:"reaction" help:"Manage emoji reactions on a message"`
}

type ChatMessagesSearchCmd struct {
	Query     []string `arg:"" name:"query" help:"Search query using Google Chat filter syntax"`
	Max       int64    `name:"max" aliases:"limit" help:"Max results per page" default:"25"`
	Page      string   `name:"page" aliases:"cursor" help:"Page token"`
	All       bool     `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	Order     string   `name:"order" help:"Order by: create_time desc or relevance desc (Developer Preview)" enum:"create_time desc,relevance desc," default:""`
	View      string   `name:"view" help:"Result view: basic or full" enum:"basic,full" default:"basic"`
	Markup    string   `name:"markup" help:"Formatted text syntax: chat or markdown" enum:"chat,markdown," default:""`
}

func (c *ChatMessagesSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	query := strings.TrimSpace(strings.Join(c.Query, " "))
	if query == "" {
		return usage("missing query")
	}
	if c.Max <= 0 || c.Max > 100 {
		return usage("max must be between 1 and 100")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}

	svc, err := chatSearchService(ctx, account)
	if err != nil {
		return err
	}

	view := "SEARCH_MESSAGES_VIEW_BASIC"
	if c.View == "full" {
		view = "SEARCH_MESSAGES_VIEW_FULL"
	}
	markup := ""
	switch c.Markup {
	case "chat":
		markup = "MARKUP_SYNTAX_CHAT"
	case "markdown":
		markup = "MARKUP_SYNTAX_MARKDOWN"
	}

	fetch := func(pageToken string) ([]*googleapi.ChatSearchResult, string, error) {
		req := &chat.SearchMessagesRequest{
			Filter:       query,
			MarkupSyntax: markup,
			OrderBy:      strings.TrimSpace(c.Order),
			PageSize:     c.Max,
			PageToken:    strings.TrimSpace(pageToken),
			View:         view,
		}
		resp, callErr := svc.Search(ctx, req)
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Results, resp.NextPageToken, nil
	}

	results, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}
	items := compactChatSearchRows(results, c.View == "full")

	if outfmt.IsJSON(ctx) {
		return writePagedJSONResult(ctx, map[string]any{
			"results":       items,
			"nextPageToken": nextPageToken,
		}, len(items), c.FailEmpty)
	}

	if len(items) == 0 {
		u.Err().Println("No results")
		return failEmptyExit(c.FailEmpty)
	}
	if err := outfmt.WriteTable(ctx, stdoutWriter(ctx), items, chatMessageSearchColumns(c.View == "full")); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}

type chatMessageSearchItem struct {
	Resource         string `json:"resource"`
	Space            string `json:"space,omitempty"`
	Sender           string `json:"sender,omitempty"`
	Text             string `json:"text,omitempty"`
	FormattedText    string `json:"formattedText,omitempty"`
	CreateTime       string `json:"createTime,omitempty"`
	Thread           string `json:"thread,omitempty"`
	Read             *bool  `json:"read,omitempty"`
	SpaceMuteSetting string `json:"spaceMuteSetting,omitempty"`
}

func compactChatSearchRows(results []*googleapi.ChatSearchResult, includeRead bool) []*chatMessageSearchItem {
	items := make([]*chatMessageSearchItem, 0, len(results))
	for _, result := range results {
		if result == nil || result.Message == nil {
			continue
		}
		msg := result.Message
		item := &chatMessageSearchItem{
			Resource:         msg.Name,
			Space:            chatMessageSpace(msg),
			Sender:           chatMessageSender(msg),
			Text:             chatMessageText(msg),
			FormattedText:    msg.FormattedText,
			CreateTime:       msg.CreateTime,
			Thread:           chatMessageThread(msg),
			SpaceMuteSetting: result.SpaceMuteSetting,
		}
		if includeRead {
			item.Read = result.Read
		}
		items = append(items, item)
	}
	return items
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
			Resource               string                       `json:"resource"`
			Sender                 string                       `json:"sender,omitempty"`
			Text                   string                       `json:"text,omitempty"`
			CreateTime             string                       `json:"createTime,omitempty"`
			Thread                 string                       `json:"thread,omitempty"`
			Annotations            []*chat.Annotation           `json:"annotations,omitempty"`
			EmojiReactionSummaries []*chat.EmojiReactionSummary `json:"emojiReactionSummaries,omitempty"`
		}
		items := make([]item, 0, len(messages))
		for _, msg := range messages {
			if msg == nil {
				continue
			}
			var mentions []*chat.Annotation
			for _, annotation := range msg.Annotations {
				if annotation != nil && annotation.Type == "USER_MENTION" && annotation.UserMention != nil {
					mentions = append(mentions, &chat.Annotation{
						Type:        annotation.Type,
						StartIndex:  annotation.StartIndex,
						Length:      annotation.Length,
						UserMention: annotation.UserMention,
					})
				}
			}
			var reactions []*chat.EmojiReactionSummary
			for _, summary := range msg.EmojiReactionSummaries {
				if summary == nil || summary.Emoji == nil {
					continue
				}
				emoji := &chat.Emoji{Unicode: summary.Emoji.Unicode}
				if custom := summary.Emoji.CustomEmoji; custom != nil {
					emoji.CustomEmoji = &chat.CustomEmoji{Name: custom.Name, EmojiName: custom.EmojiName, Uid: custom.Uid}
				}
				reactions = append(reactions, &chat.EmojiReactionSummary{Emoji: emoji, ReactionCount: summary.ReactionCount})
			}
			items = append(items, item{
				Resource:               msg.Name,
				Sender:                 chatMessageSender(msg),
				Text:                   chatMessageText(msg),
				CreateTime:             msg.CreateTime,
				Thread:                 chatMessageThread(msg),
				Annotations:            mentions,
				EmojiReactionSummaries: reactions,
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
