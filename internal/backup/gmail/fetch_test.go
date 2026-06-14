//nolint:wsl_v5 // Test setup and assertions stay grouped for scanability.
package gmailbackup

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestListMessageIDsResumesDeduplicatesAndCompletes(t *testing.T) {
	t.Parallel()
	cache := newPlannerCache(t)
	selection := Selection{
		AccountHash:      "accthash",
		Query:            "in:anywhere",
		IncludeSpamTrash: true,
	}
	if err := cache.WriteListState(selection, []string{"m1", "m1", ""}, "p2", false); err != nil {
		t.Fatalf("WriteListState: %v", err)
	}
	source := &fakeSource{
		listPages: map[string]ListPage{
			"p2": {IDs: []string{"m1", "", "m2"}, NextPageToken: ""},
		},
	}
	var events []Event
	ids, err := ListMessageIDs(context.Background(), source, ListOptions{
		Selection: selection,
		Cache:     cache,
		UseCache:  true,
		Progress:  func(event Event) { events = append(events, event) },
	})
	if err != nil {
		t.Fatalf("ListMessageIDs: %v", err)
	}
	if fmt.Sprint(ids) != "[m1 m2]" {
		t.Fatalf("ids = %v", ids)
	}
	if len(source.listRequests) != 1 || source.listRequests[0].PageToken != "p2" {
		t.Fatalf("requests = %+v", source.listRequests)
	}
	state, found, err := cache.ReadListState(selection)
	if err != nil {
		t.Fatalf("ReadListState: %v", err)
	}
	if !found || !state.Complete || fmt.Sprint(state.IDs) != "[m1 m2]" {
		t.Fatalf("state = %+v found=%t", state, found)
	}
	if len(events) < 2 || events[0].Resume != "partial" {
		t.Fatalf("events = %+v", events)
	}
}

func TestListMessageIDsReusesCompleteStateWithoutSource(t *testing.T) {
	t.Parallel()
	cache := newPlannerCache(t)
	selection := Selection{AccountHash: "accthash"}
	if err := cache.WriteListState(selection, []string{"m1", "m2"}, "", true); err != nil {
		t.Fatalf("WriteListState: %v", err)
	}
	source := &fakeSource{}
	ids, err := ListMessageIDs(context.Background(), source, ListOptions{
		Selection: selection,
		Cache:     cache,
		UseCache:  true,
	})
	if err != nil {
		t.Fatalf("ListMessageIDs: %v", err)
	}
	if fmt.Sprint(ids) != "[m1 m2]" || len(source.listRequests) != 0 {
		t.Fatalf("ids=%v requests=%+v", ids, source.listRequests)
	}
}

func TestListMessageIDsHonorsMaxAfterDeduplication(t *testing.T) {
	t.Parallel()
	source := &fakeSource{
		listPages: map[string]ListPage{
			"": {IDs: []string{"m1", "m1", "", "m2", "m3"}, NextPageToken: "unused"},
		},
	}
	ids, err := ListMessageIDs(context.Background(), source, ListOptions{
		Selection: Selection{Max: 2},
	})
	if err != nil {
		t.Fatalf("ListMessageIDs: %v", err)
	}
	if fmt.Sprint(ids) != "[m1 m2]" {
		t.Fatalf("ids = %v", ids)
	}
}

func TestListMessageIDsMarksMaxedPartialStateComplete(t *testing.T) {
	t.Parallel()
	cache := newPlannerCache(t)
	selection := Selection{AccountHash: "accthash", Max: 1}
	if err := cache.WriteListState(selection, []string{"m1"}, "unused", false); err != nil {
		t.Fatalf("WriteListState: %v", err)
	}
	source := &fakeSource{}
	ids, err := ListMessageIDs(context.Background(), source, ListOptions{
		Selection: selection,
		Cache:     cache,
		UseCache:  true,
	})
	if err != nil {
		t.Fatalf("ListMessageIDs: %v", err)
	}
	state, found, err := cache.ReadListState(selection)
	if err != nil {
		t.Fatalf("ReadListState: %v", err)
	}
	if fmt.Sprint(ids) != "[m1]" || !found || !state.Complete || state.PageToken != "" {
		t.Fatalf("ids=%v state=%+v found=%t", ids, state, found)
	}
	if len(source.listRequests) != 0 {
		t.Fatalf("requests = %+v", source.listRequests)
	}
}

func TestFetchMessagesPreservesInputOrderAcrossWorkers(t *testing.T) {
	t.Parallel()
	source := &fakeSource{
		messages: map[string]Message{
			"m1": {ID: "m1", Raw: "raw-1"},
			"m2": {ID: "m2", Raw: "raw-2"},
		},
		delays: map[string]time.Duration{
			"m1": 25 * time.Millisecond,
			"m2": time.Millisecond,
		},
	}
	var completed []string
	messages, stats, err := FetchMessages(context.Background(), source, []string{"m1", "m2", "m1", ""}, FetchOptions{
		Concurrency: 2,
		AfterMessage: func(_ context.Context, id string, _ Event) error {
			completed = append(completed, id)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("FetchMessages: %v", err)
	}
	if len(messages) != 2 || messages[0].ID != "m1" || messages[1].ID != "m2" {
		t.Fatalf("messages = %+v", messages)
	}
	if fmt.Sprint(completed) != "[m1 m2]" {
		t.Fatalf("completed = %v", completed)
	}
	if stats.Total != 2 || stats.Fetched != 2 || stats.CacheHits != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestEnsureMessageCacheUsesHitsAndRefreshPolicy(t *testing.T) {
	t.Parallel()
	cache := newPlannerCache(t)
	if err := cache.WriteMessage("accthash", Message{ID: "m1", Raw: "cached"}); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	source := &fakeSource{
		messages: map[string]Message{
			"m1": {ID: "m1", Raw: "fresh-1"},
			"m2": {ID: "m2", Raw: "fresh-2"},
		},
	}
	stats, err := EnsureMessageCache(context.Background(), source, []string{"m1", "m2"}, FetchOptions{
		AccountHash: "accthash",
		Cache:       cache,
		UseCache:    true,
	})
	if err != nil {
		t.Fatalf("EnsureMessageCache: %v", err)
	}
	if stats.CacheHits != 1 || stats.Fetched != 1 || source.rawCalls.Load() != 1 {
		t.Fatalf("stats=%+v rawCalls=%d", stats, source.rawCalls.Load())
	}

	stats, err = EnsureMessageCache(context.Background(), source, []string{"m1"}, FetchOptions{
		AccountHash: "accthash",
		Cache:       cache,
		UseCache:    true,
		Refresh:     true,
	})
	if err != nil {
		t.Fatalf("EnsureMessageCache refresh: %v", err)
	}
	if stats.Fetched != 1 || source.rawCalls.Load() != 2 {
		t.Fatalf("refresh stats=%+v rawCalls=%d", stats, source.rawCalls.Load())
	}
}

func TestFetchMessagesFirstErrorCancelsOtherWorkers(t *testing.T) {
	t.Parallel()
	source := &fakeSource{
		messages: map[string]Message{
			"m2": {ID: "m2", Raw: "raw-2"},
		},
		errors: map[string]error{
			"m1": errInjectedFetch,
		},
		blockUntilCancel: map[string]bool{
			"m2": true,
		},
	}
	_, _, err := FetchMessages(context.Background(), source, []string{"m1", "m2", "m3"}, FetchOptions{Concurrency: 2})
	if !errors.Is(err, errInjectedFetch) {
		t.Fatalf("error = %v, want injected fetch", err)
	}
	if source.rawCalls.Load() > 2 {
		t.Fatalf("raw calls = %d, want bounded cancellation", source.rawCalls.Load())
	}
}

func TestFetchMessagesCancellationReturnsPromptly(t *testing.T) {
	t.Parallel()
	source := &fakeSource{
		blockUntilCancel: map[string]bool{"m1": true},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := FetchMessages(ctx, source, []string{"m1"}, FetchOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

var errInjectedFetch = errors.New("injected fetch failure")

type fakeSource struct {
	mu               sync.Mutex
	listPages        map[string]ListPage
	listRequests     []ListRequest
	messages         map[string]Message
	delays           map[string]time.Duration
	errors           map[string]error
	blockUntilCancel map[string]bool
	rawCalls         atomic.Int32
}

func (s *fakeSource) Labels(context.Context) ([]Label, error) {
	return nil, nil
}

func (s *fakeSource) ListMessageIDs(_ context.Context, req ListRequest) (ListPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listRequests = append(s.listRequests, req)
	return s.listPages[req.PageToken], nil
}

func (s *fakeSource) RawMessage(ctx context.Context, id string) (Message, error) {
	s.rawCalls.Add(1)
	if s.blockUntilCancel[id] {
		<-ctx.Done()
		return Message{}, fmt.Errorf("blocked fake source: %w", ctx.Err())
	}
	if delay := s.delays[id]; delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return Message{}, fmt.Errorf("delayed fake source: %w", ctx.Err())
		case <-timer.C:
		}
	}
	if err := s.errors[id]; err != nil {
		return Message{}, err
	}
	return s.messages[id], nil
}
