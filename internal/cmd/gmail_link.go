package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/gmailcontent"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type GmailLinkCmd struct {
	MessageID string `arg:"" name:"messageId" help:"Message ID"`
	Index     string `arg:"" name:"index" help:"0-based link index from the sanitized body's [link:N] marker"`
}

// linkByIndex resolves a 0-based index from a sanitized body's [link:N] marker to the
// link itself. It reruns the body conversion that numbered the markers, so the index is
// a stable reference as long as the message content is unchanged.
func linkByIndex(ctx context.Context, svc *gmail.Service, messageID string, idx int) (gmailLink, error) {
	if idx < 0 {
		return gmailLink{}, usagef("link index must be >= 0, got %d", idx)
	}
	msg, err := svc.Users.Messages.Get("me", messageID).Format("full").Context(ctx).Do()
	if err != nil {
		return gmailLink{}, fmt.Errorf("resolve link index %d: %w", idx, err)
	}
	body, isHTML := gmailcontent.BestBodyForDisplay(msg.Payload)
	_, links := sanitizeGmailBodyLinks(body, isHTML)
	if idx >= len(links) {
		return gmailLink{}, usagef("link index %d out of range: message has %d link(s)", idx, len(links))
	}
	return links[idx], nil
}

func (c *GmailLinkCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	messageID := normalizeGmailMessageID(strings.TrimSpace(c.MessageID))
	if messageID == "" {
		return usage("empty messageId")
	}
	idx, err := strconv.Atoi(strings.TrimSpace(c.Index))
	if err != nil {
		return usagef("the link argument must be a 0-based index, got %q", c.Index)
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	link, err := linkByIndex(ctx, svc, messageID, idx)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), link)
	}
	u.Out().Linef("url\t%s", link.URL)
	if link.Text != "" {
		u.Out().Linef("text\t%s", link.Text)
	}
	return nil
}
