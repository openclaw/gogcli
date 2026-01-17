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

type ChatDMCmd struct {
	Send  ChatDMSendCmd  `cmd:"" name:"send" help:"Send a direct message"`
	Space ChatDMSpaceCmd `cmd:"" name:"space" help:"Find or create a DM space"`
}

type ChatDMSendCmd struct {
	Email  string `arg:"" name:"email" help:"Recipient email"`
	Text   string `name:"text" help:"Message text (required)"`
	Thread string `name:"thread" help:"Reply to thread (spaces/.../threads/...)"`
}

func (c *ChatDMSendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("required: email")
	}

	text := strings.TrimSpace(c.Text)
	if text == "" {
		return usage("required: --text")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	space, err := setupDMSpace(ctx, svc, email)
	if err != nil {
		return err
	}
	if space == nil || space.Name == "" {
		return fmt.Errorf("failed to setup DM space for %q", email)
	}

	message := &chat.Message{Text: text}
	thread := strings.TrimSpace(c.Thread)
	if thread != "" {
		message.Thread = &chat.Thread{Name: thread}
	}

	call := svc.Spaces.Messages.Create(space.Name, message)
	if thread != "" {
		call = call.MessageReplyOption("REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD")
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"message": resp})
	}

	if resp == nil {
		u.Out().Printf("space\t%s\n", space.Name)
		return nil
	}
	if resp.Name != "" {
		u.Out().Printf("resource\t%s\n", resp.Name)
	}
	if resp.Thread != nil && resp.Thread.Name != "" {
		u.Out().Printf("thread\t%s\n", resp.Thread.Name)
	}
	return nil
}

type ChatDMSpaceCmd struct {
	Email string `arg:"" name:"email" help:"Recipient email"`
}

func (c *ChatDMSpaceCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("required: email")
	}

	svc, err := newChatService(ctx, account)
	if err != nil {
		return err
	}

	space, err := setupDMSpace(ctx, svc, email)
	if err != nil {
		return err
	}
	if space == nil {
		return fmt.Errorf("failed to setup DM space for %q", email)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"space": space})
	}
	if space.Name != "" {
		u.Out().Printf("resource\t%s\n", space.Name)
	}
	if space.DisplayName != "" {
		u.Out().Printf("name\t%s\n", space.DisplayName)
	}
	return nil
}

func setupDMSpace(ctx context.Context, svc *chat.Service, email string) (*chat.Space, error) {
	user := normalizeUser(email)
	if user == "" {
		return nil, fmt.Errorf("invalid email: %q", email)
	}
	return svc.Spaces.Setup(&chat.SetUpSpaceRequest{
		Space: &chat.Space{
			SpaceType: "DIRECT_MESSAGE",
		},
		Memberships: []*chat.Membership{{
			Member: &chat.User{
				Name: user,
				Type: "HUMAN",
			},
		}},
	}).Context(ctx).Do()
}
