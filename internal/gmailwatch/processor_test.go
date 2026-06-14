package gmailwatch

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

var (
	errStaleHistory = errors.New("stale history")
	errRateLimited  = errors.New("rate limited")
)

type processorSource struct {
	history        HistoryPage
	historyErr     error
	recentIDs      []string
	recentErr      error
	messageBatches []MessageBatch
	messageErrs    []error
	historyCalls   int
	recentCalls    int
	messageCalls   int
}

func (s *processorSource) ListHistory(context.Context, uint64, int64, []string) (HistoryPage, error) {
	s.historyCalls++

	return s.history, s.historyErr
}

func (s *processorSource) ListRecentMessageIDs(context.Context, int64) ([]string, error) {
	s.recentCalls++

	return append([]string(nil), s.recentIDs...), s.recentErr
}

func (s *processorSource) FetchMessages(context.Context, []string) (MessageBatch, error) {
	index := s.messageCalls

	s.messageCalls++

	if index < len(s.messageErrs) && s.messageErrs[index] != nil {
		return MessageBatch{}, s.messageErrs[index]
	}

	if index < len(s.messageBatches) {
		return s.messageBatches[index], nil
	}

	return MessageBatch{}, nil
}

func TestProcessorHandleHistory(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	repository := NewMemory(State{Account: "a@example.com", HistoryID: "100"}, Options{Now: func() time.Time { return now }})
	source := &processorSource{
		history: HistoryPage{
			HistoryID: "201",
			Records: []HistoryRecord{{
				Added:   []string{"m1"},
				Deleted: []string{"m2"},
			}},
		},
		messageBatches: []MessageBatch{{Messages: []Message{{ID: "m1"}}}},
	}
	processor := newTestProcessor(repository, source, now)

	payload, err := processor.Handle(context.Background(), Notification{HistoryID: "200", MessageID: "push-1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if payload.HistoryID != "201" || len(payload.Messages) != 1 || payload.Messages[0].ID != "m1" {
		t.Fatalf("payload = %#v", payload)
	}

	if want := []string{"m2"}; !reflect.DeepEqual(payload.DeletedMessageIDs, want) {
		t.Fatalf("deleted IDs = %v, want %v", payload.DeletedMessageIDs, want)
	}

	state := repository.Get()
	if state.HistoryID != "201" || state.LastPushMessageID != "push-1" {
		t.Fatalf("state = %#v", state)
	}
}

func TestProcessorSuppressesDuplicateAndStaleNotifications(t *testing.T) {
	t.Parallel()

	repository := NewMemory(State{HistoryID: "200", LastPushMessageID: "duplicate"}, Options{})
	sourceCalls := 0
	processor := &Processor{
		Repository: repository,
		NewSource: func(context.Context) (Source, error) {
			sourceCalls++

			return &processorSource{}, nil
		},
	}

	if _, err := processor.Handle(context.Background(), Notification{HistoryID: "201", MessageID: "duplicate"}); !errors.Is(err, ErrNoNewMessages) {
		t.Fatalf("duplicate error = %v", err)
	}

	if _, err := processor.Handle(context.Background(), Notification{HistoryID: "199", MessageID: "new"}); !errors.Is(err, ErrNoNewMessages) {
		t.Fatalf("stale error = %v", err)
	}

	if sourceCalls != 0 {
		t.Fatalf("source calls = %d", sourceCalls)
	}
}

func TestProcessorResyncsStaleHistory(t *testing.T) {
	t.Parallel()

	now := time.Unix(200, 0)
	repository := NewMemory(State{HistoryID: "100"}, Options{})
	source := &processorSource{
		historyErr: errStaleHistory,
		recentIDs:  []string{"m1"},
		messageBatches: []MessageBatch{{
			Messages: []Message{{ID: "m1"}},
		}},
	}
	processor := newTestProcessor(repository, source, now)
	processor.IsStaleHistoryError = func(err error) bool {
		return errors.Is(err, errStaleHistory)
	}

	payload, err := processor.Handle(context.Background(), Notification{HistoryID: "200"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if payload.HistoryID != "200" || len(payload.Messages) != 1 {
		t.Fatalf("payload = %#v", payload)
	}

	if source.recentCalls != 1 || source.messageCalls != 1 {
		t.Fatalf("source calls = recent %d messages %d", source.recentCalls, source.messageCalls)
	}

	if repository.Get().HistoryID != "200" {
		t.Fatalf("state = %#v", repository.Get())
	}
}

func TestProcessorRateLimitCircuit(t *testing.T) {
	t.Parallel()

	now := time.Unix(300, 0)
	repository := NewMemory(State{HistoryID: "100"}, Options{})
	source := &processorSource{historyErr: errRateLimited}
	processor := newTestProcessor(repository, source, now)
	processor.RateLimitUntil = func(err error, at time.Time) (time.Time, bool) {
		if !errors.Is(err, errRateLimited) {
			return time.Time{}, false
		}

		return at.Add(time.Minute), true
	}

	_, err := processor.Handle(context.Background(), Notification{HistoryID: "200"})

	var got *RateLimitError
	if !errors.As(err, &got) || !got.Until.Equal(now.Add(time.Minute)) {
		t.Fatalf("rate limit error = %#v", err)
	}

	source.historyErr = nil
	got = nil

	_, err = processor.Handle(context.Background(), Notification{HistoryID: "201"})
	if !errors.As(err, &got) {
		t.Fatalf("open circuit error = %#v", err)
	}

	if source.historyCalls != 1 {
		t.Fatalf("history calls = %d", source.historyCalls)
	}
}

func TestProcessorAdvancesEmptyFilteredHistory(t *testing.T) {
	t.Parallel()

	now := time.Unix(400, 0)
	repository := NewMemory(State{HistoryID: "100"}, Options{})
	source := &processorSource{history: HistoryPage{HistoryID: "200"}}
	processor := newTestProcessor(repository, source, now)
	processor.Config.HistoryTypes = []string{"messageAdded"}

	if _, err := processor.Handle(context.Background(), Notification{HistoryID: "200", MessageID: "push"}); !errors.Is(err, ErrNoNewMessages) {
		t.Fatalf("Handle error = %v", err)
	}

	state := repository.Get()
	if state.HistoryID != "200" || state.LastPushMessageID != "push" {
		t.Fatalf("state = %#v", state)
	}

	if source.messageCalls != 0 {
		t.Fatalf("message calls = %d", source.messageCalls)
	}
}

func TestProcessorSkipsAllExcludedMessages(t *testing.T) {
	t.Parallel()

	repository := NewMemory(State{HistoryID: "100"}, Options{})
	source := &processorSource{
		history: HistoryPage{
			HistoryID: "200",
			Records:   []HistoryRecord{{Added: []string{"m1"}}},
		},
		messageBatches: []MessageBatch{{Excluded: 1}},
	}
	processor := newTestProcessor(repository, source, time.Unix(500, 0))

	if _, err := processor.Handle(context.Background(), Notification{HistoryID: "200"}); !errors.Is(err, ErrNoNewMessages) {
		t.Fatalf("Handle error = %v", err)
	}

	if repository.Get().HistoryID != "200" {
		t.Fatalf("state = %#v", repository.Get())
	}
}

func TestProcessorProcessRecordsDelivery(t *testing.T) {
	t.Parallel()

	now := time.Unix(600, 0)
	repository := NewMemory(State{HistoryID: "100"}, Options{})
	source := &processorSource{
		history: HistoryPage{
			HistoryID: "200",
			Records:   []HistoryRecord{{Added: []string{"m1"}}},
		},
		messageBatches: []MessageBatch{{Messages: []Message{{ID: "m1"}}}},
	}
	processor := newTestProcessor(repository, source, now)
	processor.Deliver = func(context.Context, *Payload) DeliveryResult {
		return DeliveryResult{Status: DeliveryStatusOK, Record: true}
	}

	processed, err := processor.Process(context.Background(), Notification{HistoryID: "200", MessageID: "push"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	if processed == nil || processed.Payload == nil || processed.HookFailed {
		t.Fatalf("processed = %#v", processed)
	}

	state := repository.Get()
	if state.LastDeliveryStatus != DeliveryStatusOK || state.LastDeliveryAtMs != now.UnixMilli() {
		t.Fatalf("state = %#v", state)
	}
}

func TestProcessorProcessRestoresProgressAfterDeliveryFailure(t *testing.T) {
	t.Parallel()

	now := time.Unix(700, 0)
	repository := NewMemory(State{HistoryID: "100", LastPushMessageID: "before"}, Options{})
	source := &processorSource{
		history: HistoryPage{
			HistoryID: "200",
			Records:   []HistoryRecord{{Added: []string{"m1"}}},
		},
		messageBatches: []MessageBatch{{Messages: []Message{{ID: "m1"}}}},
	}
	deliveryErr := errors.New("delivery failed") //nolint:err113 // Test-only injected failure.
	processor := newTestProcessor(repository, source, now)
	processor.Deliver = func(context.Context, *Payload) DeliveryResult {
		return DeliveryResult{
			Status: DeliveryStatusHTTPError,
			Note:   "status 502",
			Err:    deliveryErr,
			Record: true,
		}
	}

	processed, err := processor.Process(context.Background(), Notification{HistoryID: "200", MessageID: "push"})

	var hookErr *HookDeliveryError
	if !errors.As(err, &hookErr) || !errors.Is(err, deliveryErr) {
		t.Fatalf("Process error = %v", err)
	}

	if processed == nil || !processed.HookFailed {
		t.Fatalf("processed = %#v", processed)
	}

	state := repository.Get()
	if state.HistoryID != "100" || state.LastPushMessageID != "before" {
		t.Fatalf("progress state = %#v", state)
	}

	if state.LastDeliveryStatus != DeliveryStatusHTTPError || state.LastDeliveryStatusNote != "status 502" {
		t.Fatalf("delivery state = %#v", state)
	}
}

func newTestProcessor(repository *Repository, source Source, now time.Time) *Processor {
	return &Processor{
		Config: ProcessorConfig{
			Account:    "a@example.com",
			HistoryMax: 100,
			ResyncMax:  10,
		},
		Repository: repository,
		NewSource: func(context.Context) (Source, error) {
			return source, nil
		},
		Now: func() time.Time {
			return now
		},
	}
}
