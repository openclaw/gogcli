//nolint:wsl_v5 // Worker coordination reads best with state transitions grouped.
package gmailbackup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const (
	DefaultListPageSize    = int64(500)
	DefaultFetchConcurrent = 2
	EventPhaseList         = "list"
	EventPhaseFetch        = "fetch"
)

var (
	errCacheRequired     = errors.New("gmail backup cache is required")
	errFetchStopped      = errors.New("gmail backup fetch stopped before completion")
	errMessageIDMismatch = errors.New("gmail backup source returned a different message ID")
)

type CacheStore interface {
	ReadMessage(accountHash, messageID string) (Message, bool, error)
	WriteMessage(accountHash string, msg Message) error
	ReadListState(selection Selection) (ListState, bool, error)
	WriteListState(selection Selection, ids []string, pageToken string, complete bool) error
}

type Event struct {
	Phase     string
	Resume    string
	Done      int
	Total     int
	Fetched   int
	CacheHits int
}

type ListOptions struct {
	Selection Selection
	Cache     CacheStore
	UseCache  bool
	Refresh   bool
	PageSize  int64
	Progress  func(Event)
}

type FetchOptions struct {
	AccountHash   string
	Cache         CacheStore
	UseCache      bool
	Refresh       bool
	Concurrency   int
	Progress      func(Event)
	AfterMessage  func(context.Context, string, Event) error
	ReleaseMemory func()
}

type FetchStats struct {
	Total     int
	Fetched   int
	CacheHits int
}

type fetchResult struct {
	index    int
	id       string
	message  Message
	cacheHit bool
	err      error
}

func ListMessageIDs(ctx context.Context, source Source, opts ListOptions) ([]string, error) {
	if source == nil {
		return nil, errSourceRequired
	}
	if opts.UseCache && opts.Cache == nil {
		return nil, errCacheRequired
	}
	if opts.PageSize <= 0 {
		opts.PageSize = DefaultListPageSize
	}

	ids := make([]string, 0)
	seen := make(map[string]struct{})
	pageToken := ""
	if opts.UseCache && !opts.Refresh {
		state, found, err := opts.Cache.ReadListState(opts.Selection)
		if err != nil {
			return nil, fmt.Errorf("read Gmail backup list state: %w", err)
		}
		if found {
			ids = appendUniqueIDs(ids, seen, state.IDs, opts.Selection.Max)
			if state.Complete || reachedSelectionMax(ids, opts.Selection.Max) {
				if !state.Complete {
					if err := opts.Cache.WriteListState(opts.Selection, ids, "", true); err != nil {
						return nil, fmt.Errorf("complete Gmail backup list state: %w", err)
					}
				}
				emitEvent(opts.Progress, Event{Phase: EventPhaseList, Resume: "complete", Done: len(ids)})
				return ids, nil
			}
			pageToken = state.PageToken
			emitEvent(opts.Progress, Event{Phase: EventPhaseList, Resume: "partial", Done: len(ids)})
		}
	}
	emitEvent(opts.Progress, Event{Phase: EventPhaseList, Resume: "start", Done: len(ids)})

	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("list Gmail backup messages: %w", err)
		}
		maxResults := opts.PageSize
		if opts.Selection.Max > 0 {
			remaining := opts.Selection.Max - int64(len(ids))
			if remaining <= 0 {
				break
			}
			if remaining < maxResults {
				maxResults = remaining
			}
		}
		page, err := source.ListMessageIDs(ctx, ListRequest{
			Query:            opts.Selection.Query,
			MaxResults:       maxResults,
			IncludeSpamTrash: opts.Selection.IncludeSpamTrash,
			PageToken:        pageToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list Gmail backup message IDs: %w", err)
		}
		ids = appendUniqueIDs(ids, seen, page.IDs, opts.Selection.Max)
		emitEvent(opts.Progress, Event{Phase: EventPhaseList, Done: len(ids)})

		complete := strings.TrimSpace(page.NextPageToken) == "" || reachedSelectionMax(ids, opts.Selection.Max)
		if opts.UseCache {
			nextToken := page.NextPageToken
			if complete {
				nextToken = ""
			}
			if err := opts.Cache.WriteListState(opts.Selection, ids, nextToken, complete); err != nil {
				return nil, fmt.Errorf("write Gmail backup list state: %w", err)
			}
		}
		if complete {
			break
		}
		pageToken = page.NextPageToken
	}
	return ids, nil
}

func FetchMessages(ctx context.Context, source Source, ids []string, opts FetchOptions) ([]Message, FetchStats, error) {
	messages, stats, err := runFetch(ctx, source, ids, opts, true)
	return messages, stats, err
}

func EnsureMessageCache(ctx context.Context, source Source, ids []string, opts FetchOptions) (FetchStats, error) {
	if !opts.UseCache {
		return FetchStats{}, errCacheRequired
	}
	_, stats, err := runFetch(ctx, source, ids, opts, false)
	return stats, err
}

func runFetch(ctx context.Context, source Source, ids []string, opts FetchOptions, retain bool) ([]Message, FetchStats, error) {
	if source == nil {
		return nil, FetchStats{}, errSourceRequired
	}
	if opts.UseCache && opts.Cache == nil {
		return nil, FetchStats{}, errCacheRequired
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = DefaultFetchConcurrent
	}
	ids = uniqueIDs(ids)
	stats := FetchStats{Total: len(ids)}
	emitEvent(opts.Progress, Event{Phase: EventPhaseFetch, Total: len(ids)})
	if len(ids) == 0 {
		return nil, stats, nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	results := make(chan fetchResult, opts.Concurrency)
	var workers sync.WaitGroup
	for range opts.Concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				result := fetchOne(runCtx, source, ids[index], index, opts)
				select {
				case results <- result:
				case <-runCtx.Done():
					return
				}
				if result.err != nil {
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range ids {
			select {
			case jobs <- index:
			case <-runCtx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	var messages []Message
	if retain {
		messages = make([]Message, len(ids))
	}
	pending := make(map[int]fetchResult)
	next := 0
	var firstErr error
	for result := range results {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
			cancel()
		}
		pending[result.index] = result
		for {
			ordered, found := pending[next]
			if !found {
				break
			}
			delete(pending, next)
			if ordered.err != nil {
				break
			}
			if retain {
				messages[next] = ordered.message
			}
			if ordered.cacheHit {
				stats.CacheHits++
			} else {
				stats.Fetched++
			}
			next++
			event := Event{
				Phase:     EventPhaseFetch,
				Done:      next,
				Total:     len(ids),
				Fetched:   stats.Fetched,
				CacheHits: stats.CacheHits,
			}
			if opts.AfterMessage != nil {
				if err := opts.AfterMessage(runCtx, ordered.id, event); err != nil {
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					break
				}
			}
			if next == len(ids) || next%100 == 0 {
				emitEvent(opts.Progress, event)
			}
			if next%1000 == 0 && opts.ReleaseMemory != nil {
				opts.ReleaseMemory()
			}
		}
	}
	if firstErr != nil {
		return nil, stats, firstErr
	}
	if next != len(ids) {
		if err := ctx.Err(); err != nil {
			return nil, stats, fmt.Errorf("fetch Gmail backup messages: %w", err)
		}
		return nil, stats, fmt.Errorf("%w: %d/%d", errFetchStopped, next, len(ids))
	}
	return messages, stats, nil
}

func fetchOne(ctx context.Context, source Source, id string, index int, opts FetchOptions) fetchResult {
	if opts.UseCache && !opts.Refresh {
		msg, found, err := opts.Cache.ReadMessage(opts.AccountHash, id)
		if err != nil {
			return fetchResult{index: index, id: id, err: fmt.Errorf("read Gmail backup cache %s: %w", id, err)}
		}
		if found {
			return fetchResult{index: index, id: id, message: msg, cacheHit: true}
		}
	}
	msg, err := source.RawMessage(ctx, id)
	if err != nil {
		return fetchResult{index: index, id: id, err: err}
	}
	if msg.ID != id {
		return fetchResult{index: index, id: id, err: fmt.Errorf("%w: got %s, want %s", errMessageIDMismatch, msg.ID, id)}
	}
	if opts.UseCache {
		if err := opts.Cache.WriteMessage(opts.AccountHash, msg); err != nil {
			return fetchResult{index: index, id: id, err: fmt.Errorf("write Gmail backup cache %s: %w", id, err)}
		}
	}
	return fetchResult{index: index, id: id, message: msg}
}

func appendUniqueIDs(dst []string, seen map[string]struct{}, ids []string, maxMessages int64) []string {
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, found := seen[id]; found {
			continue
		}
		seen[id] = struct{}{}
		dst = append(dst, id)
		if reachedSelectionMax(dst, maxMessages) {
			break
		}
	}
	return dst
}

func uniqueIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	return appendUniqueIDs(out, make(map[string]struct{}), ids, 0)
}

func reachedSelectionMax(ids []string, maxMessages int64) bool {
	return maxMessages > 0 && int64(len(ids)) >= maxMessages
}

func emitEvent(progress func(Event), event Event) {
	if progress != nil {
		progress(event)
	}
}
