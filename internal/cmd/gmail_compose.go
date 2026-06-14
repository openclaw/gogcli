package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/mailmime"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type composeFromResult struct {
	header       string
	sendingEmail string
}

type gmailMessageResult struct {
	From        string
	To          string
	MessageID   string
	ThreadID    string
	TrackingID  string
	Attachments []mailmime.AttachmentMetadata
}

func requireGmailSendService(ctx context.Context, flags *RootFlags) (string, *gmail.Service, error) {
	account, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return "", nil, err
	}
	if err = checkAccountNoSend(ctx, account); err != nil {
		return "", nil, err
	}
	return account, svc, nil
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

func attachmentsFromPaths(paths []string) []mailmime.Attachment {
	attachments := make([]mailmime.Attachment, 0, len(paths))
	for _, path := range paths {
		attachments = append(attachments, mailmime.Attachment{Path: path})
	}
	return attachments
}

func validateComposeHeaderInputs(to, cc, bcc, replyTo, subject, from string) error {
	for _, tc := range []struct {
		flag   string
		values []string
	}{
		{flag: "--to", values: splitCSV(to)},
		{flag: "--cc", values: splitCSV(cc)},
		{flag: "--bcc", values: splitCSV(bcc)},
		{flag: "--reply-to", values: []string{replyTo}},
		{flag: "--subject", values: []string{subject}},
		{flag: "--from", values: []string{from}},
	} {
		for _, value := range tc.values {
			if strings.TrimSpace(value) == "" {
				continue
			}
			if err := mailmime.ValidateHeaderValue(value); err != nil {
				return usagef("invalid %s: %v", tc.flag, err)
			}
		}
	}
	return nil
}

func resolveComposeSender(ctx context.Context, svc *gmail.Service, account, from string) (composeFromResult, error) {
	sendAsList, sendAsListErr := listSendAs(ctx, svc)
	return resolveComposeFrom(ctx, svc, account, from, sendAsList, sendAsListErr)
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
		} else if displayName := primaryDisplayNameFromPeople(ctx, account); displayName != "" {
			result.header = displayName + " <" + account + ">"
		}
	}
	return result, nil
}

func primaryDisplayNameFromPeople(ctx context.Context, account string) string {
	svc, err := peopleContactsService(ctx, account)
	if err != nil {
		return ""
	}
	person, err := svc.People.Get(peopleMeResource).PersonFields("names").Context(ctx).Do()
	if err != nil {
		return ""
	}
	return primaryDisplayNameFromPerson(person)
}

func primaryDisplayNameFromPerson(person *people.Person) string {
	if person == nil {
		return ""
	}
	for _, name := range person.Names {
		if name == nil {
			continue
		}
		if displayName := strings.TrimSpace(name.DisplayName); displayName != "" {
			return displayName
		}
	}
	return ""
}

func prepareComposeReply(ctx context.Context, svc *gmail.Service, replyToMessageID, threadID string, quote bool, plainBody, htmlBody string) (*replyInfo, string, string, error) {
	info, err := fetchReplyInfo(ctx, svc, replyToMessageID, threadID, quote)
	if err != nil {
		return nil, "", "", err
	}
	plainBody, htmlBody = applyQuoteToBodies(plainBody, htmlBody, quote, info)
	return info, plainBody, htmlBody, nil
}

func buildGmailMessage(ctx context.Context, opts sendMessageOptions, batch sendBatch, allowMissingTo bool) (*gmail.Message, error) {
	reply := replyInfo{}
	if opts.ReplyInfo != nil {
		reply = *opts.ReplyInfo
	}

	dateLocation, err := mailDateLocation(ctx, stderrWriter(ctx))
	if err != nil {
		return nil, err
	}
	raw, err := mailmime.BuildRFC822(mailmime.Options{
		From:              opts.FromAddr,
		To:                batch.To,
		Cc:                batch.Cc,
		Bcc:               batch.Bcc,
		ReplyTo:           opts.ReplyTo,
		Subject:           opts.Subject,
		Body:              opts.Body,
		BodyHTML:          opts.BodyHTML,
		InReplyTo:         reply.InReplyTo,
		References:        reply.References,
		AdditionalHeaders: opts.Headers,
		Attachments:       opts.Attachments,
	}, mailmime.Config{
		AllowMissingTo: allowMissingTo,
		DateLocation:   dateLocation,
		Now:            time.Now,
		Random:         rand.Reader,
		ReadFile:       os.ReadFile,
	})
	if err != nil {
		return nil, err
	}

	msg := &gmail.Message{
		Raw: base64.RawURLEncoding.EncodeToString(raw),
	}
	if reply.ThreadID != "" {
		msg.ThreadId = reply.ThreadID
	}
	return msg, nil
}

func writeGmailMessageResults(ctx context.Context, u *ui.UI, results []gmailMessageResult) error {
	if outfmt.IsJSON(ctx) {
		if len(results) == 1 {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), gmailMessageResultJSON(results[0], false))
		}

		items := make([]map[string]any, 0, len(results))
		for _, r := range results {
			items = append(items, gmailMessageResultJSON(r, true))
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"messages": items})
	}

	for i, r := range results {
		if i > 0 {
			u.Out().Println("")
		}
		if len(results) > 1 && r.To != "" {
			u.Out().Linef("to\t%s", r.To)
		}
		u.Out().Linef("message_id\t%s", r.MessageID)
		if r.ThreadID != "" {
			u.Out().Linef("thread_id\t%s", r.ThreadID)
		}
		if r.TrackingID != "" {
			u.Out().Linef("tracking_id\t%s", r.TrackingID)
		}
	}
	return nil
}

func gmailMessageResultJSON(r gmailMessageResult, includeTo bool) map[string]any {
	item := map[string]any{
		"messageId": r.MessageID,
		"threadId":  r.ThreadID,
	}
	if r.From != "" {
		item["from"] = r.From
	}
	if includeTo && r.To != "" {
		item["to"] = r.To
	}
	if r.TrackingID != "" {
		item["tracking_id"] = r.TrackingID
	}
	if len(r.Attachments) > 0 {
		item["attachments"] = r.Attachments
	}
	return item
}
