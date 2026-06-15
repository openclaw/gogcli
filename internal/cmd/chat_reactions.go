package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ChatMessagesReactCmd struct {
	Message string `arg:"" name:"message" help:"Message resource (spaces/.../messages/...) or bare message ID"`
	Emoji   string `arg:"" name:"emoji" help:"Emoji unicode character (e.g. 👍)"`
	Space   string `name:"space" help:"Space name (required when message is a bare ID)"`
}

func (c *ChatMessagesReactCmd) Run(ctx context.Context, flags *RootFlags) error {
	cmd := &ChatMessagesReactionsCreateCmd{Message: c.Message, Emoji: c.Emoji, Space: c.Space}
	return cmd.Run(ctx, flags)
}

type ChatMessagesReactionsCmd struct {
	Create ChatMessagesReactionsCreateCmd `cmd:"" name:"create" aliases:"add" help:"Add an emoji reaction to a message"`
	List   ChatMessagesReactionsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List reactions on a message"`
	Delete ChatMessagesReactionsDeleteCmd `cmd:"" name:"delete" aliases:"remove,rm" help:"Delete a reaction"`
}

type ChatMessagesReactionsCreateCmd struct {
	Message string `arg:"" name:"message" help:"Message resource (spaces/.../messages/...) or bare message ID"`
	Emoji   string `arg:"" name:"emoji" help:"Emoji unicode character (e.g. 📦)"`
	Space   string `name:"space" help:"Space name (required when message is a bare ID)"`
}

func (c *ChatMessagesReactionsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	message, err := normalizeMessage(c.Space, c.Message)
	if err != nil {
		return usage("required: message (full resource path, or bare ID with --space)")
	}

	emoji := strings.TrimSpace(c.Emoji)
	if emoji == "" {
		return usage("required: emoji")
	}

	if dryRunErr := dryRunExit(ctx, flags, "chat.messages.reactions.create", map[string]any{
		"message": message,
		"emoji":   emoji,
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

	svc, err := chatService(ctx, account)
	if err != nil {
		return err
	}

	reaction := &chat.Reaction{
		Emoji: &chat.Emoji{Unicode: emoji},
	}
	resp, err := svc.Spaces.Messages.Reactions.Create(message, reaction).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"reaction": resp})
	}

	u.Out().Linef("resource\t%s", resp.Name)
	return nil
}

type ChatMessagesReactionsListCmd struct {
	Message string `arg:"" name:"message" help:"Message resource (spaces/.../messages/...) or bare message ID"`
	Space   string `name:"space" help:"Space name (required when message is a bare ID)"`
	Max     int64  `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page    string `name:"page" aliases:"cursor" help:"Page token"`
	All     bool   `name:"all" help:"Fetch all pages"`
}

func (c *ChatMessagesReactionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	message, err := normalizeMessage(c.Space, c.Message)
	if err != nil {
		return usage("required: message (full resource path, or bare ID with --space)")
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

	fetch := func(pageToken string) ([]*chat.Reaction, string, error) {
		call := svc.Spaces.Messages.Reactions.List(message).
			PageSize(c.Max).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Reactions, resp.NextPageToken, nil
	}

	reactions, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		type item struct {
			Resource string `json:"resource"`
			Emoji    string `json:"emoji,omitempty"`
			User     string `json:"user,omitempty"`
		}
		items := make([]item, 0, len(reactions))
		for _, r := range reactions {
			if r == nil {
				continue
			}
			items = append(items, item{
				Resource: r.Name,
				Emoji:    reactionEmoji(r),
				User:     reactionUser(r),
			})
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"reactions":     items,
			"nextPageToken": nextPageToken,
		})
	}

	if len(reactions) == 0 {
		u.Err().Println("No reactions")
		return nil
	}

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactChatRows(reactions),
		chatReactionColumns(),
	); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all")
	return nil
}

type ChatMessagesReactionsDeleteCmd struct {
	Reaction string `arg:"" name:"reaction" help:"Reaction resource (spaces/.../messages/.../reactions/...)"`
}

func (c *ChatMessagesReactionsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	reaction := strings.TrimSpace(c.Reaction)
	if reaction == "" {
		return usage("required: reaction")
	}
	reaction, err := normalizeReaction(reaction)
	if err != nil {
		return usage("required: reaction resource (spaces/.../messages/.../reactions/...)")
	}

	if dryRunErr := dryRunExit(ctx, flags, "chat.messages.reactions.delete", map[string]any{
		"reaction": reaction,
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

	svc, err := chatService(ctx, account)
	if err != nil {
		return err
	}

	_, err = svc.Spaces.Messages.Reactions.Delete(reaction).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"deleted": reaction})
	}

	u.Out().Linef("deleted\t%s", reaction)
	return nil
}

func reactionEmoji(r *chat.Reaction) string {
	if r == nil || r.Emoji == nil {
		return ""
	}
	if r.Emoji.Unicode != "" {
		return r.Emoji.Unicode
	}
	if r.Emoji.CustomEmoji != nil {
		return r.Emoji.CustomEmoji.Uid
	}
	return ""
}

func reactionUser(r *chat.Reaction) string {
	if r == nil || r.User == nil {
		return ""
	}
	if r.User.DisplayName != "" {
		return r.User.DisplayName
	}
	return r.User.Name
}
