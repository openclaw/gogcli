//nolint:tagliatelle // Gmail watch hook payloads retain their existing camelCase schema.
package gmailwatch

import "context"

type Message struct {
	ID            string   `json:"id"`
	ThreadID      string   `json:"threadId"`
	From          string   `json:"from,omitempty"`
	To            string   `json:"to,omitempty"`
	Subject       string   `json:"subject,omitempty"`
	Date          string   `json:"date,omitempty"`
	Snippet       string   `json:"snippet,omitempty"`
	Body          string   `json:"body,omitempty"`
	BodyTruncated bool     `json:"bodyTruncated,omitempty"`
	Labels        []string `json:"labels,omitempty"`
}

type Payload struct {
	Source            string    `json:"source"`
	Account           string    `json:"account"`
	HistoryID         string    `json:"historyId"`
	Messages          []Message `json:"messages"`
	DeletedMessageIDs []string  `json:"deletedMessageIds,omitempty"`
}

type HistoryRecord struct {
	Added         []string
	Deleted       []string
	LabelsAdded   []string
	LabelsRemoved []string
	Messages      []string
}

type HistoryPage struct {
	HistoryID string
	Records   []HistoryRecord
}

type HistoryMessageIDs struct {
	FetchIDs   []string
	DeletedIDs []string
}

type MessageBatch struct {
	Messages []Message
	Excluded int
}

type Source interface {
	ListHistory(context.Context, uint64, int64, []string) (HistoryPage, error)
	ListRecentMessageIDs(context.Context, int64) ([]string, error)
	FetchMessages(context.Context, []string) (MessageBatch, error)
}

func CollectHistoryMessageIDs(records []HistoryRecord) HistoryMessageIDs {
	fetchIndex := make(map[string]int)
	seenDeleted := make(map[string]struct{})
	var result HistoryMessageIDs

	addFetch := func(id string) {
		if id == "" {
			return
		}

		if _, ok := fetchIndex[id]; ok {
			return
		}

		if _, ok := seenDeleted[id]; ok {
			return
		}

		fetchIndex[id] = len(result.FetchIDs)
		result.FetchIDs = append(result.FetchIDs, id)
	}

	addDeleted := func(id string) {
		if id == "" {
			return
		}

		if _, ok := seenDeleted[id]; ok {
			return
		}

		if index, ok := fetchIndex[id]; ok {
			delete(fetchIndex, id)
			result.FetchIDs[index] = ""
		}

		seenDeleted[id] = struct{}{}
		result.DeletedIDs = append(result.DeletedIDs, id)
	}

	for _, record := range records {
		for _, id := range record.Added {
			addFetch(id)
		}

		for _, id := range record.Deleted {
			addDeleted(id)
		}

		for _, id := range record.LabelsAdded {
			addFetch(id)
		}

		for _, id := range record.LabelsRemoved {
			addFetch(id)
		}

		for _, id := range record.Messages {
			addFetch(id)
		}
	}

	if len(fetchIndex) != len(result.FetchIDs) {
		compacted := result.FetchIDs[:0]
		for _, id := range result.FetchIDs {
			if id != "" {
				compacted = append(compacted, id)
			}
		}
		result.FetchIDs = compacted
	}

	return result
}
