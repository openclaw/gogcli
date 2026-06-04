package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/config"
)

func normalizeSpace(resource string) (string, error) {
	space := strings.TrimSpace(resource)
	if space == "" {
		return "", fmt.Errorf("empty space")
	}
	if strings.HasPrefix(space, "spaces/") {
		parts := strings.Split(space, "/")
		if len(parts) != 2 || parts[1] == "" {
			return "", fmt.Errorf("invalid space resource %q", space)
		}
		return space, nil
	}
	if strings.Contains(space, "/") {
		return "", fmt.Errorf("invalid space id %q", space)
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

func normalizeChatMemberUser(member string) (string, error) {
	user := strings.TrimSpace(member)
	if user == "" {
		return "", nil
	}
	if strings.HasPrefix(user, "users/") {
		id := strings.TrimPrefix(user, "users/")
		if id == "" || strings.Contains(id, "/") || strings.ContainsAny(id, " \t\r\n<>") {
			return "", usagef("invalid --member %q", member)
		}
		return user, nil
	}
	if err := validatePlainEmail("--member", user); err != nil {
		return "", err
	}
	return "users/" + user, nil
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
		parts := strings.Split(thread, "/")
		if len(parts) != 4 || parts[0] != "spaces" || parts[1] == "" || parts[2] != "threads" || parts[3] == "" {
			return "", fmt.Errorf("invalid thread resource %q", thread)
		}
		return thread, nil
	}
	thread = strings.TrimPrefix(thread, "threads/")
	if thread == "" || strings.Contains(thread, "/") {
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
		parts := strings.Split(msg, "/")
		if len(parts) != 4 || parts[0] != "spaces" || parts[1] == "" || parts[2] != "messages" || parts[3] == "" {
			return "", fmt.Errorf("invalid message resource %q", msg)
		}
		return msg, nil
	}
	msg = strings.TrimPrefix(msg, "messages/")
	if msg == "" || strings.Contains(msg, "/") {
		return "", fmt.Errorf("invalid message id %q", msg)
	}
	space, err := normalizeSpace(space)
	if err != nil {
		return "", fmt.Errorf("--space required when message is a bare ID")
	}
	return fmt.Sprintf("%s/messages/%s", space, msg), nil
}

func normalizeReaction(resource string) (string, error) {
	reaction := strings.TrimSpace(resource)
	if reaction == "" {
		return "", fmt.Errorf("empty reaction")
	}
	parts := strings.Split(reaction, "/")
	if len(parts) != 6 || parts[0] != "spaces" || parts[2] != "messages" || parts[4] != "reactions" {
		return "", fmt.Errorf("invalid reaction resource %q", reaction)
	}
	for _, part := range []string{parts[1], parts[3], parts[5]} {
		if part == "" {
			return "", fmt.Errorf("invalid reaction resource %q", reaction)
		}
	}
	return reaction, nil
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

// expandChatAttachmentPaths resolves user-supplied attachment paths (~, env vars)
// to absolute paths without reading the files. Used for validation and dry-run.
func expandChatAttachmentPaths(paths []string) ([]string, error) {
	expanded := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return nil, usage("attachment path must not be empty")
		}
		resolved, err := config.ExpandPath(trimmed)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, resolved)
	}
	return expanded, nil
}

// uploadChatAttachments uploads each file to the given space and returns the
// attachment refs to embed in a message. Uploads happen in order; the first
// failure aborts and is returned to the caller.
func uploadChatAttachments(ctx context.Context, svc *chat.Service, space string, paths []string) ([]*chat.Attachment, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	attachments := make([]*chat.Attachment, 0, len(paths))
	for _, path := range paths {
		f, err := os.Open(path) //nolint:gosec // path is user-supplied by design (local CLI)
		if err != nil {
			return nil, fmt.Errorf("open attachment %q: %w", path, err)
		}
		req := &chat.UploadAttachmentRequest{Filename: filepath.Base(path)}
		resp, err := svc.Media.Upload(space, req).Media(f).Context(ctx).Do()
		closeErr := f.Close()
		if err != nil {
			return nil, fmt.Errorf("upload attachment %q: %w", path, err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close attachment %q: %w", path, closeErr)
		}
		if resp == nil || resp.AttachmentDataRef == nil {
			return nil, fmt.Errorf("upload attachment %q: empty data ref in response", path)
		}
		attachments = append(attachments, &chat.Attachment{AttachmentDataRef: resp.AttachmentDataRef})
	}
	return attachments, nil
}
