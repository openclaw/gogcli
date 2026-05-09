package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/backup"
	"github.com/steipete/gogcli/internal/ui"
)

type gmailBackupOptions struct {
	Query            string
	Max              int64
	IncludeSpamTrash bool
	ShardMaxRows     int
	AccountHash      string
	CacheMessages    bool
	RefreshCache     bool
	Checkpoints      bool
	CheckpointRows   int
	CheckpointEvery  time.Duration
	CheckpointRunID  string
	BackupOptions    backup.Options
}

type gmailBackupMessage struct {
	ID           string   `json:"id"`
	ThreadID     string   `json:"threadId,omitempty"`
	HistoryID    string   `json:"historyId,omitempty"`
	InternalDate int64    `json:"internalDate,omitempty"`
	LabelIDs     []string `json:"labelIds,omitempty"`
	SizeEstimate int64    `json:"sizeEstimate,omitempty"`
	Raw          string   `json:"raw"`
}

type gmailBackupLabel struct {
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

type gmailBackupListState struct {
	Version          int       `json:"version"`
	AccountHash      string    `json:"accountHash"`
	Query            string    `json:"query,omitempty"`
	Max              int64     `json:"max,omitempty"`
	IncludeSpamTrash bool      `json:"includeSpamTrash"`
	PageToken        string    `json:"pageToken,omitempty"`
	IDs              []string  `json:"ids"`
	Complete         bool      `json:"complete"`
	Updated          time.Time `json:"updated"`
}

type gmailBackupFetchResult struct {
	id    string
	cache bool
	err   error
}

func buildGmailBackupSnapshot(ctx context.Context, flags *RootFlags, opts gmailBackupOptions) (backup.Snapshot, error) {
	if opts.ShardMaxRows <= 0 {
		opts.ShardMaxRows = 1000
	}
	account, err := requireAccount(flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	svc, err := newGmailService(ctx, account)
	if err != nil {
		return backup.Snapshot{}, err
	}
	accountHash := backupAccountHash(account)
	opts.AccountHash = accountHash
	labels, err := fetchGmailBackupLabels(ctx, svc)
	if err != nil {
		return backup.Snapshot{}, err
	}
	shards := make([]backup.PlainShard, 0, 1)
	labelShard, err := backup.NewJSONLShard(backupServiceGmail, "labels", accountHash, fmt.Sprintf("data/gmail/%s/labels.jsonl.gz.age", accountHash), labels)
	if err != nil {
		return backup.Snapshot{}, err
	}
	shards = append(shards, labelShard)
	var messageCount int
	if opts.CacheMessages {
		ids, listErr := listGmailBackupMessageIDs(ctx, svc, opts)
		if listErr != nil {
			return backup.Snapshot{}, listErr
		}
		opts.CheckpointRunID = gmailBackupResolvedCheckpointRunID(ctx, opts, ids)
		if cacheErr := ensureGmailBackupMessageCache(ctx, svc, opts, ids); cacheErr != nil {
			return backup.Snapshot{}, cacheErr
		}
		messageShards, promoted, shardErr := buildGmailMessageShardsFromCheckpoint(ctx, opts, ids)
		if shardErr != nil {
			return backup.Snapshot{}, shardErr
		}
		if !promoted {
			messageShards, shardErr = buildGmailMessageShardsFromCache(ctx, opts, ids)
			if shardErr != nil {
				return backup.Snapshot{}, shardErr
			}
		}
		shards = append(shards, messageShards...)
		messageCount = len(ids)
	} else {
		messages, err := fetchGmailBackupMessages(ctx, svc, opts)
		if err != nil {
			return backup.Snapshot{}, err
		}
		messageShards, err := buildGmailMessageShards(accountHash, messages, opts.ShardMaxRows)
		if err != nil {
			return backup.Snapshot{}, err
		}
		shards = append(shards, messageShards...)
		messageCount = len(messages)
	}
	return backup.Snapshot{
		Services: []string{backupServiceGmail},
		Accounts: []string{accountHash},
		Counts: map[string]int{
			"gmail.labels":   len(labels),
			"gmail.messages": messageCount,
		},
		Shards: shards,
	}, nil
}

func fetchGmailBackupLabels(ctx context.Context, svc *gmail.Service) ([]gmailBackupLabel, error) {
	resp, err := svc.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	out := make([]gmailBackupLabel, 0, len(resp.Labels))
	for _, label := range resp.Labels {
		if label == nil {
			continue
		}
		out = append(out, gmailBackupLabel{
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

func fetchGmailBackupMessages(ctx context.Context, svc *gmail.Service, opts gmailBackupOptions) ([]gmailBackupMessage, error) {
	ids, err := listGmailBackupMessageIDs(ctx, svc, opts)
	if err != nil {
		return nil, err
	}
	if !opts.CacheMessages {
		return fetchGmailBackupMessagesDirect(ctx, svc, ids)
	}
	if err := ensureGmailBackupMessageCache(ctx, svc, opts, ids); err != nil {
		return nil, err
	}
	ordered := make([]gmailBackupMessage, 0, len(ids))
	for _, id := range ids {
		msg, ok, err := readGmailBackupMessageCache(opts.AccountHash, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("gmail message %s missing from backup cache", id)
		}
		ordered = append(ordered, msg)
	}
	return ordered, nil
}

func fetchGmailBackupMessagesDirect(ctx context.Context, svc *gmail.Service, ids []string) ([]gmailBackupMessage, error) {
	gmailBackupProgressf(ctx, "backup gmail fetch\tqueued=%d", len(ids))
	out := make([]gmailBackupMessage, 0, len(ids))
	for i, id := range ids {
		msg, err := svc.Users.Messages.Get("me", id).
			Format(gmailFormatRaw).
			Fields("id,threadId,historyId,internalDate,labelIds,sizeEstimate,raw").
			Context(ctx).
			Do()
		if err != nil {
			return nil, fmt.Errorf("gmail message %s: %w", id, err)
		}
		if strings.TrimSpace(msg.Raw) == "" {
			return nil, fmt.Errorf("gmail message %s returned empty raw payload", id)
		}
		out = append(out, gmailBackupMessage{
			ID:           msg.Id,
			ThreadID:     msg.ThreadId,
			HistoryID:    formatHistoryID(msg.HistoryId),
			InternalDate: msg.InternalDate,
			LabelIDs:     append([]string(nil), msg.LabelIds...),
			SizeEstimate: msg.SizeEstimate,
			Raw:          msg.Raw,
		})
		done := i + 1
		if done == len(ids) || done%100 == 0 {
			gmailBackupProgressf(ctx, "backup gmail fetch\t%d/%d\tfetched=%d\tcache=0", done, len(ids), done)
		}
	}
	return out, nil
}

func ensureGmailBackupMessageCache(ctx context.Context, svc *gmail.Service, opts gmailBackupOptions, ids []string) error {
	gmailBackupProgressf(ctx, "backup gmail fetch\tqueued=%d", len(ids))
	checkpointer := newGmailBackupCheckpointer(ctx, opts, len(ids))
	const maxConcurrency = 2
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan string)
	results := make(chan gmailBackupFetchResult, maxConcurrency)
	var wg sync.WaitGroup
	for range maxConcurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case messageID, ok := <-jobs:
					if !ok {
						return
					}
					results <- fetchGmailBackupMessageCacheResult(ctx, svc, opts, messageID)
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, id := range ids {
			select {
			case <-ctx.Done():
				return
			case jobs <- id:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	var firstErr error
	done := 0
	cacheHits := 0
	fetched := 0
	for res := range results {
		if res.err != nil {
			if firstErr == nil {
				firstErr = res.err
				cancel()
			}
			continue
		}
		done++
		if res.cache {
			cacheHits++
		} else if res.err == nil {
			fetched++
		}
		if err := checkpointer.record(ctx, res.id, done, fetched, cacheHits); err != nil {
			firstErr = err
			cancel()
			continue
		}
		if done == len(ids) || done%100 == 0 {
			gmailBackupProgressf(ctx, "backup gmail fetch\t%d/%d\tfetched=%d\tcache=%d", done, len(ids), fetched, cacheHits)
		}
		if done%1000 == 0 {
			debug.FreeOSMemory()
		}
	}
	if firstErr != nil {
		return firstErr
	}
	if done != len(ids) {
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("gmail backup fetch stopped after %d/%d messages", done, len(ids))
	}
	return checkpointer.flush(ctx, done, fetched, cacheHits)
}

func fetchGmailBackupMessageCacheResult(ctx context.Context, svc *gmail.Service, opts gmailBackupOptions, messageID string) gmailBackupFetchResult {
	if opts.CacheMessages && !opts.RefreshCache {
		_, ok, err := readGmailBackupMessageCache(opts.AccountHash, messageID)
		if err != nil {
			return gmailBackupFetchResult{id: messageID, err: err}
		}
		if ok {
			return gmailBackupFetchResult{id: messageID, cache: true}
		}
	}
	msg, err := svc.Users.Messages.Get("me", messageID).
		Format(gmailFormatRaw).
		Fields("id,threadId,historyId,internalDate,labelIds,sizeEstimate,raw").
		Context(ctx).
		Do()
	if err != nil {
		return gmailBackupFetchResult{id: messageID, err: fmt.Errorf("gmail message %s: %w", messageID, err)}
	}
	if strings.TrimSpace(msg.Raw) == "" {
		return gmailBackupFetchResult{id: messageID, err: fmt.Errorf("gmail message %s returned empty raw payload", messageID)}
	}
	backupMsg := gmailBackupMessage{
		ID:           msg.Id,
		ThreadID:     msg.ThreadId,
		HistoryID:    formatHistoryID(msg.HistoryId),
		InternalDate: msg.InternalDate,
		LabelIDs:     append([]string(nil), msg.LabelIds...),
		SizeEstimate: msg.SizeEstimate,
		Raw:          msg.Raw,
	}
	if opts.CacheMessages {
		if err := writeGmailBackupMessageCache(opts.AccountHash, backupMsg); err != nil {
			return gmailBackupFetchResult{id: messageID, err: err}
		}
	}
	return gmailBackupFetchResult{id: messageID}
}

type gmailBackupCheckpointer struct {
	enabled bool
	opts    gmailBackupOptions
	total   int
	part    int
	last    time.Time
	pending []string
}

const gmailCheckpointShardMaxRows = 250

var (
	gmailCheckpointShardMaxPlaintextBytes int64 = 32 * 1024 * 1024
	gmailMessageShardMaxPlaintextBytes    int64 = 32 * 1024 * 1024
)

func newGmailBackupCheckpointer(ctx context.Context, opts gmailBackupOptions, total int) *gmailBackupCheckpointer {
	enabled := opts.Checkpoints &&
		opts.CacheMessages &&
		strings.TrimSpace(opts.AccountHash) != "" &&
		strings.TrimSpace(opts.CheckpointRunID) != "" &&
		(opts.CheckpointRows > 0 || opts.CheckpointEvery > 0)
	cp := &gmailBackupCheckpointer{
		enabled: enabled,
		opts:    opts,
		total:   total,
		last:    time.Now(),
	}
	if enabled {
		gmailBackupProgressf(ctx, "backup gmail checkpoint\trun=%s\trows=%d\tinterval=%s", opts.CheckpointRunID, opts.CheckpointRows, opts.CheckpointEvery)
	}
	return cp
}

func (c *gmailBackupCheckpointer) record(ctx context.Context, messageID string, done, fetched, cacheHits int) error {
	if c == nil || !c.enabled || strings.TrimSpace(messageID) == "" {
		return nil
	}
	c.pending = append(c.pending, messageID)
	if c.shouldFlush(done) {
		return c.flush(ctx, done, fetched, cacheHits)
	}
	return nil
}

func (c *gmailBackupCheckpointer) shouldFlush(done int) bool {
	if len(c.pending) == 0 {
		return false
	}
	if c.opts.CheckpointRows > 0 && len(c.pending) >= c.opts.CheckpointRows {
		return true
	}
	if c.opts.CheckpointEvery > 0 && time.Since(c.last) >= c.opts.CheckpointEvery {
		return true
	}
	return done == c.total
}

func (c *gmailBackupCheckpointer) flush(ctx context.Context, done, fetched, cacheHits int) error {
	if c == nil || !c.enabled || len(c.pending) == 0 {
		return nil
	}
	c.part++
	ids := append([]string(nil), c.pending...)
	c.pending = c.pending[:0]
	shards, err := buildGmailCheckpointShardsFromCache(c.opts.AccountHash, c.opts.CheckpointRunID, c.part, ids)
	if err != nil {
		return err
	}
	c.part += len(shards) - 1
	snapshot := backup.Snapshot{
		Services: []string{backupServiceGmail},
		Accounts: []string{c.opts.AccountHash},
		Counts:   map[string]int{"gmail.messages": len(ids)},
		Shards:   shards,
	}
	result, err := backup.PushCheckpoint(ctx, snapshot, backup.Checkpoint{
		RunID:     c.opts.CheckpointRunID,
		Service:   backupServiceGmail,
		Account:   c.opts.AccountHash,
		Done:      done,
		Total:     c.total,
		Fetched:   fetched,
		CacheHits: cacheHits,
	}, c.opts.BackupOptions)
	if err != nil {
		return err
	}
	c.last = time.Now()
	gmailBackupProgressf(ctx, "backup gmail checkpoint\t%d/%d\tparts=%d\trows=%d\tchanged=%t", done, c.total, len(shards), len(ids), result.Changed)
	return nil
}

func readGmailBackupMessageCache(accountHash, messageID string) (gmailBackupMessage, bool, error) {
	path, ok := gmailBackupMessageCachePath(accountHash, messageID)
	if !ok {
		return gmailBackupMessage{}, false, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // cache path is derived from the OS cache dir, account hash, and hashed message ID.
	if err != nil {
		if os.IsNotExist(err) {
			return gmailBackupMessage{}, false, nil
		}
		return gmailBackupMessage{}, false, fmt.Errorf("read gmail backup cache %s: %w", path, err)
	}
	var msg gmailBackupMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return gmailBackupMessage{}, false, fmt.Errorf("decode gmail backup cache %s: %w", path, err)
	}
	if msg.ID != messageID {
		return gmailBackupMessage{}, false, fmt.Errorf("gmail backup cache %s has id %q, want %q", path, msg.ID, messageID)
	}
	if strings.TrimSpace(msg.Raw) == "" {
		return gmailBackupMessage{}, false, fmt.Errorf("gmail backup cache %s has empty raw payload", path)
	}
	return msg, true, nil
}

func writeGmailBackupMessageCache(accountHash string, msg gmailBackupMessage) error {
	if strings.TrimSpace(msg.ID) == "" {
		return nil
	}
	path, ok := gmailBackupMessageCachePath(accountHash, msg.ID)
	if !ok {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create gmail backup cache dir: %w", err)
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode gmail backup cache %s: %w", msg.ID, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".message-*.json")
	if err != nil {
		return fmt.Errorf("create gmail backup cache temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write gmail backup cache temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close gmail backup cache temp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod gmail backup cache temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace gmail backup cache %s: %w", path, err)
	}
	return nil
}

func gmailBackupMessageCachePath(accountHash, messageID string) (string, bool) {
	accountHash = strings.TrimSpace(accountHash)
	messageID = strings.TrimSpace(messageID)
	if accountHash == "" || messageID == "" {
		return "", false
	}
	dir, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(messageID))
	name := hex.EncodeToString(sum[:]) + ".json"
	return filepath.Join(dir, "gogcli", "backup", "gmail", accountHash, "raw-v1", name), true
}

func listGmailBackupMessageIDs(ctx context.Context, svc *gmail.Service, opts gmailBackupOptions) ([]string, error) {
	var ids []string
	pageToken := ""
	statePath, hasStatePath := gmailBackupListStatePath(opts)
	if opts.CacheMessages && !opts.RefreshCache && hasStatePath {
		state, ok, err := readGmailBackupListState(statePath)
		if err != nil {
			return nil, err
		}
		if ok {
			if state.Complete {
				gmailBackupProgressf(ctx, "backup gmail list\tresume=complete\tmessages=%d", len(state.IDs))
				return append([]string(nil), state.IDs...), nil
			}
			ids = append(ids, state.IDs...)
			pageToken = state.PageToken
			gmailBackupProgressf(ctx, "backup gmail list\tresume=partial\tmessages=%d", len(ids))
		}
	}
	gmailBackupProgressf(ctx, "backup gmail list\tstart\tmessages=%d", len(ids))
	for {
		maxResults := int64(500)
		if opts.Max > 0 {
			remaining := opts.Max - int64(len(ids))
			if remaining <= 0 {
				break
			}
			if remaining < maxResults {
				maxResults = remaining
			}
		}
		call := svc.Users.Messages.List("me").
			MaxResults(maxResults).
			IncludeSpamTrash(opts.IncludeSpamTrash).
			Fields("messages(id),nextPageToken").
			Context(ctx)
		if strings.TrimSpace(opts.Query) != "" {
			call = call.Q(opts.Query)
		}
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		for _, message := range resp.Messages {
			if message != nil && strings.TrimSpace(message.Id) != "" {
				ids = append(ids, message.Id)
			}
		}
		gmailBackupProgressf(ctx, "backup gmail list\tmessages=%d", len(ids))
		complete := resp.NextPageToken == "" || (opts.Max > 0 && int64(len(ids)) >= opts.Max)
		if complete {
			if opts.CacheMessages && hasStatePath {
				if err := writeGmailBackupListState(statePath, opts, ids, "", true); err != nil {
					return nil, err
				}
			}
			break
		}
		pageToken = resp.NextPageToken
		if opts.CacheMessages && hasStatePath {
			if err := writeGmailBackupListState(statePath, opts, ids, pageToken, false); err != nil {
				return nil, err
			}
		}
	}
	return ids, nil
}

func readGmailBackupListState(path string) (gmailBackupListState, bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the OS cache dir and query hash.
	if err != nil {
		if os.IsNotExist(err) {
			return gmailBackupListState{}, false, nil
		}
		return gmailBackupListState{}, false, fmt.Errorf("read gmail backup list state %s: %w", path, err)
	}
	var state gmailBackupListState
	if err := json.Unmarshal(data, &state); err != nil {
		return gmailBackupListState{}, false, fmt.Errorf("decode gmail backup list state %s: %w", path, err)
	}
	if state.Version != 1 {
		return gmailBackupListState{}, false, nil
	}
	return state, true, nil
}

func writeGmailBackupListState(path string, opts gmailBackupOptions, ids []string, pageToken string, complete bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create gmail backup list state dir: %w", err)
	}
	state := gmailBackupListState{
		Version:          1,
		AccountHash:      opts.AccountHash,
		Query:            strings.TrimSpace(opts.Query),
		Max:              opts.Max,
		IncludeSpamTrash: opts.IncludeSpamTrash,
		PageToken:        pageToken,
		IDs:              append([]string(nil), ids...),
		Complete:         complete,
		Updated:          time.Now().UTC(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode gmail backup list state: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".list-*.json")
	if err != nil {
		return fmt.Errorf("create gmail backup list state temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write gmail backup list state temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close gmail backup list state temp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod gmail backup list state temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace gmail backup list state %s: %w", path, err)
	}
	return nil
}

func gmailBackupListStatePath(opts gmailBackupOptions) (string, bool) {
	accountHash := strings.TrimSpace(opts.AccountHash)
	if accountHash == "" {
		return "", false
	}
	dir, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return "", false
	}
	key := struct {
		Query            string `json:"query,omitempty"`
		Max              int64  `json:"max,omitempty"`
		IncludeSpamTrash bool   `json:"includeSpamTrash"`
	}{
		Query:            strings.TrimSpace(opts.Query),
		Max:              opts.Max,
		IncludeSpamTrash: opts.IncludeSpamTrash,
	}
	data, err := json.Marshal(key)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + ".json"
	return filepath.Join(dir, "gogcli", "backup", "gmail", accountHash, "list-v1", name), true
}

func gmailBackupProgressf(ctx context.Context, format string, args ...any) {
	u := ui.FromContext(ctx)
	if u == nil {
		return
	}
	u.Err().Printf(format, args...)
}

type gmailBackupMessageRef struct {
	ID           string
	InternalDate int64
	LineBytes    int64
}

func buildGmailMessageShardsFromCache(ctx context.Context, opts gmailBackupOptions, ids []string) ([]backup.PlainShard, error) {
	return buildGmailMessageShardsFromCacheWithLimit(ctx, opts, ids, gmailMessageShardMaxPlaintextBytes)
}

func buildGmailMessageShardsFromCacheWithLimit(ctx context.Context, opts gmailBackupOptions, ids []string, maxPlaintextBytes int64) ([]backup.PlainShard, error) {
	if opts.ShardMaxRows <= 0 {
		opts.ShardMaxRows = 1000
	}
	tempDir, ok := gmailBackupTempShardDir(opts.AccountHash)
	if !ok {
		return nil, fmt.Errorf("gmail backup temp shard directory unavailable")
	}
	if err := os.RemoveAll(tempDir); err != nil {
		return nil, fmt.Errorf("clear gmail backup temp shard dir: %w", err)
	}
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return nil, fmt.Errorf("create gmail backup temp shard dir: %w", err)
	}
	buckets := map[string][]gmailBackupMessageRef{}
	for i, id := range ids {
		msg, ok, err := readGmailBackupMessageCache(opts.AccountHash, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("gmail message %s missing from backup cache", id)
		}
		lineBytes, err := gmailBackupMessageJSONLSize(msg)
		if err != nil {
			return nil, err
		}
		key := gmailBackupMessageMonthKey(msg.InternalDate)
		buckets[key] = append(buckets[key], gmailBackupMessageRef{ID: msg.ID, InternalDate: msg.InternalDate, LineBytes: lineBytes})
		done := i + 1
		if done == len(ids) || done%5000 == 0 {
			gmailBackupProgressf(ctx, "backup gmail shard-index\t%d/%d", done, len(ids))
		}
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	shards := make([]backup.PlainShard, 0, len(keys))
	shardCount := 0
	for _, key := range keys {
		refs := buckets[key]
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].InternalDate == refs[j].InternalDate {
				return refs[i].ID < refs[j].ID
			}
			return refs[i].InternalDate < refs[j].InternalDate
		})
		for part, start := 1, 0; start < len(refs); part++ {
			chunkStart := start
			var chunkBytes int64
			for start < len(refs) {
				lineBytes := refs[start].LineBytes
				overRows := start-chunkStart >= opts.ShardMaxRows
				overBytes := maxPlaintextBytes > 0 && start > chunkStart && chunkBytes+lineBytes > maxPlaintextBytes
				if overRows || overBytes {
					break
				}
				chunkBytes += lineBytes
				start++
			}
			rel := fmt.Sprintf("data/gmail/%s/messages/%s/part-%04d.jsonl.gz.age", opts.AccountHash, key, part)
			shard, err := buildGmailMessageShardFromCache(opts.AccountHash, rel, tempDir, refs[chunkStart:start])
			if err != nil {
				return nil, err
			}
			shards = append(shards, shard)
			shardCount++
			if shardCount%25 == 0 || start == len(refs) {
				gmailBackupProgressf(ctx, "backup gmail shard-build\tshards=%d\tmessages=%d/%d", shardCount, countGmailShardRows(shards), len(ids))
			}
		}
	}
	return shards, nil
}

func buildGmailMessageShardFromCache(accountHash, rel, tempDir string, refs []gmailBackupMessageRef) (backup.PlainShard, error) {
	sum := sha256.Sum256([]byte(rel))
	path := filepath.Join(tempDir, hex.EncodeToString(sum[:])+".jsonl")
	f, err := os.Create(path) //nolint:gosec // path is derived from the OS cache dir and a hash of the shard path.
	if err != nil {
		return backup.PlainShard{}, fmt.Errorf("create gmail backup temp shard: %w", err)
	}
	enc := json.NewEncoder(f)
	count := 0
	for _, ref := range refs {
		msg, ok, err := readGmailBackupMessageCache(accountHash, ref.ID)
		if err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return backup.PlainShard{}, err
		}
		if !ok {
			_ = f.Close()
			_ = os.Remove(path)
			return backup.PlainShard{}, fmt.Errorf("gmail message %s missing from backup cache", ref.ID)
		}
		if err := enc.Encode(msg); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return backup.PlainShard{}, fmt.Errorf("encode gmail backup temp shard: %w", err)
		}
		count++
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return backup.PlainShard{}, fmt.Errorf("close gmail backup temp shard: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = os.Remove(path)
		return backup.PlainShard{}, fmt.Errorf("chmod gmail backup temp shard: %w", err)
	}
	return backup.PlainShard{
		Service:       backupServiceGmail,
		Kind:          "messages",
		Account:       accountHash,
		Path:          filepath.ToSlash(rel),
		Rows:          count,
		PlaintextPath: path,
	}, nil
}

func countGmailShardRows(shards []backup.PlainShard) int {
	total := 0
	for _, shard := range shards {
		total += shard.Rows
	}
	return total
}

func gmailBackupTempShardDir(accountHash string) (string, bool) {
	accountHash = strings.TrimSpace(accountHash)
	if accountHash == "" {
		return "", false
	}
	dir, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return "", false
	}
	return filepath.Join(dir, "gogcli", "backup", "gmail", accountHash, "tmp-shards"), true
}

func buildGmailCheckpointShardFromCache(accountHash, runID string, part int, ids []string) (backup.PlainShard, error) {
	if part <= 0 {
		return backup.PlainShard{}, fmt.Errorf("gmail checkpoint part must be positive")
	}
	tempDir, ok := gmailBackupCheckpointTempShardDir(accountHash, runID)
	if !ok {
		return backup.PlainShard{}, fmt.Errorf("gmail backup checkpoint temp shard directory unavailable")
	}
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return backup.PlainShard{}, fmt.Errorf("create gmail backup checkpoint temp shard dir: %w", err)
	}
	rel := fmt.Sprintf("checkpoints/gmail/%s/%s/messages/part-%06d.jsonl.gz.age", accountHash, runID, part)
	sum := sha256.Sum256([]byte(rel))
	path := filepath.Join(tempDir, hex.EncodeToString(sum[:])+".jsonl")
	f, err := os.Create(path) //nolint:gosec // path is derived from the OS cache dir and a hash of the checkpoint path.
	if err != nil {
		return backup.PlainShard{}, fmt.Errorf("create gmail backup checkpoint temp shard: %w", err)
	}
	enc := json.NewEncoder(f)
	count := 0
	for _, id := range ids {
		msg, ok, err := readGmailBackupMessageCache(accountHash, id)
		if err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return backup.PlainShard{}, err
		}
		if !ok {
			_ = f.Close()
			_ = os.Remove(path)
			return backup.PlainShard{}, fmt.Errorf("gmail message %s missing from backup cache", id)
		}
		if err := enc.Encode(msg); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return backup.PlainShard{}, fmt.Errorf("encode gmail backup checkpoint shard: %w", err)
		}
		count++
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return backup.PlainShard{}, fmt.Errorf("close gmail backup checkpoint shard: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = os.Remove(path)
		return backup.PlainShard{}, fmt.Errorf("chmod gmail backup checkpoint shard: %w", err)
	}
	return backup.PlainShard{
		Service:       backupServiceGmail,
		Kind:          "messages",
		Account:       accountHash,
		Path:          filepath.ToSlash(rel),
		Rows:          count,
		PlaintextPath: path,
	}, nil
}

func buildGmailCheckpointShardsFromCache(accountHash, runID string, firstPart int, ids []string) ([]backup.PlainShard, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	shards := make([]backup.PlainShard, 0, (len(ids)+gmailCheckpointShardMaxRows-1)/gmailCheckpointShardMaxRows)
	chunk := make([]string, 0, gmailCheckpointShardMaxRows)
	var chunkBytes int64
	cleanup := func() {
		for _, shard := range shards {
			if strings.TrimSpace(shard.PlaintextPath) != "" {
				_ = os.Remove(shard.PlaintextPath)
			}
		}
	}
	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		shard, err := buildGmailCheckpointShardFromCache(accountHash, runID, firstPart+len(shards), chunk)
		if err != nil {
			cleanup()
			return err
		}
		shards = append(shards, shard)
		chunk = chunk[:0]
		chunkBytes = 0
		return nil
	}
	for _, id := range ids {
		msg, ok, err := readGmailBackupMessageCache(accountHash, id)
		if err != nil {
			cleanup()
			return nil, err
		}
		if !ok {
			cleanup()
			return nil, fmt.Errorf("gmail message %s missing from backup cache", id)
		}
		lineBytes, err := gmailBackupMessageJSONLSize(msg)
		if err != nil {
			cleanup()
			return nil, err
		}
		overRows := len(chunk) >= gmailCheckpointShardMaxRows
		overBytes := gmailCheckpointShardMaxPlaintextBytes > 0 && len(chunk) > 0 && chunkBytes+lineBytes > gmailCheckpointShardMaxPlaintextBytes
		if overRows || overBytes {
			if err := flush(); err != nil {
				return nil, err
			}
		}
		chunk = append(chunk, id)
		chunkBytes += lineBytes
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return shards, nil
}

func buildGmailMessageShardsFromCheckpoint(ctx context.Context, opts gmailBackupOptions, ids []string) ([]backup.PlainShard, bool, error) {
	if !opts.CacheMessages || !opts.Checkpoints || strings.TrimSpace(opts.AccountHash) == "" || strings.TrimSpace(opts.CheckpointRunID) == "" {
		return nil, false, nil
	}
	cfg, err := backup.ResolveOptions(opts.BackupOptions)
	if err != nil {
		return nil, false, err
	}
	if len(cfg.Recipients) == 0 {
		recipient, recipientErr := backup.RecipientFromIdentity(cfg.Identity)
		if recipientErr != nil {
			return nil, false, recipientErr
		}
		cfg.Recipients = []string{recipient}
	}
	manifest, err := backup.ReadCheckpointManifest(cfg.Repo, gmailBackupCheckpointManifestRel(opts.AccountHash, opts.CheckpointRunID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !gmailBackupCheckpointCompleteForSelection(manifest, opts, ids) {
		return nil, false, nil
	}
	if !sameBackupRecipients(manifest.Recipients, cfg.Recipients) {
		gmailBackupProgressf(ctx, "backup gmail checkpoint-promote\tskip=recipients-changed\trun=%s", opts.CheckpointRunID)
		return nil, false, nil
	}
	shards := make([]backup.PlainShard, 0, len(manifest.Shards))
	rows := 0
	for _, entry := range manifest.Shards {
		if entry.Service != backupServiceGmail || entry.Kind != "messages" || entry.Account != opts.AccountHash {
			return nil, false, fmt.Errorf("gmail checkpoint %s contains unexpected shard %s/%s/%s", opts.CheckpointRunID, entry.Service, entry.Kind, entry.Account)
		}
		shards = append(shards, backup.ExistingShard(entry, manifest.Recipients))
		rows += entry.Rows
	}
	if rows != len(ids) {
		return nil, false, fmt.Errorf("gmail checkpoint %s row count = %d, want %d", opts.CheckpointRunID, rows, len(ids))
	}
	gmailBackupProgressf(ctx, "backup gmail checkpoint-promote\trun=%s\tshards=%d\tmessages=%d", opts.CheckpointRunID, len(shards), rows)
	return shards, true, nil
}

func gmailBackupCheckpointTempShardDir(accountHash, runID string) (string, bool) {
	accountHash = strings.TrimSpace(accountHash)
	runID = strings.TrimSpace(runID)
	if accountHash == "" || runID == "" {
		return "", false
	}
	dir, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return "", false
	}
	return filepath.Join(dir, "gogcli", "backup", "gmail", accountHash, "checkpoint-shards", runID), true
}

func gmailBackupCheckpointRunID(opts gmailBackupOptions, ids []string) string {
	return time.Now().UTC().Format("20060102T150405Z") + "-" + gmailBackupCheckpointRunIDSuffix(opts, ids)
}

func gmailBackupCheckpointRunIDSuffix(opts gmailBackupOptions, ids []string) string {
	key := struct {
		AccountHash      string `json:"accountHash"`
		Query            string `json:"query,omitempty"`
		Max              int64  `json:"max,omitempty"`
		IncludeSpamTrash bool   `json:"includeSpamTrash"`
		IDs              int    `json:"ids"`
	}{
		AccountHash:      opts.AccountHash,
		Query:            strings.TrimSpace(opts.Query),
		Max:              opts.Max,
		IncludeSpamTrash: opts.IncludeSpamTrash,
		IDs:              len(ids),
	}
	data, _ := json.Marshal(key)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:6])
}

func gmailBackupResolvedCheckpointRunID(ctx context.Context, opts gmailBackupOptions, ids []string) string {
	generated := gmailBackupCheckpointRunID(opts, ids)
	if !opts.Checkpoints || !opts.CacheMessages || strings.TrimSpace(opts.AccountHash) == "" {
		return generated
	}
	suffix := gmailBackupCheckpointRunIDSuffix(opts, ids)
	cfg, err := backup.ResolveOptions(opts.BackupOptions)
	if err != nil {
		return generated
	}
	root := filepath.Join(cfg.Repo, "checkpoints", "gmail", opts.AccountHash)
	entries, err := os.ReadDir(root)
	if err != nil {
		return generated
	}
	runIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), "-"+suffix) {
			runIDs = append(runIDs, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(runIDs)))
	for _, runID := range runIDs {
		manifest, err := backup.ReadCheckpointManifest(cfg.Repo, gmailBackupCheckpointManifestRel(opts.AccountHash, runID))
		if err != nil {
			continue
		}
		if !gmailBackupCheckpointMatchesSelection(manifest, opts, ids) {
			continue
		}
		gmailBackupProgressf(ctx, "backup gmail checkpoint\treuse=%s\tdone=%d/%d", runID, manifest.Done, manifest.Total)
		return runID
	}
	return generated
}

func gmailBackupCheckpointManifestRel(accountHash, runID string) string {
	return fmt.Sprintf("checkpoints/gmail/%s/%s/manifest.json", accountHash, runID)
}

func gmailBackupCheckpointMatchesSelection(manifest backup.CheckpointManifest, opts gmailBackupOptions, ids []string) bool {
	return manifest.Service == backupServiceGmail &&
		manifest.Account == opts.AccountHash &&
		manifest.Total == len(ids) &&
		strings.TrimSpace(manifest.RunID) != ""
}

func gmailBackupCheckpointCompleteForSelection(manifest backup.CheckpointManifest, opts gmailBackupOptions, ids []string) bool {
	return gmailBackupCheckpointMatchesSelection(manifest, opts, ids) &&
		manifest.Done == len(ids) &&
		manifest.Total == len(ids)
}

func sameBackupRecipients(a, b []string) bool {
	a = normalizedBackupStrings(a)
	b = normalizedBackupStrings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizedBackupStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func gmailBackupMessageMonthKey(internalDate int64) string {
	t := time.UnixMilli(internalDate).UTC()
	if internalDate <= 0 {
		t = time.Unix(0, 0).UTC()
	}
	return fmt.Sprintf("%04d/%02d", t.Year(), int(t.Month()))
}

func buildGmailMessageShards(accountHash string, messages []gmailBackupMessage, shardMaxRows int) ([]backup.PlainShard, error) {
	return buildGmailMessageShardsWithLimit(accountHash, messages, shardMaxRows, gmailMessageShardMaxPlaintextBytes)
}

func buildGmailMessageShardsWithLimit(accountHash string, messages []gmailBackupMessage, shardMaxRows int, maxPlaintextBytes int64) ([]backup.PlainShard, error) {
	if shardMaxRows <= 0 {
		shardMaxRows = 1000
	}
	buckets := map[string][]gmailBackupMessage{}
	for _, message := range messages {
		key := gmailBackupMessageMonthKey(message.InternalDate)
		buckets[key] = append(buckets[key], message)
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	shards := make([]backup.PlainShard, 0, len(keys))
	for _, key := range keys {
		values := buckets[key]
		sort.Slice(values, func(i, j int) bool {
			if values[i].InternalDate == values[j].InternalDate {
				return values[i].ID < values[j].ID
			}
			return values[i].InternalDate < values[j].InternalDate
		})
		for part, start := 1, 0; start < len(values); part++ {
			chunkStart := start
			var chunkBytes int64
			for start < len(values) {
				lineBytes, err := gmailBackupMessageJSONLSize(values[start])
				if err != nil {
					return nil, err
				}
				overRows := start-chunkStart >= shardMaxRows
				overBytes := maxPlaintextBytes > 0 && start > chunkStart && chunkBytes+lineBytes > maxPlaintextBytes
				if overRows || overBytes {
					break
				}
				chunkBytes += lineBytes
				start++
			}
			rel := fmt.Sprintf("data/gmail/%s/messages/%s/part-%04d.jsonl.gz.age", accountHash, key, part)
			shard, err := backup.NewJSONLShard(backupServiceGmail, "messages", accountHash, rel, values[chunkStart:start])
			if err != nil {
				return nil, err
			}
			shards = append(shards, shard)
		}
	}
	return shards, nil
}

func gmailBackupMessageJSONLSize(message gmailBackupMessage) (int64, error) {
	line, err := json.Marshal(message)
	if err != nil {
		return 0, fmt.Errorf("encode gmail backup shard estimate: %w", err)
	}
	return int64(len(line) + 1), nil
}
