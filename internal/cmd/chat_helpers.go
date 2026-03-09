package cmd

import (
	"fmt"
	"strings"

	"google.golang.org/api/chat/v1"
)

func normalizeSpace(resource string) (string, error) {
	space := strings.TrimSpace(resource)
	if space == "" {
		return "", fmt.Errorf("empty space")
	}
	if strings.HasPrefix(space, "spaces/") {
		return space, nil
	}
	return "spaces/" + space, nil
}

func spaceID(space string) string {
	return strings.TrimPrefix(space, "spaces/")
}

func normalizeUser(resource string) string {
	user := strings.TrimSpace(resource)
	if user == "" {
		return ""
	}
	if strings.HasPrefix(user, "users/") {
		return user
	}
	return "users/" + user
}

func requireWorkspaceAccount(account string) error {
	if isConsumerAccount(account) {
		return usage("chat requires a Google Workspace account (non-gmail.com)")
	}
	return nil
}

func normalizeThread(space, resource string) (string, error) {
	thread := strings.TrimSpace(resource)
	if thread == "" {
		return "", fmt.Errorf("empty thread")
	}
	if strings.HasPrefix(thread, "spaces/") {
		if !strings.Contains(thread, "/threads/") {
			return "", fmt.Errorf("invalid thread resource %q", thread)
		}
		return thread, nil
	}
	thread = strings.TrimPrefix(thread, "threads/")
	if strings.Contains(thread, "/") {
		return "", fmt.Errorf("invalid thread id %q", thread)
	}
	space, err := normalizeSpace(space)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/threads/%s", space, thread), nil
}

func normalizeMessage(space, resource string) (string, error) {
	msg := strings.TrimSpace(resource)
	if msg == "" {
		return "", fmt.Errorf("empty message")
	}
	if strings.HasPrefix(msg, "spaces/") {
		if !strings.Contains(msg, "/messages/") {
			return "", fmt.Errorf("invalid message resource %q", msg)
		}
		return msg, nil
	}
	msg = strings.TrimPrefix(msg, "messages/")
	if strings.Contains(msg, "/") {
		return "", fmt.Errorf("invalid message id %q", msg)
	}
	space, err := normalizeSpace(space)
	if err != nil {
		return "", fmt.Errorf("--space required when message is a bare ID")
	}
	return fmt.Sprintf("%s/messages/%s", space, msg), nil
}

func parseCommaArgs(values []string) []string {
	out := make([]string, 0, len(values))
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			out = append(out, trimmed)
		}
	}
	return out
}

func chatSpaceType(space *chat.Space) string {
	if space == nil {
		return ""
	}
	if space.SpaceType != "" {
		return space.SpaceType
	}
	return space.Type
}

func chatMessageSender(msg *chat.Message) string {
	if msg == nil || msg.Sender == nil {
		return ""
	}
	if msg.Sender.DisplayName != "" {
		return msg.Sender.DisplayName
	}
	return msg.Sender.Name
}

func chatMessageText(msg *chat.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Text != "" {
		return msg.Text
	}
	return msg.ArgumentText
}

func chatMessageThread(msg *chat.Message) string {
	if msg == nil || msg.Thread == nil {
		return ""
	}
	return msg.Thread.Name
}

func sanitizeChatText(s string) string {
	replacer := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")
	return replacer.Replace(s)
}
