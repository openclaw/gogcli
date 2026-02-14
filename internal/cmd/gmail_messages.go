package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailMessagesCmd struct {
	Search GmailMessagesSearchCmd `cmd:"" name:"search" aliases:"find,query,ls,list" group:"Read" help:"Search messages using Gmail query syntax"`
}

type GmailMessagesSearchCmd struct {
	Query       []string `arg:"" name:"query" help:"Search query"`
	Max         int64    `name:"max" aliases:"limit" help:"Max results" default:"10"`
	Page        string   `name:"page" aliases:"cursor" help:"Page token"`
	All         bool     `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty   bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	Timezone    string   `name:"timezone" short:"z" help:"Output timezone (IANA name, e.g. America/New_York, UTC). Default: local"`
	Local       bool     `name:"local" help:"Use local timezone (default behavior, useful to override --timezone)"`
	IncludeBody bool     `name:"include-body" help:"Include decoded message body (JSON is full; text output is truncated)"`
}

func (c *GmailMessagesSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(c.Query, " "))
	if query == "" {
		return usage("missing query")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*gmail.Message, string, error) {
		call := svc.Users.Messages.List("me").
			Q(query).
			MaxResults(c.Max).
			Fields("messages(id,threadId),nextPageToken").
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Messages, resp.NextPageToken, nil
	}

	var messages []*gmail.Message
	nextPageToken := ""
	if c.All {
		all, collectErr := collectAllPages(c.Page, fetch)
		if collectErr != nil {
			return collectErr
		}
		messages = all
	} else {
		messagesPage, pageToken, fetchErr := fetch(c.Page)
		if fetchErr != nil {
			return fetchErr
		}
		messages = messagesPage
		nextPageToken = pageToken
	}

	if len(messages) == 0 {
		if outfmt.IsJSON(ctx) {
			if writeErr := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
				"messages":      []messageItem{},
				"nextPageToken": nextPageToken,
			}); writeErr != nil {
				return writeErr
			}
			return failEmptyExit(c.FailEmpty)
		}
		u.Err().Println("No results")
		return failEmptyExit(c.FailEmpty)
	}

	idToName, err := fetchLabelIDToName(svc)
	if err != nil {
		return err
	}

	loc, err := resolveOutputLocation(c.Timezone, c.Local)
	if err != nil {
		return err
	}

	items, err := fetchMessageDetails(ctx, svc, messages, idToName, loc, c.IncludeBody)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		if writeErr := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"messages":      items,
			"nextPageToken": nextPageToken,
		}); writeErr != nil {
			return writeErr
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(items) == 0 {
		u.Err().Println("No results")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()

	if c.IncludeBody {
		fmt.Fprintln(w, "ID\tTHREAD\tDATE\tFROM\tSUBJECT\tLABELS\tBODY")
	} else {
		fmt.Fprintln(w, "ID\tTHREAD\tDATE\tFROM\tSUBJECT\tLABELS")
	}
	for _, it := range items {
		body := ""
		if c.IncludeBody {
			body = sanitizeMessageBody(it.Body)
		}
		if c.IncludeBody {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", it.ID, it.ThreadID, it.Date, it.From, it.Subject, strings.Join(it.Labels, ","), body)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", it.ID, it.ThreadID, it.Date, it.From, it.Subject, strings.Join(it.Labels, ","))
		}
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type messageItem struct {
	ID       string   `json:"id"`
	ThreadID string   `json:"threadId,omitempty"`
	Date     string   `json:"date,omitempty"`
	From     string   `json:"from,omitempty"`
	Subject  string   `json:"subject,omitempty"`
	Labels   []string `json:"labels,omitempty"`
	Body     string   `json:"body,omitempty"`
}

func fetchMessageDetails(ctx context.Context, svc *gmail.Service, messages []*gmail.Message, idToName map[string]string, loc *time.Location, includeBody bool) ([]messageItem, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)

	type result struct {
		index     int
		messageID string
		item      messageItem
		err       error
	}

	results := make(chan result, len(messages))
	var wg sync.WaitGroup

	for i, m := range messages {
		if m == nil || m.Id == "" {
			continue
		}
		wg.Add(1)
		go func(idx int, messageID string) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- result{index: idx, messageID: messageID, err: ctx.Err()}
				return
			}

			call := svc.Users.Messages.Get("me", messageID)
			if includeBody {
				call = call.Format("full")
			} else {
				call = call.Format("metadata").
					MetadataHeaders("From", "Subject", "Date").
					Fields("id,threadId,labelIds,payload(headers)")
			}
			msg, err := call.Context(ctx).Do()
			if err != nil {
				results <- result{index: idx, messageID: messageID, err: fmt.Errorf("message %s: %w", messageID, err)}
				return
			}

			item := messageItem{
				ID:       messageID,
				ThreadID: msg.ThreadId,
			}

			item.From = sanitizeTab(headerValue(msg.Payload, "From"))
			item.Subject = sanitizeTab(headerValue(msg.Payload, "Subject"))
			item.Date = formatGmailDateInLocation(headerValue(msg.Payload, "Date"), loc)
			if includeBody {
				item.Body = bestBodyText(msg.Payload)
			}

			if len(msg.LabelIds) > 0 {
				names := make([]string, 0, len(msg.LabelIds))
				for _, lid := range msg.LabelIds {
					if n, ok := idToName[lid]; ok {
						names = append(names, n)
					} else {
						names = append(names, lid)
					}
				}
				item.Labels = names
			}

			results <- result{index: idx, messageID: messageID, item: item}
		}(i, m.Id)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	ordered := make([]messageItem, len(messages))
	var firstErr error
	for r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			ordered[r.index] = messageItem{}
			continue
		}
		ordered[r.index] = r.item
	}
	if firstErr != nil {
		return nil, firstErr
	}

	items := make([]messageItem, 0, len(ordered))
	for _, item := range ordered {
		if item.ID != "" {
			items = append(items, item)
		}
	}
	return items, nil
}

func sanitizeMessageBody(body string) string {
	if body == "" {
		return ""
	}
	if looksLikeHTML(body) {
		body = stripHTMLTags(body)
	}
	body = strings.ReplaceAll(body, "\t", " ")
	body = strings.ReplaceAll(body, "\n", " ")
	body = strings.ReplaceAll(body, "\r", " ")
	body = strings.TrimSpace(body)
	return truncateRunes(body, 200)
}

func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}
