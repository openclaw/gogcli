package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/config"
)

type composeFromResult struct {
	header       string
	sendingEmail string
}

func expandComposeAttachmentPaths(paths []string) ([]string, error) {
	expanded := make([]string, 0, len(paths))
	for _, path := range paths {
		resolved, err := config.ExpandPath(path)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, resolved)
	}
	return expanded, nil
}

func attachmentsFromPaths(paths []string) []mailAttachment {
	attachments := make([]mailAttachment, 0, len(paths))
	for _, path := range paths {
		attachments = append(attachments, mailAttachment{Path: path})
	}
	return attachments
}

func resolveComposeFrom(ctx context.Context, svc *gmail.Service, account, from string, sendAsList []*gmail.SendAs, sendAsListErr error) (composeFromResult, error) {
	account = strings.TrimSpace(account)
	from = strings.TrimSpace(from)
	result := composeFromResult{
		header:       account,
		sendingEmail: account,
	}

	if from != "" {
		var sendAs *gmail.SendAs
		if sendAsListErr == nil {
			sendAs = findSendAsByEmail(sendAsList, from)
			if sendAs == nil {
				return composeFromResult{}, fmt.Errorf("invalid --from address %q: not found in send-as settings", from)
			}
		} else {
			var err error
			sendAs, err = svc.Users.Settings.SendAs.Get("me", from).Context(ctx).Do()
			if err != nil {
				return composeFromResult{}, fmt.Errorf("invalid --from address %q: %w", from, err)
			}
		}
		if !sendAsAllowedForFrom(sendAs) {
			return composeFromResult{}, fmt.Errorf("--from address %q is not verified (status: %s)", from, sendAs.VerificationStatus)
		}
		result.sendingEmail = from
		result.header = from
		if displayName := strings.TrimSpace(sendAs.DisplayName); displayName != "" {
			result.header = displayName + " <" + from + ">"
		}
		return result, nil
	}

	if sendAsListErr == nil {
		if displayName := primaryDisplayNameFromSendAsList(sendAsList, account); displayName != "" {
			result.header = displayName + " <" + account + ">"
		}
	}
	return result, nil
}

func prepareComposeReply(ctx context.Context, svc *gmail.Service, replyToMessageID, threadID string, quote bool, plainBody, htmlBody string) (*replyInfo, string, string, error) {
	info, err := fetchReplyInfo(ctx, svc, replyToMessageID, threadID, quote)
	if err != nil {
		return nil, "", "", err
	}
	plainBody, htmlBody = applyQuoteToBodies(plainBody, htmlBody, quote, info)
	return info, plainBody, htmlBody, nil
}
