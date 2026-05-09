package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/backup"
	"github.com/steipete/gogcli/internal/ui"
)

func TestBackupAccountHashStableAndOpaque(t *testing.T) {
	got := backupAccountHash("  User@Example.COM ")
	want := backupAccountHash("user@example.com")
	if got != want {
		t.Fatalf("hash not normalized: got %s want %s", got, want)
	}
	if len(got) != 24 {
		t.Fatalf("hash length = %d, want 24 hex chars", len(got))
	}
	if strings.Contains(got, "user") || strings.Contains(got, "example") {
		t.Fatalf("hash leaks account text: %s", got)
	}
}

func TestBackupReadFlagsOptionsSkipPull(t *testing.T) {
	opts := backupReadFlags{NoPull: true}.options()
	if !opts.SkipPull {
		t.Fatal("SkipPull = false, want true")
	}
	if opts.Push {
		t.Fatal("Push = true, want false")
	}
}

func TestBuildGmailMessageShardsBucketsSortsAndChunks(t *testing.T) {
	accountHash := "accthash"
	messages := []gmailBackupMessage{
		{ID: "march-new", InternalDate: mustUnixMilli(t, "2026-03-02T10:00:00Z"), Raw: "raw-3"},
		{ID: "april-later", InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"), Raw: "raw-2"},
		{ID: "april-earlier-b", InternalDate: mustUnixMilli(t, "2026-04-01T10:00:00Z"), Raw: "raw-1b"},
		{ID: "april-earlier-a", InternalDate: mustUnixMilli(t, "2026-04-01T10:00:00Z"), Raw: "raw-1a"},
	}

	shards, err := buildGmailMessageShards(accountHash, messages, 2)
	if err != nil {
		t.Fatalf("buildGmailMessageShards: %v", err)
	}
	if len(shards) != 3 {
		t.Fatalf("len(shards) = %d, want 3", len(shards))
	}
	wantPaths := []string{
		"data/gmail/accthash/messages/2026/03/part-0001.jsonl.gz.age",
		"data/gmail/accthash/messages/2026/04/part-0001.jsonl.gz.age",
		"data/gmail/accthash/messages/2026/04/part-0002.jsonl.gz.age",
	}
	for i, want := range wantPaths {
		if shards[i].Path != want {
			t.Fatalf("shards[%d].Path = %q, want %q", i, shards[i].Path, want)
		}
	}
	if shards[0].Rows != 1 || shards[1].Rows != 2 || shards[2].Rows != 1 {
		t.Fatalf("unexpected row counts: %d %d %d", shards[0].Rows, shards[1].Rows, shards[2].Rows)
	}

	var aprilFirst []gmailBackupMessage
	if err := backup.DecodeJSONL(shards[1].Plaintext, &aprilFirst); err != nil {
		t.Fatalf("DecodeJSONL: %v", err)
	}
	gotIDs := []string{aprilFirst[0].ID, aprilFirst[1].ID}
	wantIDs := []string{"april-earlier-a", "april-earlier-b"}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("april shard IDs = %v, want %v", gotIDs, wantIDs)
		}
	}
}

func TestBuildGmailMessageShardsSplitsByPlaintextSize(t *testing.T) {
	accountHash := "accthash"
	messages := []gmailBackupMessage{
		{ID: "m1", InternalDate: mustUnixMilli(t, "2026-04-01T10:00:00Z"), Raw: strings.Repeat("raw-1", 8)},
		{ID: "m2", InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"), Raw: strings.Repeat("raw-2", 8)},
		{ID: "m3", InternalDate: mustUnixMilli(t, "2026-04-03T10:00:00Z"), Raw: strings.Repeat("raw-3", 8)},
	}

	shards, err := buildGmailMessageShardsWithLimit(accountHash, messages, 100, 1)
	if err != nil {
		t.Fatalf("buildGmailMessageShardsWithLimit: %v", err)
	}
	if len(shards) != 3 {
		t.Fatalf("len(shards) = %d, want 3", len(shards))
	}
	for i, shard := range shards {
		if shard.Rows != 1 {
			t.Fatalf("shards[%d].Rows = %d, want 1", i, shard.Rows)
		}
		want := fmt.Sprintf("part-%04d.jsonl.gz.age", i+1)
		if !strings.HasSuffix(shard.Path, want) {
			t.Fatalf("shards[%d].Path = %q, want suffix %q", i, shard.Path, want)
		}
	}
}

func TestMergeBackupSnapshotsKeepsCountsAndShardOrder(t *testing.T) {
	left := backup.Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct1"},
		Counts:   map[string]int{"gmail.messages": 2},
		Shards:   []backup.PlainShard{{Path: "data/gmail/acct1/messages/2026/04/part-0001.jsonl.gz.age"}},
	}
	right := backup.Snapshot{
		Services: []string{"calendar"},
		Accounts: []string{"acct1"},
		Counts:   map[string]int{"calendar.events": 3},
		Shards:   []backup.PlainShard{{Path: "data/calendar/acct1/events.jsonl.gz.age"}},
	}

	merged := mergeBackupSnapshots(left, right)
	if merged.Counts["gmail.messages"] != 2 || merged.Counts["calendar.events"] != 3 {
		t.Fatalf("unexpected counts: %+v", merged.Counts)
	}
	if len(merged.Shards) != 2 || merged.Shards[0].Path != left.Shards[0].Path || merged.Shards[1].Path != right.Shards[0].Path {
		t.Fatalf("unexpected shard order: %+v", merged.Shards)
	}
}

func TestExpandBackupServicesAllIncludesWorkspaceAdapters(t *testing.T) {
	got := strings.Join(expandBackupServices([]string{"all"}), ",")
	for _, want := range []string{
		"appscript",
		"calendar",
		"chat",
		"classroom",
		"contacts",
		"drive",
		"gmail",
		"gmail-settings",
		"groups",
		"admin",
		"keep",
		"tasks",
		"workspace",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expanded all missing %q in %q", want, got)
		}
	}
}

func TestGmailBackupMessageCacheRoundTrips(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	message := gmailBackupMessage{
		ID:           "msg-one",
		ThreadID:     "thread-one",
		InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"),
		LabelIDs:     []string{"INBOX"},
		Raw:          base64.RawURLEncoding.EncodeToString([]byte("Subject: Cached\r\n\r\nBody")),
	}
	if err := writeGmailBackupMessageCache("accthash", message); err != nil {
		t.Fatalf("writeGmailBackupMessageCache: %v", err)
	}
	got, ok, err := readGmailBackupMessageCache("accthash", "msg-one")
	if err != nil {
		t.Fatalf("readGmailBackupMessageCache: %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ID != message.ID || got.ThreadID != message.ThreadID || got.Raw != message.Raw {
		t.Fatalf("cache round trip mismatch: %#v", got)
	}
	path, ok := gmailBackupMessageCachePath("accthash", "msg-one")
	if !ok {
		t.Fatal("expected cache path")
	}
	if strings.Contains(path, "msg-one") {
		t.Fatalf("cache path should hash message IDs, got %q", path)
	}
}

func TestGmailBackupMessageCacheRejectsWrongID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	message := gmailBackupMessage{ID: "msg-one", Raw: "raw"}
	if err := writeGmailBackupMessageCache("accthash", message); err != nil {
		t.Fatalf("writeGmailBackupMessageCache: %v", err)
	}
	path, ok := gmailBackupMessageCachePath("accthash", "msg-one")
	if !ok {
		t.Fatal("expected cache path")
	}
	data, err := json.Marshal(gmailBackupMessage{ID: "other", Raw: "raw"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, err := readGmailBackupMessageCache("accthash", "msg-one"); err == nil {
		t.Fatal("expected wrong cache ID to fail")
	}
}

func TestListGmailBackupMessageIDsResumesFromCheckpoint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	opts := gmailBackupOptions{
		AccountHash:      "accthash",
		IncludeSpamTrash: true,
		CacheMessages:    true,
	}
	path, ok := gmailBackupListStatePath(opts)
	if !ok {
		t.Fatal("expected list state path")
	}
	if err := writeGmailBackupListState(path, opts, []string{"m1"}, "p2", false); err != nil {
		t.Fatalf("writeGmailBackupListState: %v", err)
	}

	requests := 0
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("pageToken"); got != "p2" {
			t.Fatalf("pageToken = %q, want p2", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"messages": []map[string]string{{"id": "m2"}},
		})
	})
	defer cleanup()

	var stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ids, err := listGmailBackupMessageIDs(ui.WithUI(context.Background(), u), svc, opts)
	if err != nil {
		t.Fatalf("listGmailBackupMessageIDs: %v", err)
	}
	if strings.Join(ids, ",") != "m1,m2" {
		t.Fatalf("ids = %v, want [m1 m2]", ids)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if !strings.Contains(stderr.String(), "resume=partial") || !strings.Contains(stderr.String(), "messages=2") {
		t.Fatalf("stderr missing progress: %s", stderr.String())
	}
	state, ok, err := readGmailBackupListState(path)
	if err != nil {
		t.Fatalf("readGmailBackupListState: %v", err)
	}
	if !ok || !state.Complete || strings.Join(state.IDs, ",") != "m1,m2" {
		t.Fatalf("state = %#v ok=%t", state, ok)
	}
}

func TestListGmailBackupMessageIDsReusesCompleteCheckpoint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	opts := gmailBackupOptions{
		AccountHash:      "accthash",
		IncludeSpamTrash: true,
		CacheMessages:    true,
	}
	path, ok := gmailBackupListStatePath(opts)
	if !ok {
		t.Fatal("expected list state path")
	}
	if err := writeGmailBackupListState(path, opts, []string{"m1", "m2"}, "", true); err != nil {
		t.Fatalf("writeGmailBackupListState: %v", err)
	}

	requests := 0
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	})
	defer cleanup()

	ids, err := listGmailBackupMessageIDs(context.Background(), svc, opts)
	if err != nil {
		t.Fatalf("listGmailBackupMessageIDs: %v", err)
	}
	if strings.Join(ids, ",") != "m1,m2" {
		t.Fatalf("ids = %v, want [m1 m2]", ids)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
}

func TestListGmailBackupMessageIDsMarksMaxLimitedRunComplete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	opts := gmailBackupOptions{
		AccountHash:      "accthash",
		Max:              1,
		IncludeSpamTrash: true,
		CacheMessages:    true,
	}
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"messages":      []map[string]string{{"id": "m1"}},
			"nextPageToken": "p2",
		})
	})
	defer cleanup()

	ids, err := listGmailBackupMessageIDs(context.Background(), svc, opts)
	if err != nil {
		t.Fatalf("listGmailBackupMessageIDs: %v", err)
	}
	if strings.Join(ids, ",") != "m1" {
		t.Fatalf("ids = %v, want [m1]", ids)
	}
	path, ok := gmailBackupListStatePath(opts)
	if !ok {
		t.Fatal("expected list state path")
	}
	state, ok, err := readGmailBackupListState(path)
	if err != nil {
		t.Fatalf("readGmailBackupListState: %v", err)
	}
	if !ok || !state.Complete || state.PageToken != "" {
		t.Fatalf("state = %#v ok=%t", state, ok)
	}
}

func TestEnsureGmailBackupMessageCacheStopsOnFirstFetchError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var requests atomic.Int32
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, `{"error":{"code":401,"message":"invalid credentials"}}`, http.StatusUnauthorized)
	})
	defer cleanup()

	ids := make([]string, 100)
	for i := range ids {
		ids[i] = fmt.Sprintf("msg-%03d", i)
	}
	err := ensureGmailBackupMessageCache(context.Background(), svc, gmailBackupOptions{
		AccountHash:      "accthash",
		CacheMessages:    true,
		IncludeSpamTrash: true,
	}, ids)
	if err == nil || !strings.Contains(err.Error(), "gmail message msg-") {
		t.Fatalf("expected message fetch error, got %v", err)
	}
	if got := requests.Load(); got > 4 {
		t.Fatalf("requests = %d, want fail-fast bounded requests", got)
	}
}

func TestEnsureGmailBackupMessageCacheWritesEncryptedCheckpoints(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	identity := filepath.Join(dir, "age.key")
	config := filepath.Join(dir, "backup.json")
	recipient, err := backup.EnsureIdentity(identity)
	if err != nil {
		t.Fatalf("EnsureIdentity: %v", err)
	}
	if saveErr := backup.SaveConfig(config, backup.Config{
		Repo:       repo,
		Identity:   identity,
		Recipients: []string{recipient},
	}); saveErr != nil {
		t.Fatalf("SaveConfig: %v", saveErr)
	}
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		id := filepath.Base(r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           id,
			"threadId":     "thread-" + id,
			"historyId":    "100",
			"internalDate": fmt.Sprint(mustUnixMilli(t, "2026-04-02T10:00:00Z")),
			"labelIds":     []string{"INBOX"},
			"sizeEstimate": 42,
			"raw":          base64.RawURLEncoding.EncodeToString([]byte("Subject: " + id + "\r\n\r\nBody")),
		})
	})
	defer cleanup()

	ids := []string{"m1", "m2", "m3", "m4", "m5"}
	err = ensureGmailBackupMessageCache(context.Background(), svc, gmailBackupOptions{
		AccountHash:      "accthash",
		CacheMessages:    true,
		IncludeSpamTrash: true,
		Checkpoints:      true,
		CheckpointRows:   2,
		CheckpointRunID:  "run-test",
		BackupOptions:    backup.Options{ConfigPath: config, Push: false},
	}, ids)
	if err != nil {
		t.Fatalf("ensureGmailBackupMessageCache: %v", err)
	}
	manifestPath := filepath.Join(repo, "checkpoints", "gmail", "accthash", "run-test", "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read checkpoint manifest: %v", err)
	}
	var manifest backup.CheckpointManifest
	if unmarshalErr := json.Unmarshal(data, &manifest); unmarshalErr != nil {
		t.Fatalf("unmarshal checkpoint manifest: %v", unmarshalErr)
	}
	if !manifest.Incomplete || manifest.Done != 5 || manifest.Total != 5 || len(manifest.Shards) != 3 {
		t.Fatalf("unexpected checkpoint manifest: %+v", manifest)
	}
	ciphertext, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(manifest.Shards[0].Path)))
	if err != nil {
		t.Fatalf("read checkpoint shard: %v", err)
	}
	if strings.Contains(string(ciphertext), "Subject:") {
		t.Fatal("checkpoint shard contains plaintext")
	}
}

func TestBuildGmailMessageShardsFromCheckpointPromotesCompleteRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	repo, config, recipients := newBackupConfigForCmdTest(t)
	checkpointShard, err := backup.NewJSONLShard(backupServiceGmail, "messages", "accthash", "checkpoints/gmail/accthash/run-test/messages/part-000001.jsonl.gz.age", []gmailBackupMessage{
		{ID: "m1", Raw: "raw-1"},
		{ID: "m2", Raw: "raw-2"},
	})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	if _, pushErr := backup.PushCheckpoint(ctx, backup.Snapshot{
		Services: []string{backupServiceGmail},
		Accounts: []string{"accthash"},
		Counts:   map[string]int{"gmail.messages": 2},
		Shards:   []backup.PlainShard{checkpointShard},
	}, backup.Checkpoint{
		RunID:   "run-test",
		Service: backupServiceGmail,
		Account: "accthash",
		Done:    2,
		Total:   2,
	}, backup.Options{ConfigPath: config, Push: false}); pushErr != nil {
		t.Fatalf("PushCheckpoint: %v", pushErr)
	}

	shards, promoted, err := buildGmailMessageShardsFromCheckpoint(ctx, gmailBackupOptions{
		AccountHash:     "accthash",
		CacheMessages:   true,
		Checkpoints:     true,
		CheckpointRunID: "run-test",
		BackupOptions:   backup.Options{ConfigPath: config},
	}, []string{"m1", "m2"})
	if err != nil {
		t.Fatalf("buildGmailMessageShardsFromCheckpoint: %v", err)
	}
	if !promoted || len(shards) != 1 || shards[0].Existing == nil {
		t.Fatalf("expected promoted existing shard, promoted=%t shards=%+v", promoted, shards)
	}
	if shards[0].Path != "checkpoints/gmail/accthash/run-test/messages/part-000001.jsonl.gz.age" {
		t.Fatalf("promoted path = %q", shards[0].Path)
	}
	if !sameBackupRecipients(shards[0].ExistingRecipients, recipients) {
		t.Fatalf("promoted recipients = %v, want %v", shards[0].ExistingRecipients, recipients)
	}
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(shards[0].Path))); err != nil {
		t.Fatalf("checkpoint shard missing: %v", err)
	}
}

func TestGmailBackupResolvedCheckpointRunIDReusesSelectionRun(t *testing.T) {
	ctx := context.Background()
	_, config, _ := newBackupConfigForCmdTest(t)
	ids := []string{"m1"}
	opts := gmailBackupOptions{
		AccountHash:      "accthash",
		CacheMessages:    true,
		Checkpoints:      true,
		IncludeSpamTrash: true,
		BackupOptions:    backup.Options{ConfigPath: config},
	}
	runID := "20260428T010203Z-" + gmailBackupCheckpointRunIDSuffix(opts, ids)
	checkpointShard, err := backup.NewJSONLShard(backupServiceGmail, "messages", "accthash", fmt.Sprintf("checkpoints/gmail/accthash/%s/messages/part-000001.jsonl.gz.age", runID), []gmailBackupMessage{
		{ID: "m1", Raw: "raw-1"},
	})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	if _, err := backup.PushCheckpoint(ctx, backup.Snapshot{
		Services: []string{backupServiceGmail},
		Accounts: []string{"accthash"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []backup.PlainShard{checkpointShard},
	}, backup.Checkpoint{
		RunID:   runID,
		Service: backupServiceGmail,
		Account: "accthash",
		Done:    1,
		Total:   1,
	}, backup.Options{ConfigPath: config, Push: false}); err != nil {
		t.Fatalf("PushCheckpoint: %v", err)
	}

	if got := gmailBackupResolvedCheckpointRunID(ctx, opts, ids); got != runID {
		t.Fatalf("resolved run ID = %q, want %q", got, runID)
	}
}

func TestBuildGmailCheckpointShardFromCacheWritesPlaintextPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	accountHash := "accthash"
	for _, message := range []gmailBackupMessage{
		{ID: "m1", InternalDate: mustUnixMilli(t, "2026-04-01T10:00:00Z"), Raw: "raw-1"},
		{ID: "m2", InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"), Raw: "raw-2"},
	} {
		if err := writeGmailBackupMessageCache(accountHash, message); err != nil {
			t.Fatalf("writeGmailBackupMessageCache: %v", err)
		}
	}
	shard, err := buildGmailCheckpointShardFromCache(accountHash, "run-test", 3, []string{"m1", "m2"})
	if err != nil {
		t.Fatalf("buildGmailCheckpointShardFromCache: %v", err)
	}
	if shard.Path != "checkpoints/gmail/accthash/run-test/messages/part-000003.jsonl.gz.age" {
		t.Fatalf("checkpoint shard path = %q", shard.Path)
	}
	if shard.Rows != 2 || shard.PlaintextPath == "" {
		t.Fatalf("unexpected shard: %+v", shard)
	}
	data, err := os.ReadFile(shard.PlaintextPath)
	if err != nil {
		t.Fatalf("read checkpoint plaintext: %v", err)
	}
	var rows []gmailBackupMessage
	if err := backup.DecodeJSONL(data, &rows); err != nil {
		t.Fatalf("DecodeJSONL: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != "m1" || rows[1].ID != "m2" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestBuildGmailCheckpointShardsFromCacheSplitsLargeChunks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	accountHash := "accthash"
	ids := make([]string, gmailCheckpointShardMaxRows+1)
	for i := range ids {
		id := fmt.Sprintf("m-%04d", i)
		ids[i] = id
		if err := writeGmailBackupMessageCache(accountHash, gmailBackupMessage{ID: id, Raw: "raw-" + id}); err != nil {
			t.Fatalf("writeGmailBackupMessageCache: %v", err)
		}
	}
	shards, err := buildGmailCheckpointShardsFromCache(accountHash, "run-test", 7, ids)
	if err != nil {
		t.Fatalf("buildGmailCheckpointShardsFromCache: %v", err)
	}
	if len(shards) != 2 {
		t.Fatalf("len(shards) = %d, want 2", len(shards))
	}
	if shards[0].Rows != gmailCheckpointShardMaxRows || shards[1].Rows != 1 {
		t.Fatalf("rows = %d,%d", shards[0].Rows, shards[1].Rows)
	}
	if !strings.HasSuffix(shards[0].Path, "part-000007.jsonl.gz.age") || !strings.HasSuffix(shards[1].Path, "part-000008.jsonl.gz.age") {
		t.Fatalf("paths = %q %q", shards[0].Path, shards[1].Path)
	}
}

func TestBuildGmailCheckpointShardsFromCacheSplitsByPlaintextSize(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	oldLimit := gmailCheckpointShardMaxPlaintextBytes
	gmailCheckpointShardMaxPlaintextBytes = 1
	t.Cleanup(func() { gmailCheckpointShardMaxPlaintextBytes = oldLimit })
	accountHash := "accthash"
	ids := []string{"m1", "m2", "m3"}
	for _, id := range ids {
		if err := writeGmailBackupMessageCache(accountHash, gmailBackupMessage{ID: id, Raw: strings.Repeat("raw-"+id, 8)}); err != nil {
			t.Fatalf("writeGmailBackupMessageCache: %v", err)
		}
	}
	shards, err := buildGmailCheckpointShardsFromCache(accountHash, "run-test", 11, ids)
	if err != nil {
		t.Fatalf("buildGmailCheckpointShardsFromCache: %v", err)
	}
	if len(shards) != 3 {
		t.Fatalf("len(shards) = %d, want 3", len(shards))
	}
	for i, shard := range shards {
		if shard.Rows != 1 {
			t.Fatalf("shards[%d].Rows = %d, want 1", i, shard.Rows)
		}
		want := fmt.Sprintf("part-%06d.jsonl.gz.age", 11+i)
		if !strings.HasSuffix(shard.Path, want) {
			t.Fatalf("shards[%d].Path = %q, want suffix %q", i, shard.Path, want)
		}
	}
}

func TestBuildGmailMessageShardsFromCacheSplitsByPlaintextSize(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	accountHash := "accthash"
	ids := []string{"m1", "m2", "m3"}
	for _, id := range ids {
		if err := writeGmailBackupMessageCache(accountHash, gmailBackupMessage{
			ID:           id,
			InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"),
			Raw:          strings.Repeat("raw-"+id, 8),
		}); err != nil {
			t.Fatalf("writeGmailBackupMessageCache: %v", err)
		}
	}
	shards, err := buildGmailMessageShardsFromCacheWithLimit(context.Background(), gmailBackupOptions{
		AccountHash:  accountHash,
		ShardMaxRows: 100,
	}, ids, 1)
	if err != nil {
		t.Fatalf("buildGmailMessageShardsFromCacheWithLimit: %v", err)
	}
	if len(shards) != 3 {
		t.Fatalf("len(shards) = %d, want 3", len(shards))
	}
	for i, shard := range shards {
		if shard.Rows != 1 {
			t.Fatalf("shards[%d].Rows = %d, want 1", i, shard.Rows)
		}
		want := fmt.Sprintf("part-%04d.jsonl.gz.age", i+1)
		if !strings.HasSuffix(shard.Path, want) {
			t.Fatalf("shards[%d].Path = %q, want suffix %q", i, shard.Path, want)
		}
	}
}

func TestBuildGmailMessageShardsFromCacheWritesPlaintextPaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	accountHash := "accthash"
	messages := []gmailBackupMessage{
		{ID: "april-b", InternalDate: mustUnixMilli(t, "2026-04-02T10:00:00Z"), Raw: "raw-b"},
		{ID: "april-a", InternalDate: mustUnixMilli(t, "2026-04-01T10:00:00Z"), Raw: "raw-a"},
		{ID: "march-a", InternalDate: mustUnixMilli(t, "2026-03-01T10:00:00Z"), Raw: "raw-m"},
	}
	for _, message := range messages {
		if err := writeGmailBackupMessageCache(accountHash, message); err != nil {
			t.Fatalf("writeGmailBackupMessageCache: %v", err)
		}
	}
	shards, err := buildGmailMessageShardsFromCache(context.Background(), gmailBackupOptions{
		AccountHash:  accountHash,
		ShardMaxRows: 1,
	}, []string{"april-b", "april-a", "march-a"})
	if err != nil {
		t.Fatalf("buildGmailMessageShardsFromCache: %v", err)
	}
	if len(shards) != 3 {
		t.Fatalf("len(shards) = %d, want 3", len(shards))
	}
	wantPaths := []string{
		"data/gmail/accthash/messages/2026/03/part-0001.jsonl.gz.age",
		"data/gmail/accthash/messages/2026/04/part-0001.jsonl.gz.age",
		"data/gmail/accthash/messages/2026/04/part-0002.jsonl.gz.age",
	}
	for i, want := range wantPaths {
		if shards[i].Path != want {
			t.Fatalf("shards[%d].Path = %q, want %q", i, shards[i].Path, want)
		}
		if shards[i].PlaintextPath == "" {
			t.Fatalf("shards[%d] missing PlaintextPath", i)
		}
		data, err := os.ReadFile(shards[i].PlaintextPath)
		if err != nil {
			t.Fatalf("read plaintext shard: %v", err)
		}
		var rows []gmailBackupMessage
		if err := backup.DecodeJSONL(data, &rows); err != nil {
			t.Fatalf("DecodeJSONL: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("rows len = %d, want 1", len(rows))
		}
	}
}

func TestFetchBackupDriveCollaborationCollectsMetadataAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/file1/permissions"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{{"id": "perm1", "type": "user", "role": "reader", "emailAddress": "a@example.com"}},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/file1/comments"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"comments": []map[string]any{{"id": "comment1", "content": "hello", "resolved": false}},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/file1/revisions"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"revisions": []map[string]any{{"id": "rev1", "modifiedTime": "2026-04-02T10:00:00Z"}},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/file2/permissions"):
			http.Error(w, `{"error":{"message":"denied"}}`, http.StatusForbidden)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/file2/comments"):
			_ = json.NewEncoder(w).Encode(map[string]any{})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/files/file2/revisions"):
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(t.Context(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	got, counts := fetchBackupDriveCollaboration(t.Context(), svc, []driveBackupFile{
		{File: &drive.File{Id: "file1"}},
		{File: &drive.File{Id: "file2"}},
	})
	if counts["drive.permissions"] != 2 || counts["drive.comments"] != 1 || counts["drive.revisions"] != 1 || counts["drive.collab.errors"] != 1 {
		t.Fatalf("unexpected counts: %#v", counts)
	}
	if got.Permissions[0].FileID != "file1" || got.Permissions[0].Permission.Id != "perm1" {
		t.Fatalf("unexpected permission row: %#v", got.Permissions[0])
	}
	if got.Permissions[1].FileID != "file2" || got.Permissions[1].Error == "" {
		t.Fatalf("expected file2 permission error row: %#v", got.Permissions[1])
	}
}

func TestDomainFromAccount(t *testing.T) {
	if got := domainFromAccount("Admin@Example.COM"); got != "Example.COM" {
		t.Fatalf("domainFromAccount = %q", got)
	}
	if got := domainFromAccount("example.com"); got != "example.com" {
		t.Fatalf("domainFromAccount without @ = %q", got)
	}
}

func TestDriveBackupContentPlansPreferReadableWorkspaceFormats(t *testing.T) {
	docPlans := driveBackupContentPlans(&drive.File{Id: "doc1", Name: "Spec", MimeType: driveMimeGoogleDoc}, false)
	if len(docPlans) != 2 || docPlans[0].Extension != ".docx" || docPlans[1].Extension != ".md" {
		t.Fatalf("unexpected doc plans: %#v", docPlans)
	}
	sheetPlans := driveBackupContentPlans(&drive.File{Id: "sheet1", Name: "Budget", MimeType: driveMimeGoogleSheet}, false)
	if len(sheetPlans) != 1 || sheetPlans[0].Extension != ".xlsx" {
		t.Fatalf("unexpected sheet plans: %#v", sheetPlans)
	}
	folderPlans := driveBackupContentPlans(&drive.File{Id: "folder1", Name: "Folder", MimeType: driveMimeGoogleFolder}, false)
	if len(folderPlans) != 0 {
		t.Fatalf("folder should not have content plans: %#v", folderPlans)
	}
	binaryPlans := driveBackupContentPlans(&drive.File{Id: "bin1", Name: "Archive.zip", MimeType: "application/zip"}, false)
	if len(binaryPlans) != 0 {
		t.Fatalf("binary should be opt-in: %#v", binaryPlans)
	}
	binaryPlans = driveBackupContentPlans(&drive.File{Id: "bin1", Name: "Archive.zip", MimeType: "application/zip"}, true)
	if len(binaryPlans) != 1 || binaryPlans[0].Source != "download" {
		t.Fatalf("unexpected binary plans: %#v", binaryPlans)
	}
}

func TestDownloadDriveBackupContentHonorsTimeout(t *testing.T) {
	origExport := driveExportDownload
	t.Cleanup(func() { driveExportDownload = origExport })
	driveExportDownload = func(ctx context.Context, _ *drive.Service, _, _ string) (*http.Response, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, err := downloadDriveBackupContent(t.Context(), nil, &drive.File{Id: "doc1"}, driveBackupContentPlan{
		MimeType: mimePDF,
		Source:   "export",
	}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestExportDriveContentsWritesReadableFilesAndIndex(t *testing.T) {
	outDir := t.TempDir()
	row := driveBackupContent{
		FileID:     "file/one",
		Name:       "Quarterly Plan",
		MimeType:   driveMimeGoogleDoc,
		ExportName: "Quarterly_Plan.md",
		ExportMime: mimeTextMarkdown,
		Source:     "export",
		Size:       8,
		DataBase64: base64.StdEncoding.EncodeToString([]byte("# Plan\n")),
	}
	shard, err := backup.NewJSONLShard("drive", "contents", "acct/hash", "data/drive/acct/contents/part-0001.jsonl.gz.age", []driveBackupContent{row})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}

	files, count, err := exportDriveContents(outDir, shard)
	if err != nil {
		t.Fatalf("exportDriveContents: %v", err)
	}
	if files != 2 || count != 1 {
		t.Fatalf("files,count = %d,%d want 2,1", files, count)
	}
	exported := readText(t, filepath.Join(outDir, "drive", "acct_hash", "files", "file_one", "Quarterly_Plan.md"))
	if exported != "# Plan\n" {
		t.Fatalf("exported = %q", exported)
	}
	index := readText(t, filepath.Join(outDir, "drive", "acct_hash", "files", "index.jsonl"))
	if !strings.Contains(index, `"fileId":"file/one"`) || !strings.Contains(index, `"path":"drive/acct_hash/files/file_one/Quarterly_Plan.md"`) {
		t.Fatalf("index missing expected fields: %s", index)
	}
}

func TestEnsureExportOutsideRepoRejectsNestedPlaintext(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, "data"), 0o700); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := ensureExportOutsideRepo(filepath.Join(repo, "plaintext"), repo); err == nil {
		t.Fatal("expected nested export dir to be rejected")
	}
	if err := ensureExportOutsideRepo(filepath.Join(filepath.Dir(repo), "export"), repo); err != nil {
		t.Fatalf("outside export rejected: %v", err)
	}
}

func TestResetExportTargetsKeepsGmailMessageFiles(t *testing.T) {
	outDir := t.TempDir()
	messagePath := filepath.Join(outDir, "gmail", "acct_hash", "messages", "2026", "04", "message.md")
	indexPath := filepath.Join(outDir, "gmail", "acct_hash", "messages", "index.jsonl")
	if err := os.MkdirAll(filepath.Dir(messagePath), 0o700); err != nil {
		t.Fatalf("mkdir message dir: %v", err)
	}
	if err := os.WriteFile(messagePath, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write message: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("reset"), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}

	err := resetExportTargets(outDir, []backup.ShardEntry{{
		Service: backupServiceGmail,
		Kind:    "messages",
		Account: "acct/hash",
	}})
	if err != nil {
		t.Fatalf("resetExportTargets: %v", err)
	}
	if got := readText(t, messagePath); got != "keep" {
		t.Fatalf("message file = %q, want keep", got)
	}
	if _, err := os.Stat(indexPath); !os.IsNotExist(err) {
		t.Fatalf("index still exists or stat failed: %v", err)
	}
}

func newBackupConfigForCmdTest(t *testing.T) (string, string, []string) {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	identity := filepath.Join(dir, "age.key")
	config := filepath.Join(dir, "backup.json")
	recipient, err := backup.EnsureIdentity(identity)
	if err != nil {
		t.Fatalf("EnsureIdentity: %v", err)
	}
	recipients := []string{recipient}
	if err := backup.SaveConfig(config, backup.Config{Repo: repo, Identity: identity, Recipients: recipients}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	return repo, config, recipients
}

func mustUnixMilli(t *testing.T, value string) int64 {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed.UnixMilli()
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
