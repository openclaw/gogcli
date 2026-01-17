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
