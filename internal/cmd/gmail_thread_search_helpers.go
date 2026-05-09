package cmd

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/gmail/v1"
)

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

type threadItem struct {
	ID           string   `json:"id"`
	Date         string   `json:"date,omitempty"`
	From         string   `json:"from,omitempty"`
	Subject      string   `json:"subject,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	MessageCount int      `json:"messageCount,omitempty"`
}

func fetchThreadDetails(ctx context.Context, svc *gmail.Service, threads []*gmail.Thread, idToName map[string]string, oldest bool, loc *time.Location) ([]threadItem, error) {
	if len(threads) == 0 {
		return nil, nil
	}

	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)

	type result struct {
		index int
		item  threadItem
		err   error
	}

	results := make(chan result, len(threads))
	var wg sync.WaitGroup

	for i, thread := range threads {
		if thread == nil || thread.Id == "" {
			continue
		}

		wg.Add(1)
		go func(idx int, threadID string) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- result{index: idx, err: ctx.Err()}
				return
			}

			fullThread, err := svc.Users.Threads.Get("me", threadID).
				Format("metadata").
				MetadataHeaders(gmailMessageSummaryMetadataHeaders...).
				Context(ctx).
				Do()
			if err != nil {
				results <- result{index: idx, err: err}
				return
			}

			item := threadItem{ID: threadID, MessageCount: len(fullThread.Messages)}
			if first := firstMessage(fullThread); first != nil {
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

			dateMsg := newestMessageByDate(fullThread)
			if oldest {
				dateMsg = oldestMessageByDate(fullThread)
			}
			if dateMsg != nil {
				item.Date = formatGmailDateInLocation(headerValue(dateMsg.Payload, "Date"), loc)
			}

			results <- result{index: idx, item: item}
		}(i, thread.Id)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	ordered := make([]threadItem, len(threads))
	hasErr := false
	for r := range results {
		if r.err != nil {
			hasErr = true
			continue
		}
		ordered[r.index] = r.item
	}

	if hasErr {
		for _, thread := range threads {
			if thread == nil || thread.Id == "" {
				continue
			}
			_, err := svc.Users.Threads.Get("me", thread.Id).
				Format("metadata").
				MetadataHeaders(gmailMessageSummaryMetadataHeaders...).
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
