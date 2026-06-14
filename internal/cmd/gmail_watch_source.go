package cmd

import (
	"context"
	"strings"
	"time"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/gmailwatch"
)

const gmailWatchFormatMetadata = "metadata"

type gmailWatchSource struct {
	service         *gmail.Service
	includeBody     bool
	maxBodyBytes    int
	dateLocation    *time.Location
	excludeLabelIDs map[string]struct{}
	verbose         bool
	logf            func(string, ...any)
}

func newGmailWatchSource(service *gmail.Service, cfg gmailWatchServeConfig, excludeLabelIDs map[string]struct{}, logf func(string, ...any)) *gmailWatchSource {
	return &gmailWatchSource{
		service:         service,
		includeBody:     cfg.IncludeBody,
		maxBodyBytes:    cfg.MaxBodyBytes,
		dateLocation:    cfg.DateLocation,
		excludeLabelIDs: excludeLabelIDs,
		verbose:         cfg.VerboseOutput,
		logf:            logf,
	}
}

func (s *gmailWatchSource) ListHistory(ctx context.Context, startID uint64, maxResults int64, historyTypes []string) (gmailwatch.HistoryPage, error) {
	call := s.service.Users.History.List("me").StartHistoryId(startID).MaxResults(maxResults)
	if len(historyTypes) > 0 {
		call.HistoryTypes(historyTypes...)
	}
	response, err := call.Context(ctx).Do()
	if err != nil {
		return gmailwatch.HistoryPage{}, err
	}

	return historyPageFromGmail(response), nil
}

func (s *gmailWatchSource) ListRecentMessageIDs(ctx context.Context, maxResults int64) ([]string, error) {
	response, err := s.service.Users.Messages.List("me").MaxResults(maxResults).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, nil
	}

	ids := make([]string, 0, len(response.Messages))
	for _, message := range response.Messages {
		if message != nil && strings.TrimSpace(message.Id) != "" {
			ids = append(ids, strings.TrimSpace(message.Id))
		}
	}

	return ids, nil
}

func (s *gmailWatchSource) FetchMessages(ctx context.Context, ids []string) (gmailwatch.MessageBatch, error) {
	batch := gmailwatch.MessageBatch{
		Messages: make([]gmailwatch.Message, 0, len(ids)),
	}
	format := gmailWatchFormatMetadata
	if s.includeBody {
		format = gmailFormatFull
	}

	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		message, err := s.service.Users.Messages.Get("me", id).
			Format(format).
			MetadataHeaders(gmailBasicMetadataHeaders...).
			Context(ctx).
			Do()
		if err != nil {
			if isNotFoundAPIError(err) {
				continue
			}
			return gmailwatch.MessageBatch{}, err
		}
		if message == nil {
			continue
		}
		if labelSetContains(s.excludeLabelIDs, message.LabelIds) {
			batch.Excluded++
			if s.verbose && s.logf != nil {
				s.logf("watch: excluded message %s labels=%v", message.Id, message.LabelIds)
			}
			continue
		}

		item := gmailwatch.Message{
			ID:       message.Id,
			ThreadID: message.ThreadId,
			From:     headerValue(message.Payload, "From"),
			To:       headerValue(message.Payload, "To"),
			Subject:  headerValue(message.Payload, "Subject"),
			Date:     formatGmailDateInLocation(headerValue(message.Payload, "Date"), s.dateLocation),
			Snippet:  message.Snippet,
			Labels:   message.LabelIds,
		}
		if s.includeBody {
			body := bestBodyText(message.Payload)
			item.Body, item.BodyTruncated = truncateUTF8Bytes(body, s.maxBodyBytes)
		}
		batch.Messages = append(batch.Messages, item)
	}

	return batch, nil
}

func historyPageFromGmail(response *gmail.ListHistoryResponse) gmailwatch.HistoryPage {
	if response == nil {
		return gmailwatch.HistoryPage{}
	}

	page := gmailwatch.HistoryPage{
		HistoryID: formatHistoryID(response.HistoryId),
		Records:   make([]gmailwatch.HistoryRecord, 0, len(response.History)),
	}
	for _, history := range response.History {
		if history == nil {
			continue
		}
		page.Records = append(page.Records, gmailwatch.HistoryRecord{
			Added:         gmailHistoryAddedIDs(history.MessagesAdded),
			Deleted:       gmailHistoryDeletedIDs(history.MessagesDeleted),
			LabelsAdded:   gmailHistoryLabelAddedIDs(history.LabelsAdded),
			LabelsRemoved: gmailHistoryLabelRemovedIDs(history.LabelsRemoved),
			Messages:      gmailMessageIDs(history.Messages),
		})
	}

	return page
}

func collectHistoryMessageIDs(response *gmail.ListHistoryResponse) gmailwatch.HistoryMessageIDs {
	return gmailwatch.CollectHistoryMessageIDs(historyPageFromGmail(response).Records)
}

func gmailHistoryAddedIDs(items []*gmail.HistoryMessageAdded) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil {
			ids = appendMessageID(ids, item.Message)
		}
	}
	return ids
}

func gmailHistoryDeletedIDs(items []*gmail.HistoryMessageDeleted) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil {
			ids = appendMessageID(ids, item.Message)
		}
	}
	return ids
}

func gmailHistoryLabelAddedIDs(items []*gmail.HistoryLabelAdded) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil {
			ids = appendMessageID(ids, item.Message)
		}
	}
	return ids
}

func gmailHistoryLabelRemovedIDs(items []*gmail.HistoryLabelRemoved) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil {
			ids = appendMessageID(ids, item.Message)
		}
	}
	return ids
}

func gmailMessageIDs(messages []*gmail.Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = appendMessageID(ids, message)
	}
	return ids
}

func appendMessageID(ids []string, message *gmail.Message) []string {
	if message == nil {
		return ids
	}
	id := strings.TrimSpace(message.Id)
	if id == "" {
		return ids
	}
	return append(ids, id)
}

func labelSetContains(labels map[string]struct{}, candidates []string) bool {
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		if _, ok := labels[trimmed]; ok {
			return true
		}
	}
	return false
}
