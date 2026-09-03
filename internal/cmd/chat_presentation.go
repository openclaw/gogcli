package cmd

import (
	"fmt"

	"google.golang.org/api/chat/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
)

func chatSpaceColumns() []outfmt.Column[*chat.Space] {
	return []outfmt.Column[*chat.Space]{
		{Header: "RESOURCE", Value: func(space *chat.Space) string { return space.Name }},
		{Header: "NAME", Value: func(space *chat.Space) string { return sanitizeTab(space.DisplayName) }},
		{Header: "TYPE", Value: func(space *chat.Space) string { return sanitizeTab(chatSpaceType(space)) }},
	}
}

func chatMessageColumns() []outfmt.Column[*chat.Message] {
	return []outfmt.Column[*chat.Message]{
		{Header: "RESOURCE", Value: func(message *chat.Message) string { return message.Name }},
		{Header: "SENDER", Value: func(message *chat.Message) string {
			return sanitizeTab(chatMessageSender(message))
		}},
		{Header: "TIME", Value: func(message *chat.Message) string {
			return sanitizeTab(message.CreateTime)
		}},
		{Header: "TEXT", Value: func(message *chat.Message) string {
			return sanitizeChatText(chatMessageText(message))
		}},
	}
}

func chatMessageSearchColumns(includeRead bool) []outfmt.Column[*chatMessageSearchItem] {
	columns := []outfmt.Column[*chatMessageSearchItem]{
		{Header: "RESOURCE", Value: func(item *chatMessageSearchItem) string { return item.Resource }},
		{Header: "SPACE", Value: func(item *chatMessageSearchItem) string { return item.Space }},
		{Header: "THREAD", Value: func(item *chatMessageSearchItem) string { return item.Thread }},
		{Header: "SENDER", Value: func(item *chatMessageSearchItem) string { return sanitizeTab(item.Sender) }},
		{Header: "TIME", Value: func(item *chatMessageSearchItem) string { return sanitizeTab(item.CreateTime) }},
		{Header: "TEXT", Value: func(item *chatMessageSearchItem) string { return sanitizeChatText(item.Text) }},
	}
	if includeRead {
		columns = append(columns, outfmt.Column[*chatMessageSearchItem]{
			Header: "READ",
			Value: func(item *chatMessageSearchItem) string {
				if item.Read == nil {
					return ""
				}
				return fmt.Sprintf("%t", *item.Read)
			},
		})
	}
	return columns
}

func chatThreadColumns() []outfmt.Column[*chatMessageThreadItem] {
	return []outfmt.Column[*chatMessageThreadItem]{
		{Header: "THREAD", Value: func(item *chatMessageThreadItem) string { return item.thread }},
		{Header: "MESSAGE", Value: func(item *chatMessageThreadItem) string { return item.message.Name }},
		{Header: "SENDER", Value: func(item *chatMessageThreadItem) string {
			return sanitizeTab(chatMessageSender(item.message))
		}},
		{Header: "TIME", Value: func(item *chatMessageThreadItem) string {
			return sanitizeTab(item.message.CreateTime)
		}},
		{Header: "TEXT", Value: func(item *chatMessageThreadItem) string {
			return sanitizeChatText(chatMessageText(item.message))
		}},
	}
}

func chatReactionColumns() []outfmt.Column[*chat.Reaction] {
	return []outfmt.Column[*chat.Reaction]{
		{Header: "RESOURCE", Value: func(reaction *chat.Reaction) string { return reaction.Name }},
		{Header: "EMOJI", Value: reactionEmoji},
		{Header: "USER", Value: reactionUser},
	}
}

func compactChatRows[T any](rows []*T) []*T {
	filtered := make([]*T, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func compactChatThreadRows(rows []*chatMessageThreadItem) []*chatMessageThreadItem {
	filtered := make([]*chatMessageThreadItem, 0, len(rows))
	for _, row := range rows {
		if row != nil && row.message != nil {
			filtered = append(filtered, row)
		}
	}
	return filtered
}
