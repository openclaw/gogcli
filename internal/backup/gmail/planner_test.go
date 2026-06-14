//nolint:wsl_v5 // Table setup and assertions stay grouped for scanability.
package gmailbackup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/backup"
)

func TestBuildMessageShardsFromMessagesBucketsSortsAndChunks(t *testing.T) {
	t.Parallel()
	messages := []Message{
		{ID: "march-new", InternalDate: mustUnixMilli(t, "2026-03-02T10:00:00Z"), Raw: "raw-3"},
		{ID: "april-later", InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"), Raw: "raw-2"},
		{ID: "april-earlier-b", InternalDate: mustUnixMilli(t, "2026-04-01T10:00:00Z"), Raw: "raw-1b"},
		{ID: "april-earlier-a", InternalDate: mustUnixMilli(t, "2026-04-01T10:00:00Z"), Raw: "raw-1a"},
		{ID: "epoch", InternalDate: 0, Raw: "raw-0"},
	}

	shards, err := BuildMessageShardsFromMessages(context.Background(), messages, ShardOptions{
		AccountHash: "accthash",
		MaxRows:     2,
	})
	if err != nil {
		t.Fatalf("BuildMessageShardsFromMessages: %v", err)
	}
	wantPaths := []string{
		"data/gmail/accthash/messages/1970/01/part-0001.jsonl.gz.age",
		"data/gmail/accthash/messages/2026/03/part-0001.jsonl.gz.age",
		"data/gmail/accthash/messages/2026/04/part-0001.jsonl.gz.age",
		"data/gmail/accthash/messages/2026/04/part-0002.jsonl.gz.age",
	}
	if len(shards) != len(wantPaths) {
		t.Fatalf("len(shards) = %d, want %d", len(shards), len(wantPaths))
	}
	for i, want := range wantPaths {
		if shards[i].Path != want {
			t.Fatalf("shards[%d].Path = %q, want %q", i, shards[i].Path, want)
		}
	}

	var aprilFirst []Message
	if err := backup.DecodeJSONL(shards[2].Plaintext, &aprilFirst); err != nil {
		t.Fatalf("DecodeJSONL: %v", err)
	}
	if aprilFirst[0].ID != "april-earlier-a" || aprilFirst[1].ID != "april-earlier-b" {
		t.Fatalf("April IDs = %q,%q", aprilFirst[0].ID, aprilFirst[1].ID)
	}
}

func TestBuildMessageShardsFromMessagesSplitsByPlaintextSize(t *testing.T) {
	t.Parallel()
	messages := []Message{
		{ID: "m1", InternalDate: mustUnixMilli(t, "2026-04-01T10:00:00Z"), Raw: strings.Repeat("raw-1", 8)},
		{ID: "m2", InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"), Raw: strings.Repeat("raw-2", 8)},
		{ID: "m3", InternalDate: mustUnixMilli(t, "2026-04-03T10:00:00Z"), Raw: strings.Repeat("raw-3", 8)},
	}

	shards, err := BuildMessageShardsFromMessages(context.Background(), messages, ShardOptions{
		AccountHash:      "accthash",
		MaxRows:          100,
		MaxPlaintextSize: 1,
	})
	if err != nil {
		t.Fatalf("BuildMessageShardsFromMessages: %v", err)
	}
	if len(shards) != 3 {
		t.Fatalf("len(shards) = %d, want 3", len(shards))
	}
	for i, shard := range shards {
		if shard.Rows != 1 {
			t.Fatalf("shards[%d].Rows = %d, want 1", i, shard.Rows)
		}
	}
}

func TestBuildMessageShardsWritesPrivatePlaintextAndProgress(t *testing.T) {
	t.Parallel()
	cache := newPlannerCache(t)
	accountHash := "accthash"
	messages := []Message{
		{ID: "april-b", InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"), Raw: "raw-b"},
		{ID: "april-a", InternalDate: mustUnixMilli(t, "2026-04-01T10:00:00Z"), Raw: "raw-a"},
		{ID: "march-a", InternalDate: mustUnixMilli(t, "2026-03-01T10:00:00Z"), Raw: "raw-m"},
	}
	for _, message := range messages {
		if err := cache.WriteMessage(accountHash, message); err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
	}
	var events []ShardEvent
	shards, err := BuildMessageShards(context.Background(), cache, []string{"april-b", "april-a", "march-a"}, ShardOptions{
		AccountHash: "accthash",
		MaxRows:     1,
		Progress:    func(event ShardEvent) { events = append(events, event) },
	})
	if err != nil {
		t.Fatalf("BuildMessageShards: %v", err)
	}
	if len(shards) != 3 {
		t.Fatalf("len(shards) = %d, want 3", len(shards))
	}
	for _, shard := range shards {
		info, err := os.Stat(shard.PlaintextPath)
		if err != nil {
			t.Fatalf("Stat(%q): %v", shard.PlaintextPath, err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %o, want 600", info.Mode().Perm())
		}
	}
	if len(events) == 0 || events[len(events)-1].Done != len(messages) {
		t.Fatalf("events = %+v", events)
	}
}

func TestBuildMessageShardsCleansPartialFilesOnFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cache := &failingPlannerCache{
		root: root,
		messages: map[string]Message{
			"m1": {ID: "m1", InternalDate: mustUnixMilli(t, "2026-03-01T10:00:00Z"), Raw: "raw-1"},
			"m2": {ID: "m2", InternalDate: mustUnixMilli(t, "2026-04-01T10:00:00Z"), Raw: "raw-2"},
		},
		failAfterReads: 3,
	}
	_, err := BuildMessageShards(context.Background(), cache, []string{"m1", "m2"}, ShardOptions{
		AccountHash: "accthash",
		MaxRows:     1,
	})
	if !errors.Is(err, errInjectedCacheRead) {
		t.Fatalf("error = %v, want injected cache read", err)
	}
	tempDir, _ := cache.MessageShardDir("accthash")
	if _, statErr := os.Stat(tempDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("temp dir still exists after failure: %v", statErr)
	}
}

func TestBuildMessageShardsCancellationCleansTempDir(t *testing.T) {
	t.Parallel()
	cache := newPlannerCache(t)
	if err := cache.WriteMessage("accthash", Message{ID: "m1", Raw: "raw"}); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := BuildMessageShards(ctx, cache, []string{"m1"}, ShardOptions{AccountHash: "accthash"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	tempDir, _ := cache.MessageShardDir("accthash")
	if _, statErr := os.Stat(tempDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("temp dir still exists after cancellation: %v", statErr)
	}
}

func TestBuildCheckpointShardsSplitsRowsAndBytes(t *testing.T) {
	t.Parallel()
	cache := newPlannerCache(t)
	ids := []string{"m1", "m2", "m3"}
	for _, id := range ids {
		if err := cache.WriteMessage("accthash", Message{ID: id, Raw: strings.Repeat("raw-"+id, 8)}); err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
	}

	shards, err := BuildCheckpointShards(context.Background(), cache, ids, CheckpointShardOptions{
		AccountHash:      "accthash",
		RunID:            "run-test",
		FirstPart:        7,
		MaxRows:          2,
		MaxPlaintextSize: 1,
	})
	if err != nil {
		t.Fatalf("BuildCheckpointShards: %v", err)
	}
	if len(shards) != 3 {
		t.Fatalf("len(shards) = %d, want 3", len(shards))
	}
	for i, shard := range shards {
		want := fmt.Sprintf("part-%06d.jsonl.gz.age", 7+i)
		if shard.Rows != 1 || !strings.HasSuffix(shard.Path, want) {
			t.Fatalf("shards[%d] = %+v, want rows=1 suffix=%q", i, shard, want)
		}
	}
}

func TestBuildCheckpointShardsTightensExistingFileModeBeforeRewrite(t *testing.T) {
	t.Parallel()
	cache := newPlannerCache(t)
	if err := cache.WriteMessage("accthash", Message{ID: "m1", Raw: "raw"}); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	opts := CheckpointShardOptions{
		AccountHash: "accthash",
		RunID:       "run-test",
		FirstPart:   1,
	}
	shards, err := BuildCheckpointShards(context.Background(), cache, []string{"m1"}, opts)
	if err != nil {
		t.Fatalf("BuildCheckpointShards first: %v", err)
	}
	path := shards[0].PlaintextPath
	if chmodErr := os.Chmod(path, 0o644); chmodErr != nil {
		t.Fatalf("Chmod permissive: %v", chmodErr)
	}

	if _, rewriteErr := BuildCheckpointShards(context.Background(), cache, []string{"m1"}, opts); rewriteErr != nil {
		t.Fatalf("BuildCheckpointShards rewrite: %v", rewriteErr)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func mustUnixMilli(t *testing.T, value string) int64 {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q): %v", value, err)
	}
	return parsed.UnixMilli()
}

func newPlannerCache(t *testing.T) Cache {
	t.Helper()
	cache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	return cache
}

var errInjectedCacheRead = errors.New("injected cache read failure")

type failingPlannerCache struct {
	root           string
	messages       map[string]Message
	reads          int
	failAfterReads int
}

func (c *failingPlannerCache) ReadMessage(_ string, messageID string) (Message, bool, error) {
	c.reads++
	if c.reads > c.failAfterReads {
		return Message{}, false, errInjectedCacheRead
	}
	msg, ok := c.messages[messageID]
	return msg, ok, nil
}

func (c *failingPlannerCache) MessageShardDir(accountHash string) (string, bool) {
	return filepath.Join(c.root, accountHash, "tmp-shards"), true
}

func (c *failingPlannerCache) CheckpointShardDir(accountHash, runID string) (string, bool) {
	return filepath.Join(c.root, accountHash, "checkpoint-shards", runID), true
}
