package googleapi

import (
	"context"

	"google.golang.org/api/chat/v1"
)

const (
	scopeChatSpaces          = "https://www.googleapis.com/auth/chat.spaces"
	scopeChatMessages        = "https://www.googleapis.com/auth/chat.messages"
	scopeChatMemberships     = "https://www.googleapis.com/auth/chat.memberships"
	scopeChatReadStateRO     = "https://www.googleapis.com/auth/chat.users.readstate.readonly"
	scopeChatReactionsCreate = "https://www.googleapis.com/auth/chat.messages.reactions.create"
	scopeChatReactionsRO     = "https://www.googleapis.com/auth/chat.messages.reactions.readonly"
)

func NewChat(ctx context.Context, email string) (*chat.Service, error) {
	return newGoogleServiceForScopes(ctx, email, "chat", "chat", []string{scopeChatSpaces, scopeChatMessages, scopeChatMemberships, scopeChatReadStateRO, scopeChatReactionsCreate, scopeChatReactionsRO}, chat.NewService)
}
