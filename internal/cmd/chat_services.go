package cmd

import (
	"context"

	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/googleapi"
)

var newChatService func(ctx context.Context, email string) (*chat.Service, error) = googleapi.NewChat
