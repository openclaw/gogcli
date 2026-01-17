package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/chat/v1"
)

const (
	scopeChatSpaces      = "https://www.googleapis.com/auth/chat.spaces"
	scopeChatMessages    = "https://www.googleapis.com/auth/chat.messages"
	scopeChatMemberships = "https://www.googleapis.com/auth/chat.memberships"
	scopeChatReadStateRO = "https://www.googleapis.com/auth/chat.users.readstate.readonly"
)

func NewChat(ctx context.Context, email string) (*chat.Service, error) {
	if opts, err := optionsForAccountScopes(ctx, "chat", email, []string{scopeChatSpaces, scopeChatMessages, scopeChatMemberships, scopeChatReadStateRO}); err != nil {
		return nil, fmt.Errorf("chat options: %w", err)
	} else if svc, err := chat.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create chat service: %w", err)
	} else {
		return svc, nil
	}
}
