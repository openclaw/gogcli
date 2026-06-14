//nolint:tagliatelle,wsl_v5 // Persisted Gmail backup rows retain their existing camelCase schema.
package gmailbackup

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/api/gmail/v1"
)

var (
	errSourceRequired     = errors.New("gmail backup source is required")
	errMessageIDRequired  = errors.New("gmail backup message ID is required")
	errMessageRawRequired = errors.New("gmail backup message raw payload is required")
)

type Label struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Type                  string `json:"type,omitempty"`
	MessageListVisibility string `json:"messageListVisibility,omitempty"`
	LabelListVisibility   string `json:"labelListVisibility,omitempty"`
	MessagesTotal         int64  `json:"messagesTotal,omitempty"`
	MessagesUnread        int64  `json:"messagesUnread,omitempty"`
	ThreadsTotal          int64  `json:"threadsTotal,omitempty"`
	ThreadsUnread         int64  `json:"threadsUnread,omitempty"`
}

type ListRequest struct {
	Query            string
	MaxResults       int64
	IncludeSpamTrash bool
	PageToken        string
}

type ListPage struct {
	IDs           []string
	NextPageToken string
}

type Source interface {
	Labels(context.Context) ([]Label, error)
	ListMessageIDs(context.Context, ListRequest) (ListPage, error)
	RawMessage(context.Context, string) (Message, error)
}

type ServiceSource struct {
	service *gmail.Service
}

func NewServiceSource(service *gmail.Service) (*ServiceSource, error) {
	if service == nil {
		return nil, errSourceRequired
	}
	return &ServiceSource{service: service}, nil
}

func (s *ServiceSource) Labels(ctx context.Context) ([]Label, error) {
	resp, err := s.service.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list Gmail backup labels: %w", err)
	}
	out := make([]Label, 0, len(resp.Labels))
	for _, label := range resp.Labels {
		if label == nil {
			continue
		}
		out = append(out, Label{
			ID:                    label.Id,
			Name:                  label.Name,
			Type:                  label.Type,
			MessageListVisibility: label.MessageListVisibility,
			LabelListVisibility:   label.LabelListVisibility,
			MessagesTotal:         label.MessagesTotal,
			MessagesUnread:        label.MessagesUnread,
			ThreadsTotal:          label.ThreadsTotal,
			ThreadsUnread:         label.ThreadsUnread,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *ServiceSource) ListMessageIDs(ctx context.Context, req ListRequest) (ListPage, error) {
	call := s.service.Users.Messages.List("me").
		MaxResults(req.MaxResults).
		IncludeSpamTrash(req.IncludeSpamTrash).
		Fields("messages(id),nextPageToken").
		Context(ctx)
	if strings.TrimSpace(req.Query) != "" {
		call = call.Q(req.Query)
	}
	if strings.TrimSpace(req.PageToken) != "" {
		call = call.PageToken(req.PageToken)
	}
	resp, err := call.Do()
	if err != nil {
		return ListPage{}, fmt.Errorf("list Gmail backup messages: %w", err)
	}
	ids := make([]string, 0, len(resp.Messages))
	for _, message := range resp.Messages {
		if message == nil || strings.TrimSpace(message.Id) == "" {
			continue
		}
		ids = append(ids, message.Id)
	}
	return ListPage{IDs: ids, NextPageToken: resp.NextPageToken}, nil
}

func (s *ServiceSource) RawMessage(ctx context.Context, messageID string) (Message, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return Message{}, errMessageIDRequired
	}
	msg, err := s.service.Users.Messages.Get("me", messageID).
		Format("raw").
		Fields("id,threadId,historyId,internalDate,labelIds,sizeEstimate,raw").
		Context(ctx).
		Do()
	if err != nil {
		return Message{}, fmt.Errorf("gmail message %s: %w", messageID, err)
	}
	if strings.TrimSpace(msg.Id) == "" {
		return Message{}, fmt.Errorf("%w: requested %s", errMessageIDRequired, messageID)
	}
	if strings.TrimSpace(msg.Raw) == "" {
		return Message{}, fmt.Errorf("%w: %s", errMessageRawRequired, messageID)
	}
	return Message{
		ID:           msg.Id,
		ThreadID:     msg.ThreadId,
		HistoryID:    strconv.FormatUint(msg.HistoryId, 10),
		InternalDate: msg.InternalDate,
		LabelIDs:     append([]string(nil), msg.LabelIds...),
		SizeEstimate: msg.SizeEstimate,
		Raw:          msg.Raw,
	}, nil
}
