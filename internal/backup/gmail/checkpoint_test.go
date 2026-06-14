//nolint:wsl_v5 // Tests stay compact around setup/action/assert blocks.
package gmailbackup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/backup"
	appconfig "github.com/steipete/gogcli/internal/config"
)

func TestCheckpointerFlushesByRowsAndAtCompletion(t *testing.T) {
	ctx := context.Background()
	repo, _, _, backupOpts := newCheckpointBackup(t)
	cache := newPlannerCache(t)
	ids := []string{"m1", "m2", "m3", "m4", "m5"}
	writeCheckpointMessages(t, cache, "accthash", ids)

	var events []CheckpointEvent
	checkpointer := NewCheckpointer(CheckpointOptions{
		Enabled:       true,
		AccountHash:   "accthash",
		RunID:         "run-test",
		Rows:          2,
		Cache:         cache,
		BackupOptions: backupOpts,
		Progress:      func(event CheckpointEvent) { events = append(events, event) },
	}, len(ids))
	for index, id := range ids {
		done := index + 1
		if err := checkpointer.Record(ctx, id, Event{Done: done, Fetched: done}); err != nil {
			t.Fatalf("Record(%q): %v", id, err)
		}
	}

	manifest := readCheckpointManifestForTest(t, repo, "accthash", "run-test")
	if !manifest.Incomplete || manifest.Done != 5 || manifest.Total != 5 || len(manifest.Shards) != 3 {
		t.Fatalf("manifest = %+v", manifest)
	}
	if len(events) != 4 || events[0].Phase != CheckpointPhaseConfigured {
		t.Fatalf("events = %+v", events)
	}
	wantRows := []int{2, 2, 1}
	for index, want := range wantRows {
		event := events[index+1]
		if event.Phase != CheckpointPhaseFlushed || event.Rows != want {
			t.Fatalf("events[%d] = %+v, want flushed rows=%d", index+1, event, want)
		}
	}
	ciphertext, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(manifest.Shards[0].Path)))
	if err != nil {
		t.Fatalf("read checkpoint shard: %v", err)
	}
	if strings.Contains(string(ciphertext), "raw-m1") {
		t.Fatal("checkpoint shard contains plaintext")
	}
}

func TestCheckpointerFlushesByDurationWithInjectedClock(t *testing.T) {
	ctx := context.Background()
	repo, _, _, backupOpts := newCheckpointBackup(t)
	cache := newPlannerCache(t)
	ids := []string{"m1", "m2", "m3"}
	writeCheckpointMessages(t, cache, "accthash", ids)
	now := time.Date(2026, 4, 28, 1, 2, 3, 0, time.UTC)

	var flushed []CheckpointEvent
	checkpointer := NewCheckpointer(CheckpointOptions{
		Enabled:       true,
		AccountHash:   "accthash",
		RunID:         "run-test",
		Every:         10 * time.Minute,
		Cache:         cache,
		BackupOptions: backupOpts,
		Now:           func() time.Time { return now },
		Progress: func(event CheckpointEvent) {
			if event.Phase == CheckpointPhaseFlushed {
				flushed = append(flushed, event)
			}
		},
	}, len(ids))
	if err := checkpointer.Record(ctx, "m1", Event{Done: 1, Fetched: 1}); err != nil {
		t.Fatalf("Record(m1): %v", err)
	}
	now = now.Add(10 * time.Minute)
	if err := checkpointer.Record(ctx, "m2", Event{Done: 2, Fetched: 2}); err != nil {
		t.Fatalf("Record(m2): %v", err)
	}
	if err := checkpointer.Record(ctx, "m3", Event{Done: 3, Fetched: 3}); err != nil {
		t.Fatalf("Record(m3): %v", err)
	}

	if len(flushed) != 2 || flushed[0].Rows != 2 || flushed[1].Rows != 1 {
		t.Fatalf("flushed = %+v", flushed)
	}
	manifest := readCheckpointManifestForTest(t, repo, "accthash", "run-test")
	if manifest.Done != 3 || len(manifest.Shards) != 2 {
		t.Fatalf("manifest = %+v", manifest)
	}
}

func TestResolveCheckpointRunIDReusesExactMessageSelection(t *testing.T) {
	ctx := context.Background()
	_, _, _, backupOpts := newCheckpointBackup(t)
	ids := []string{"m1", "m2"}
	firstTime := time.Date(2026, 4, 28, 1, 2, 3, 0, time.UTC)
	opts := CheckpointOptions{
		Enabled:          true,
		AccountHash:      "accthash",
		Query:            "in:inbox",
		IncludeSpamTrash: true,
		BackupOptions:    backupOpts,
		Now:              func() time.Time { return firstTime },
	}
	runID := ResolveCheckpointRunID(ctx, ids, opts)
	pushCheckpointForTest(t, ctx, ids, runID, "accthash", backupOpts)

	var events []CheckpointEvent
	opts.Now = func() time.Time { return firstTime.Add(time.Hour) }
	opts.Progress = func(event CheckpointEvent) { events = append(events, event) }
	if got := ResolveCheckpointRunID(ctx, ids, opts); got != runID {
		t.Fatalf("resolved run ID = %q, want %q", got, runID)
	}
	if len(events) != 1 || events[0].Phase != CheckpointPhaseReused {
		t.Fatalf("events = %+v", events)
	}
	if got := ResolveCheckpointRunID(ctx, []string{"m1", "different"}, opts); got == runID {
		t.Fatalf("different message selection reused run %q", got)
	}
}

func TestResolveCheckpointRunIDRejectsMismatchedManifestRun(t *testing.T) {
	ctx := context.Background()
	repo, _, _, backupOpts := newCheckpointBackup(t)
	ids := []string{"m1"}
	firstTime := time.Date(2026, 4, 28, 1, 2, 3, 0, time.UTC)
	opts := CheckpointOptions{
		Enabled:       true,
		AccountHash:   "accthash",
		BackupOptions: backupOpts,
		Now:           func() time.Time { return firstTime },
	}
	runID := ResolveCheckpointRunID(ctx, ids, opts)
	pushCheckpointForTest(t, ctx, ids, runID, "accthash", backupOpts)
	manifestPath := checkpointManifestPathForTest(repo, "accthash", runID)
	manifest := readCheckpointManifestForTest(t, repo, "accthash", runID)
	manifest.RunID = "different-run"
	writeCheckpointManifestForTest(t, manifestPath, manifest)

	opts.Now = func() time.Time { return firstTime.Add(time.Hour) }
	if got := ResolveCheckpointRunID(ctx, ids, opts); got == runID {
		t.Fatalf("mismatched manifest reused run %q", got)
	}
}

func TestPromoteCheckpointRequiresCompleteCompatibleRun(t *testing.T) {
	ctx := context.Background()
	repo, config, recipients, backupOpts := newCheckpointBackup(t)
	ids := []string{"m1", "m2"}
	pushCheckpointForTest(t, ctx, ids, "run-test", "accthash", backupOpts)
	opts := CheckpointOptions{
		Enabled:       true,
		AccountHash:   "accthash",
		RunID:         "run-test",
		BackupOptions: backupOpts,
	}

	shards, promoted, err := PromoteCheckpoint(ctx, ids, opts)
	if err != nil {
		t.Fatalf("PromoteCheckpoint: %v", err)
	}
	if !promoted || len(shards) != 1 || shards[0].Existing == nil {
		t.Fatalf("promoted=%t shards=%+v", promoted, shards)
	}
	if !sameRecipients(shards[0].ExistingRecipients, recipients) {
		t.Fatalf("recipients = %v, want %v", shards[0].ExistingRecipients, recipients)
	}
	if _, statErr := os.Stat(filepath.Join(repo, filepath.FromSlash(shards[0].Path))); statErr != nil {
		t.Fatalf("checkpoint shard missing: %v", statErr)
	}

	secondIdentity := filepath.Join(t.TempDir(), "second.age")
	secondRecipient, err := backup.EnsureIdentity(secondIdentity)
	if err != nil {
		t.Fatalf("EnsureIdentity(second): %v", err)
	}
	if saveErr := backupOpts.ConfigStore.Save(config, backup.Config{
		Repo:       repo,
		Identity:   secondIdentity,
		Recipients: []string{secondRecipient},
	}); saveErr != nil {
		t.Fatalf("save changed recipients: %v", saveErr)
	}
	var events []CheckpointEvent
	opts.Progress = func(event CheckpointEvent) { events = append(events, event) }
	shards, promoted, err = PromoteCheckpoint(ctx, ids, opts)
	if err != nil {
		t.Fatalf("PromoteCheckpoint changed recipients: %v", err)
	}
	if promoted || len(shards) != 0 {
		t.Fatalf("changed recipients promoted=%t shards=%+v", promoted, shards)
	}
	if len(events) != 1 || events[0].Phase != CheckpointPhasePromotionSkipped || events[0].Reason != "recipients-changed" {
		t.Fatalf("events = %+v", events)
	}
}

func TestPromoteCheckpointRejectsIncompleteAndUnexpectedShards(t *testing.T) {
	ctx := context.Background()
	_, _, _, backupOpts := newCheckpointBackup(t)
	ids := []string{"m1", "m2"}
	pushCheckpointRowsForTest(t, ctx, []string{"m1"}, "run-incomplete", "accthash", backupOpts, 1, 2)
	if shards, promoted, err := PromoteCheckpoint(ctx, ids, CheckpointOptions{
		Enabled:       true,
		AccountHash:   "accthash",
		RunID:         "run-incomplete",
		BackupOptions: backupOpts,
	}); err != nil || promoted || len(shards) != 0 {
		t.Fatalf("incomplete promotion: promoted=%t shards=%+v err=%v", promoted, shards, err)
	}

	shard, err := backup.NewJSONLShard("drive", MessageShardKind, "accthash", "checkpoints/gmail/accthash/run-wrong/messages/part-000001.jsonl.gz.age", []Message{{ID: "m1", Raw: "raw-m1"}})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	if _, pushErr := backup.PushCheckpoint(ctx, backup.Snapshot{
		Services: []string{Service},
		Accounts: []string{"accthash"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []backup.PlainShard{shard},
	}, backup.Checkpoint{
		RunID:   "run-wrong",
		Service: Service,
		Account: "accthash",
		Done:    1,
		Total:   1,
	}, backupOpts); pushErr != nil {
		t.Fatalf("PushCheckpoint: %v", pushErr)
	}
	_, _, err = PromoteCheckpoint(ctx, []string{"m1"}, CheckpointOptions{
		Enabled:       true,
		AccountHash:   "accthash",
		RunID:         "run-wrong",
		BackupOptions: backupOpts,
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected shard") {
		t.Fatalf("unexpected shard error = %v", err)
	}
}

func TestCheckpointerWrapsCheckpointPushFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	repoFile := filepath.Join(dir, "repo-file")
	if err := os.WriteFile(repoFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("WriteFile(repo): %v", err)
	}
	config := filepath.Join(dir, "backup.json")
	identity := filepath.Join(dir, "age.key")
	recipient, err := backup.EnsureIdentity(identity)
	if err != nil {
		t.Fatalf("EnsureIdentity: %v", err)
	}
	backupOpts := checkpointBackupOptions(t, config)
	if saveErr := backupOpts.ConfigStore.Save(config, backup.Config{
		Repo:       repoFile,
		Identity:   identity,
		Recipients: []string{recipient},
	}); saveErr != nil {
		t.Fatalf("save config: %v", saveErr)
	}
	cache := newPlannerCache(t)
	writeCheckpointMessages(t, cache, "accthash", []string{"m1"})
	checkpointer := NewCheckpointer(CheckpointOptions{
		Enabled:       true,
		AccountHash:   "accthash",
		RunID:         "run-test",
		Rows:          1,
		Cache:         cache,
		BackupOptions: backupOpts,
	}, 1)

	err = checkpointer.Record(ctx, "m1", Event{Done: 1, Fetched: 1})
	if err == nil || !strings.Contains(err.Error(), "push Gmail backup checkpoint") {
		t.Fatalf("Record error = %v", err)
	}
}

func newCheckpointBackup(t *testing.T) (string, string, []string, backup.Options) {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	config := filepath.Join(dir, "backup.json")
	identity := filepath.Join(dir, "age.key")
	recipient, err := backup.EnsureIdentity(identity)
	if err != nil {
		t.Fatalf("EnsureIdentity: %v", err)
	}
	opts := checkpointBackupOptions(t, config)
	recipients := []string{recipient}
	if err := opts.ConfigStore.Save(config, backup.Config{
		Repo:       repo,
		Identity:   identity,
		Recipients: recipients,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return repo, config, recipients, opts
}

func checkpointBackupOptions(t *testing.T, config string) backup.Options {
	t.Helper()
	home := t.TempDir()
	store, err := backup.NewConfigStore(appconfig.Layout{
		ConfigDir:      filepath.Dir(config),
		ExplicitConfig: true,
	}, func() (string, error) { return home, nil })
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}
	return backup.Options{ConfigStore: store, ConfigPath: config, Push: false}
}

func writeCheckpointMessages(t *testing.T, cache Cache, accountHash string, ids []string) {
	t.Helper()
	for _, id := range ids {
		if err := cache.WriteMessage(accountHash, Message{ID: id, Raw: "raw-" + id}); err != nil {
			t.Fatalf("WriteMessage(%q): %v", id, err)
		}
	}
}

func pushCheckpointForTest(t *testing.T, ctx context.Context, ids []string, runID, accountHash string, opts backup.Options) {
	t.Helper()
	pushCheckpointRowsForTest(t, ctx, ids, runID, accountHash, opts, len(ids), len(ids))
}

func pushCheckpointRowsForTest(t *testing.T, ctx context.Context, ids []string, runID, accountHash string, opts backup.Options, done, total int) {
	t.Helper()
	messages := make([]Message, 0, len(ids))
	for _, id := range ids {
		messages = append(messages, Message{ID: id, Raw: "raw-" + id})
	}
	shard, err := backup.NewJSONLShard(
		Service,
		MessageShardKind,
		accountHash,
		"checkpoints/gmail/"+accountHash+"/"+runID+"/messages/part-000001.jsonl.gz.age",
		messages,
	)
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	if _, err := backup.PushCheckpoint(ctx, backup.Snapshot{
		Services: []string{Service},
		Accounts: []string{accountHash},
		Counts:   map[string]int{"gmail.messages": len(ids)},
		Shards:   []backup.PlainShard{shard},
	}, backup.Checkpoint{
		RunID:   runID,
		Service: Service,
		Account: accountHash,
		Done:    done,
		Total:   total,
	}, opts); err != nil {
		t.Fatalf("PushCheckpoint: %v", err)
	}
}

func readCheckpointManifestForTest(t *testing.T, repo, accountHash, runID string) backup.CheckpointManifest {
	t.Helper()
	data, err := os.ReadFile(checkpointManifestPathForTest(repo, accountHash, runID))
	if err != nil {
		t.Fatalf("read checkpoint manifest: %v", err)
	}
	var manifest backup.CheckpointManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal checkpoint manifest: %v", err)
	}
	return manifest
}

func writeCheckpointManifestForTest(t *testing.T, path string, manifest backup.CheckpointManifest) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal checkpoint manifest: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write checkpoint manifest: %v", err)
	}
}

func checkpointManifestPathForTest(repo, accountHash, runID string) string {
	return filepath.Join(repo, "checkpoints", Service, accountHash, runID, "manifest.json")
}
