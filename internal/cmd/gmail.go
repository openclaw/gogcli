package cmd

import (
	"context"
	"fmt"
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
	Search     GmailSearchCmd     `cmd:"" name:"search" aliases:"find,query,ls,list" group:"Read" help:"Search threads using Gmail query syntax"`
	Messages   GmailMessagesCmd   `cmd:"" name:"messages" aliases:"message,msg,msgs" group:"Read" help:"Message operations"`
	Thread     GmailThreadCmd     `cmd:"" name:"thread" aliases:"threads,read" group:"Organize" help:"Thread operations (get, modify)"`
	Get        GmailGetCmd        `cmd:"" name:"get" aliases:"info,show" group:"Read" help:"Get a message (full|metadata|raw)"`
	Attachment GmailAttachmentCmd `cmd:"" name:"attachment" group:"Read" help:"Download a single attachment"`
	URL        GmailURLCmd        `cmd:"" name:"url" group:"Read" help:"Print Gmail web URLs for threads"`
	History    GmailHistoryCmd    `cmd:"" name:"history" group:"Read" help:"Gmail history"`

	Labels GmailLabelsCmd `cmd:"" name:"labels" aliases:"label" group:"Organize" help:"Label operations"`
	Batch  GmailBatchCmd  `cmd:"" name:"batch" group:"Organize" help:"Batch operations"`

	Send   GmailSendCmd   `cmd:"" name:"send" group:"Write" help:"Send an email"`
	Track  GmailTrackCmd  `cmd:"" name:"track" group:"Write" help:"Email open tracking"`
	Drafts GmailDraftsCmd `cmd:"" name:"drafts" aliases:"draft" group:"Write" help:"Draft operations"`

	Settings GmailSettingsCmd `cmd:"" name:"settings" group:"Admin" help:"Settings and admin"`

	// Kept for backwards-compatibility; hidden from default help.
	Watch       GmailWatchCmd       `cmd:"" name:"watch" hidden:"" help:"Manage Gmail watch"`
	AutoForward GmailAutoForwardCmd `cmd:"" name:"autoforward" hidden:"" help:"Auto-forwarding settings"`
	Delegates   GmailDelegatesCmd   `cmd:"" name:"delegates" hidden:"" help:"Delegate operations"`
	Filters     GmailFiltersCmd     `cmd:"" name:"filters" hidden:"" help:"Filter operations"`
	Forwarding  GmailForwardingCmd  `cmd:"" name:"forwarding" hidden:"" help:"Forwarding addresses"`
	SendAs      GmailSendAsCmd      `cmd:"" name:"sendas" hidden:"" help:"Send-as settings"`
	Vacation    GmailVacationCmd    `cmd:"" name:"vacation" hidden:"" help:"Vacation responder"`
}

type GmailSettingsCmd struct {
	Filters     GmailFiltersCmd     `cmd:"" name:"filters" group:"Organize" help:"Filter operations"`
	Delegates   GmailDelegatesCmd   `cmd:"" name:"delegates" group:"Admin" help:"Delegate operations"`
	Forwarding  GmailForwardingCmd  `cmd:"" name:"forwarding" group:"Admin" help:"Forwarding addresses"`
	AutoForward GmailAutoForwardCmd `cmd:"" name:"autoforward" group:"Admin" help:"Auto-forwarding settings"`
	SendAs      GmailSendAsCmd      `cmd:"" name:"sendas" group:"Admin" help:"Send-as settings"`
	Vacation    GmailVacationCmd    `cmd:"" name:"vacation" group:"Admin" help:"Vacation responder"`
	Watch       GmailWatchCmd       `cmd:"" name:"watch" group:"Admin" help:"Manage Gmail watch"`
}

type GmailSearchCmd struct {
	Query     []string `arg:"" name:"query" help:"Search query"`
	Max       int64    `name:"max" aliases:"limit" help:"Max results" default:"10"`
	Page      string   `name:"page" aliases:"cursor" help:"Page token"`
	All       bool     `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	Oldest    bool     `name:"oldest" help:"Show first message date instead of last"`
	Timezone  string   `name:"timezone" short:"z" help:"Output timezone (IANA name, e.g. America/New_York, UTC). Default: local"`
	Local     bool     `name:"local" help:"Use local timezone (default behavior, useful to override --timezone)"`
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

	fetch := func(pageToken string) ([]*gmail.Thread, string, error) {
		call := svc.Users.Threads.List("me").
			Q(query).
			MaxResults(c.Max).
			Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.Threads, resp.NextPageToken, nil
	}

	var threads []*gmail.Thread
	nextPageToken := ""
	if c.All {
		all, collectErr := collectAllPages(c.Page, fetch)
		if collectErr != nil {
			return collectErr
		}
		threads = all
	} else {
		threadsPage, pageToken, fetchErr := fetch(c.Page)
		if fetchErr != nil {
			return fetchErr
		}
		threads = threadsPage
		nextPageToken = pageToken
	}

	if len(threads) == 0 {
		if outfmt.IsJSON(ctx) {
			if writeErr := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
				"threads":       []threadItem{},
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

	// Fetch thread details concurrently (fixes N+1 query pattern)
	items, err := fetchThreadDetails(ctx, svc, threads, idToName, c.Oldest, loc)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		if writeErr := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"threads":       items,
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

	fmt.Fprintln(w, "ID\tDATE\tFROM\tSUBJECT\tLABELS\tTHREAD")
	for _, it := range items {
		threadInfo := "-"
		if it.MessageCount > 1 {
			threadInfo = fmt.Sprintf("[%d msgs]", it.MessageCount)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", it.ID, it.Date, it.From, it.Subject, strings.Join(it.Labels, ","), threadInfo)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

func firstMessage(t *gmail.Thread) *gmail.Message {
	if t == nil || len(t.Messages) == 0 {
		return nil
	}
	return t.Messages[0]
}

func lastMessage(t *gmail.Thread) *gmail.Message {
	if t == nil || len(t.Messages) == 0 {
		return nil
	}
	return t.Messages[len(t.Messages)-1]
}

func messageDateMillis(msg *gmail.Message) int64 {
	if msg == nil {
		return 0
	}
	if msg.InternalDate > 0 {
		return msg.InternalDate
	}
	if msg.Payload == nil {
		return 0
	}
	raw := headerValue(msg.Payload, "Date")
	if raw == "" {
		return 0
	}
	parsed, err := mailParseDate(raw)
	if err != nil {
		return 0
	}
	return parsed.UnixMilli()
}

func messageByDate(t *gmail.Thread, oldest bool) *gmail.Message {
	if t == nil || len(t.Messages) == 0 {
		return nil
	}
	var picked *gmail.Message
	var pickedDate int64
	for _, msg := range t.Messages {
		if msg == nil {
			continue
		}
		date := messageDateMillis(msg)
		if date == 0 {
			continue
		}
		if picked == nil {
			picked = msg
			pickedDate = date
			continue
		}
		if oldest {
			if date < pickedDate {
				picked = msg
				pickedDate = date
			}
			continue
		}
		if date > pickedDate {
			picked = msg
			pickedDate = date
		}
	}
	if picked != nil {
		return picked
	}
	if oldest {
		return firstMessage(t)
	}
	return lastMessage(t)
}

func newestMessageByDate(t *gmail.Thread) *gmail.Message {
	return messageByDate(t, false)
}

func oldestMessageByDate(t *gmail.Thread) *gmail.Message {
	return messageByDate(t, true)
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

var listUnsubscribeLinkPattern = regexp.MustCompile(`<([^>]+)>`)

func bestUnsubscribeLink(p *gmail.MessagePart) string {
	links := parseListUnsubscribe(headerValue(p, "List-Unsubscribe"))
	if len(links) == 0 {
		return ""
	}
	var httpLink string
	var mailtoLink string
	for _, link := range links {
		lower := strings.ToLower(link)
		if strings.HasPrefix(lower, "https://") {
			return link
		}
		if strings.HasPrefix(lower, "http://") && httpLink == "" {
			httpLink = link
			continue
		}
		if strings.HasPrefix(lower, "mailto:") && mailtoLink == "" {
			mailtoLink = link
		}
	}
	if httpLink != "" {
		return httpLink
	}
	if mailtoLink != "" {
		return mailtoLink
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
	}
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		candidate := strings.TrimSpace(strings.Trim(part, "<>\""))
		if candidate == "" {
			continue
		}
		candidates = append(candidates, candidate)
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

// threadItem holds parsed thread metadata for display/JSON output
type threadItem struct {
	ID           string   `json:"id"`
	Date         string   `json:"date,omitempty"`
	From         string   `json:"from,omitempty"`
	Subject      string   `json:"subject,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	MessageCount int      `json:"messageCount,omitempty"` // Number of messages in the thread
}

// fetchThreadDetails fetches thread metadata concurrently with bounded parallelism.
// This eliminates N+1 queries by fetching all threads in parallel.
// When oldest is false (default), the date shown is from the last message in the thread.
// When oldest is true, the date shown is from the first message in the thread.
func fetchThreadDetails(ctx context.Context, svc *gmail.Service, threads []*gmail.Thread, idToName map[string]string, oldest bool, loc *time.Location) ([]threadItem, error) {
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

			item := threadItem{ID: threadID, MessageCount: len(thread.Messages)}
			if first := firstMessage(thread); first != nil {
				item.From = sanitizeTab(headerValue(first.Payload, "From"))
				item.Subject = sanitizeTab(headerValue(first.Payload, "Subject"))
				if len(first.LabelIds) > 0 {
					names := make([]string, 0, len(first.LabelIds))
					for _, lid := range first.LabelIds {
						if n, ok := idToName[lid]; ok {
							names = append(names, n)
						} else {
							names = append(names, lid)
						}
					}
					item.Labels = names
				}
			}
			// Date from newest message by default, oldest if --oldest
			dateMsg := newestMessageByDate(thread)
			if oldest {
				dateMsg = oldestMessageByDate(thread)
			}
			if dateMsg != nil {
				item.Date = formatGmailDateInLocation(headerValue(dateMsg.Payload, "Date"), loc)
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
			ordered[r.index] = threadItem{ID: "", Date: "", From: "", Subject: "", Labels: nil, MessageCount: 0}
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
