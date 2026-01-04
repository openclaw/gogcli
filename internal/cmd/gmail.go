package cmd

import (
	"context"
	"fmt"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newGmailService = googleapi.NewGmail

type GmailCmd struct {
	Search      GmailSearchCmd      `cmd:"" name:"search" help:"Search threads using Gmail query syntax"`
	Thread      GmailThreadCmd      `cmd:"" name:"thread" help:"Thread operations (get, modify)"`
	Get         GmailGetCmd         `cmd:"" name:"get" help:"Get a message (full|metadata|raw)"`
	Attachment  GmailAttachmentCmd  `cmd:"" name:"attachment" help:"Download a single attachment"`
	URL         GmailURLCmd         `cmd:"" name:"url" help:"Print Gmail web URLs for threads"`
	Labels      GmailLabelsCmd      `cmd:"" name:"labels" help:"Label operations"`
	Send        GmailSendCmd        `cmd:"" name:"send" help:"Send an email"`
	Drafts      GmailDraftsCmd      `cmd:"" name:"drafts" help:"Draft operations"`
	Watch       GmailWatchCmd       `cmd:"" name:"watch" help:"Manage Gmail watch"`
	History     GmailHistoryCmd     `cmd:"" name:"history" help:"Gmail history"`
	AutoForward GmailAutoForwardCmd `cmd:"" name:"autoforward" help:"Auto-forwarding settings"`
	Batch       GmailBatchCmd       `cmd:"" name:"batch" help:"Batch operations"`
	Delegates   GmailDelegatesCmd   `cmd:"" name:"delegates" help:"Delegate operations"`
	Filters     GmailFiltersCmd     `cmd:"" name:"filters" help:"Filter operations"`
	Forwarding  GmailForwardingCmd  `cmd:"" name:"forwarding" help:"Forwarding addresses"`
	SendAs      GmailSendAsCmd      `cmd:"" name:"sendas" help:"Send-as settings"`
	Vacation    GmailVacationCmd    `cmd:"" name:"vacation" help:"Vacation responder"`
}

type GmailSearchCmd struct {
	Query []string `arg:"" name:"query" help:"Search query"`
	Max   int64    `name:"max" aliases:"limit" help:"Max results" default:"10"`
	Page  string   `name:"page" help:"Page token"`
}

func (c *GmailSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	resp, err := svc.Users.Threads.List("me").
		Q(query).
		MaxResults(c.Max).
		PageToken(c.Page).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	idToName, err := fetchLabelIDToName(svc)
	if err != nil {
		return err
	}

	// Fetch thread details concurrently (fixes N+1 query pattern)
	items, err := fetchThreadDetails(ctx, svc, resp.Threads, idToName)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"threads":       items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(items) == 0 {
		u.Err().Println("No results")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintln(w, "ID\tDATE\tFROM\tSUBJECT\tLABELS")
	for _, it := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", it.ID, it.Date, it.From, it.Subject, strings.Join(it.Labels, ","))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

func firstMessage(t *gmail.Thread) *gmail.Message {
	if t == nil || len(t.Messages) == 0 {
		return nil
	}
	return t.Messages[0]
}

func headerValue(p *gmail.MessagePart, name string) string {
	if p == nil {
		return ""
	}
	for _, h := range p.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func hasHeaderName(headers []string, name string) bool {
	for _, h := range headers {
		if strings.EqualFold(strings.TrimSpace(h), name) {
			return true
		}
	}
	return false
}

func formatGmailDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if t, err := mailParseDate(raw); err == nil {
		return t.Format("2006-01-02 15:04")
	}
	return raw
}

var listUnsubscribeLinkPattern = regexp.MustCompile(`<([^>]+)>`)

func bestUnsubscribeLink(p *gmail.MessagePart) string {
	links := parseListUnsubscribe(headerValue(p, "List-Unsubscribe"))
	if len(links) == 0 {
		return ""
	}
	for _, link := range links {
		lower := strings.ToLower(link)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			return link
		}
	}
	return links[0]
}

func parseListUnsubscribe(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	candidates := make([]string, 0)
	matches := listUnsubscribeLinkPattern.FindAllStringSubmatch(raw, -1)
	if len(matches) > 0 {
		for _, match := range matches {
			candidate := strings.TrimSpace(match[1])
			if candidate == "" {
				continue
			}
			candidates = append(candidates, candidate)
		}
	} else {
		parts := strings.Split(raw, ",")
		for _, part := range parts {
			candidate := strings.TrimSpace(strings.Trim(part, "<>\""))
			if candidate == "" {
				continue
			}
			candidates = append(candidates, candidate)
		}
	}
	filtered := make([]string, 0, len(candidates))
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		if !isUnsubscribeLink(candidate) {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func isUnsubscribeLink(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:")
}

func mailParseDate(s string) (time.Time, error) {
	// net/mail has the most compatible Date parser, but we keep this isolated for easier tests/mocks later.
	return mail.ParseDate(s)
}

// threadItem holds parsed thread metadata for display/JSON output
type threadItem struct {
	ID      string   `json:"id"`
	Date    string   `json:"date,omitempty"`
	From    string   `json:"from,omitempty"`
	Subject string   `json:"subject,omitempty"`
	Labels  []string `json:"labels,omitempty"`
}

// fetchThreadDetails fetches thread metadata concurrently with bounded parallelism.
// This eliminates N+1 queries by fetching all threads in parallel.
func fetchThreadDetails(ctx context.Context, svc *gmail.Service, threads []*gmail.Thread, idToName map[string]string) ([]threadItem, error) {
	if len(threads) == 0 {
		return nil, nil
	}

	const maxConcurrency = 10 // Limit parallel requests to avoid rate limiting
	sem := make(chan struct{}, maxConcurrency)

	type result struct {
		index int
		item  threadItem
		err   error
	}

	results := make(chan result, len(threads))
	var wg sync.WaitGroup

	for i, t := range threads {
		if t.Id == "" {
			continue
		}

		wg.Add(1)
		go func(idx int, threadID string) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- result{index: idx, err: ctx.Err()}
				return
			}

			thread, err := svc.Users.Threads.Get("me", threadID).
				Format("metadata").
				MetadataHeaders("From", "Subject", "Date").
				Context(ctx).
				Do()
			if err != nil {
				results <- result{index: idx, err: err}
				return
			}

			item := threadItem{ID: threadID}
			if msg := firstMessage(thread); msg != nil {
				item.Date = formatGmailDate(headerValue(msg.Payload, "Date"))
				item.From = sanitizeTab(headerValue(msg.Payload, "From"))
				item.Subject = sanitizeTab(headerValue(msg.Payload, "Subject"))
				if len(msg.LabelIds) > 0 {
					names := make([]string, 0, len(msg.LabelIds))
					for _, id := range msg.LabelIds {
						if n, ok := idToName[id]; ok {
							names = append(names, n)
						} else {
							names = append(names, id)
						}
					}
					item.Labels = names
				}
			}

			results <- result{index: idx, item: item}
		}(i, t.Id)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results in order
	ordered := make([]threadItem, len(threads))
	hasErr := false
	for r := range results {
		if r.err != nil {
			hasErr = true
			ordered[r.index] = threadItem{ID: "", Date: "", From: "", Subject: "", Labels: nil}
			continue
		}
		ordered[r.index] = r.item
	}

	if hasErr {
		// Re-run sequentially to find and return the first actual error
		for _, t := range threads {
			if t.Id == "" {
				continue
			}
			_, err := svc.Users.Threads.Get("me", t.Id).
				Format("metadata").
				MetadataHeaders("From", "Subject", "Date").
				Context(ctx).
				Do()
			if err != nil {
				return nil, err
			}
		}
	}

	items := make([]threadItem, 0, len(ordered))
	for _, item := range ordered {
		if item.ID != "" {
			items = append(items, item)
		}
	}
	return items, nil
}
