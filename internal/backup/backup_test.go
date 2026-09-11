//nolint:wsl_v5 // Tests stay compact around setup/action/assert blocks.
package backup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestPushSnapshotAndVerify(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)

	shard, err := NewJSONLShard("gmail", "messages", "acct", "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", []map[string]string{
		{"id": "m1", "raw": "private email body"},
	})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	result, err := PushSnapshot(ctx, Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []PlainShard{shard},
	}, testOptions(t, Options{ConfigPath: config, Push: false}))
	if err != nil {
		t.Fatalf("PushSnapshot: %v", err)
	}
	if !result.Changed || result.Shards != 1 || result.Counts["gmail.messages"] != 1 {
		t.Fatalf("unexpected push result: %+v", result)
	}

	ciphertext, err := os.ReadFile(filepath.Join(repo, "data", "gmail", "acct", "messages", "2026", "04", "part-0001.jsonl.gz.age"))
	if err != nil {
		t.Fatalf("read ciphertext: %v", err)
	}
	if strings.Contains(string(ciphertext), "private email body") {
		t.Fatal("encrypted shard contains plaintext")
	}

	verify, err := Verify(ctx, testOptions(t, Options{ConfigPath: config}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verify.Shards != 1 || verify.Counts["gmail.messages"] != 1 {
		t.Fatalf("unexpected verify result: %+v", verify)
	}

	status, statusRepo, err := Status(ctx, testOptions(t, Options{ConfigPath: config}))
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if statusRepo != repo || !status.Encrypted || status.Counts["gmail.messages"] != 1 {
		t.Fatalf("unexpected status repo=%s manifest=%+v", statusRepo, status)
	}
}

func TestVerifyReportsManifestSemanticCounts(t *testing.T) {
	ctx, _, config, _ := initTestBackup(t)

	shard, err := NewJSONLShard("contacts", "people", "acct", "data/contacts/acct/people/part-0001.jsonl.gz.age", []map[string]string{
		{"source": "connections"},
		{"source": "other"},
	})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	if _, pushErr := PushSnapshot(ctx, Snapshot{
		Services: []string{"contacts"},
		Accounts: []string{"acct"},
		Counts: map[string]int{
			"contacts.connections": 1,
			"contacts.other":       1,
			"contacts.people":      99,
		},
		Shards: []PlainShard{shard},
	}, testOptions(t, Options{ConfigPath: config, Push: false})); pushErr != nil {
		t.Fatalf("PushSnapshot: %v", pushErr)
	}

	verify, err := Verify(ctx, testOptions(t, Options{ConfigPath: config}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verify.Counts["contacts.connections"] != 1 || verify.Counts["contacts.other"] != 1 || verify.Counts["contacts.people"] != 2 {
		t.Fatalf("unexpected verify counts: %+v", verify.Counts)
	}
}

func TestVerifyReportsManifestCountsForSemanticCollisions(t *testing.T) {
	ctx, _, config, _ := initTestBackup(t)

	shard, err := NewJSONLShard("drive", "contents", "acct", "data/drive/acct/contents/part-0001.jsonl.gz.age", []map[string]any{
		{"fileID": "ok", "dataBase64": "b2s="},
		{"fileID": "skipped", "skipped": true},
		{"fileID": "error", "error": "export failed"},
	})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	if _, pushErr := PushSnapshot(ctx, Snapshot{
		Services: []string{"drive"},
		Accounts: []string{"acct"},
		Counts: map[string]int{
			"drive.contents":         1,
			"drive.contents.skipped": 1,
			"drive.contents.errors":  1,
		},
		Shards: []PlainShard{shard},
	}, testOptions(t, Options{ConfigPath: config, Push: false})); pushErr != nil {
		t.Fatalf("PushSnapshot: %v", pushErr)
	}

	verify, err := Verify(ctx, testOptions(t, Options{ConfigPath: config}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verify.Counts["drive.contents"] != 1 || verify.Counts["drive.contents.skipped"] != 1 || verify.Counts["drive.contents.errors"] != 1 {
		t.Fatalf("unexpected verify counts: %+v", verify.Counts)
	}
}

func TestCommitChangesIgnoresGlobalCommitSigning(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	globalConfig := filepath.Join(dir, "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)

	if err := os.WriteFile(globalConfig, []byte("[commit]\n\tgpgsign = true\n[gpg]\n\tprogram = false\n"), 0o600); err != nil {
		t.Fatalf("write git config: %v", err)
	}
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := git(ctx, repo, "init"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "manifest.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	changed, sha, err := commitChanges(ctx, Config{Repo: repo}, "test: signed config ignored")
	if err != nil {
		t.Fatalf("commitChanges: %v", err)
	}
	if !changed || strings.TrimSpace(sha) == "" {
		t.Fatalf("expected committed change, changed=%v sha=%q", changed, sha)
	}
}

func TestPushSnapshotEncryptsAndCleansPlaintextPath(t *testing.T) {
	ctx, _, config, _ := initTestBackup(t)
	tempPath := filepath.Join(t.TempDir(), "messages.jsonl")
	if err := os.WriteFile(tempPath, []byte("{\"id\":\"m1\",\"raw\":\"private\"}\n"), 0o600); err != nil {
		t.Fatalf("write plaintext path: %v", err)
	}
	shard := PlainShard{
		Service:       "gmail",
		Kind:          "messages",
		Account:       "acct",
		Path:          "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age",
		Rows:          1,
		PlaintextPath: tempPath,
	}
	if _, err := PushSnapshot(ctx, Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []PlainShard{shard},
	}, testOptions(t, Options{ConfigPath: config, Push: false})); err != nil {
		t.Fatalf("PushSnapshot: %v", err)
	}
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("plaintext temp file still exists or stat failed: %v", err)
	}
	verify, err := Verify(ctx, testOptions(t, Options{ConfigPath: config}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verify.Counts["gmail.messages"] != 1 {
		t.Fatalf("unexpected verify counts: %+v", verify.Counts)
	}
}

func TestPushCheckpointWritesIncompleteManifestOutsideMainSnapshot(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	shard, err := NewJSONLShard("gmail", "messages", "acct", "checkpoints/gmail/acct/run-one/messages/part-000001.jsonl.gz.age", []map[string]string{
		{"id": "m1", "raw": "private checkpoint body"},
	})
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	result, err := PushCheckpoint(ctx, Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []PlainShard{shard},
	}, Checkpoint{
		RunID:     "run-one",
		Service:   "gmail",
		Account:   "acct",
		Done:      1,
		Total:     2,
		Fetched:   1,
		CacheHits: 0,
	}, testOptions(t, Options{ConfigPath: config, Push: false}))
	if err != nil {
		t.Fatalf("PushCheckpoint: %v", err)
	}
	if !result.Changed || result.Shards != 1 || result.Counts["gmail.messages"] != 1 {
		t.Fatalf("unexpected checkpoint result: %+v", result)
	}
	if _, statErr := os.Stat(filepath.Join(repo, "manifest.json")); !os.IsNotExist(statErr) {
		t.Fatalf("main manifest should not be created by checkpoint: %v", statErr)
	}
	manifest, err := readCheckpointManifest(repo, "checkpoints/gmail/acct/run-one/manifest.json")
	if err != nil {
		t.Fatalf("readCheckpointManifest: %v", err)
	}
	if !manifest.Incomplete || manifest.Done != 1 || manifest.Total != 2 || manifest.RunID != "run-one" {
		t.Fatalf("unexpected checkpoint manifest: %+v", manifest)
	}
	ciphertext := readFile(t, filepath.Join(repo, "checkpoints", "gmail", "acct", "run-one", "messages", "part-000001.jsonl.gz.age"))
	if strings.Contains(string(ciphertext), "private checkpoint body") {
		t.Fatal("checkpoint shard contains plaintext")
	}

	pushSingleShard(t, ctx, config, mustGmailMessageShard(t, "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", []map[string]string{{"id": "m1", "raw": "final"}}))
	if _, err := os.Stat(filepath.Join(repo, "checkpoints", "gmail", "acct", "run-one", "messages", "part-000001.jsonl.gz.age")); err != nil {
		t.Fatalf("final snapshot removed checkpoint shard: %v", err)
	}
}

func TestPushSnapshotCanReferenceExistingCheckpointShard(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	checkpointShard := mustGmailMessageShard(t, "checkpoints/gmail/acct/run-one/messages/part-000001.jsonl.gz.age", []map[string]string{
		{"id": "m1", "raw": "checkpoint final"},
	})
	if _, err := PushCheckpoint(ctx, Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []PlainShard{checkpointShard},
	}, Checkpoint{RunID: "run-one", Service: "gmail", Account: "acct", Done: 1, Total: 1}, testOptions(t, Options{ConfigPath: config, Push: false})); err != nil {
		t.Fatalf("PushCheckpoint: %v", err)
	}
	checkpointManifest, err := readCheckpointManifest(repo, "checkpoints/gmail/acct/run-one/manifest.json")
	if err != nil {
		t.Fatalf("readCheckpointManifest: %v", err)
	}
	if _, err := PushSnapshot(ctx, Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []PlainShard{ExistingShard(checkpointManifest.Shards[0], checkpointManifest.Recipients)},
	}, testOptions(t, Options{ConfigPath: config, Push: false})); err != nil {
		t.Fatalf("PushSnapshot existing checkpoint shard: %v", err)
	}

	manifest := readTestManifest(t, repo)
	if len(manifest.Shards) != 1 || manifest.Shards[0].Path != checkpointManifest.Shards[0].Path {
		t.Fatalf("root manifest did not reference checkpoint shard: %+v", manifest.Shards)
	}
	if _, err := Verify(ctx, testOptions(t, Options{ConfigPath: config})); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if _, err := Cat(ctx, testOptions(t, Options{ConfigPath: config}), checkpointManifest.Shards[0].Path); err != nil {
		t.Fatalf("Cat checkpoint shard from root manifest: %v", err)
	}
}

func TestAsyncCheckpointPushDrainsBeforeFinalSnapshot(t *testing.T) {
	// Keep maintenance attached so temporary-repo cleanup cannot race its writes.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "maintenance.autoDetach")
	t.Setenv("GIT_CONFIG_VALUE_0", "false")
	ctx := context.Background()
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	remote := filepath.Join(dir, "remote.git")
	config := filepath.Join(dir, "backup.json")
	identity := filepath.Join(dir, "age.key")
	if err := git(ctx, "", "init", "--bare", "--initial-branch=main", remote); err != nil {
		t.Fatalf("init remote: %v", err)
	}
	// Local transport clears command-scope Git configuration for receive-pack.
	if err := git(ctx, remote, "config", "maintenance.autoDetach", "false"); err != nil {
		t.Fatalf("configure remote maintenance: %v", err)
	}
	recipient, err := EnsureIdentity(identity)
	if err != nil {
		t.Fatalf("EnsureIdentity: %v", err)
	}
	saveTestConfig(t, config, Config{Repo: repo, Remote: remote, Identity: identity, Recipients: []string{recipient}})
	var progressMu sync.Mutex
	var progress []string
	progressf := func(format string, args ...any) {
		progressMu.Lock()
		defer progressMu.Unlock()
		progress = append(progress, strings.TrimSpace(format))
	}
	checkpointShard := mustGmailMessageShard(t, "checkpoints/gmail/acct/run-one/messages/part-000001.jsonl.gz.age", []map[string]string{{"id": "m1"}})
	if _, pushErr := PushCheckpoint(ctx, Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []PlainShard{checkpointShard},
	}, Checkpoint{RunID: "run-one", Service: "gmail", Account: "acct", Done: 1, Total: 2}, testOptions(t, Options{ConfigPath: config, Push: true, AsyncPush: true, Progress: progressf})); pushErr != nil {
		t.Fatalf("PushCheckpoint: %v", pushErr)
	}
	finalShard := mustGmailMessageShard(t, "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", []map[string]string{{"id": "m1"}})
	if _, pushErr := PushSnapshot(ctx, Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []PlainShard{finalShard},
	}, testOptions(t, Options{ConfigPath: config, Push: true, Progress: progressf})); pushErr != nil {
		t.Fatalf("PushSnapshot: %v", pushErr)
	}
	local, err := gitOutput(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("local HEAD: %v", err)
	}
	remoteHead, err := gitOutput(ctx, repo, "ls-remote", "origin", "refs/heads/main")
	if err != nil {
		t.Fatalf("remote HEAD: %v", err)
	}
	if !strings.HasPrefix(remoteHead, strings.TrimSpace(local)) {
		t.Fatalf("remote HEAD = %q, want local %q", remoteHead, local)
	}
	progressMu.Lock()
	gotProgress := append([]string(nil), progress...)
	progressMu.Unlock()
	if !containsProgress(gotProgress, "backup git push") {
		t.Fatalf("missing async push progress: %#v", gotProgress)
	}
}

func TestCommitAndPushRemovesInterruptedShardTemps(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	temp := filepath.Join(repo, "checkpoints", "gmail", "acct", "run-one", "messages", ".shard-interrupted.age")
	if err := os.MkdirAll(filepath.Dir(temp), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(temp, []byte("partial ciphertext"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pushSingleShard(t, ctx, config, mustGmailMessageShard(t, "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", []map[string]string{{"id": "m1", "raw": "final"}}))
	if _, err := os.Stat(temp); !os.IsNotExist(err) {
		t.Fatalf("temp shard should be removed before commit: %v", err)
	}
	if err := git(ctx, repo, "ls-files", "--error-unmatch", "checkpoints/gmail/acct/run-one/messages/.shard-interrupted.age"); err == nil {
		t.Fatal("temp shard was committed")
	}
}

func TestCatAndDecryptSnapshotVerifyPlaintext(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	shardPath := "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age"
	pushSingleShard(t, ctx, config, mustGmailMessageShard(t, shardPath, []map[string]string{{
		"id":  "m1",
		"raw": "plain marker",
	}}))

	cat, err := Cat(ctx, testOptions(t, Options{ConfigPath: config}), shardPath)
	if err != nil {
		t.Fatalf("Cat: %v", err)
	}
	if cat.Path != shardPath || cat.Service != "gmail" || cat.Kind != "messages" || !strings.Contains(string(cat.Plaintext), "plain marker") {
		t.Fatalf("unexpected cat shard: %+v plaintext=%q", cat, cat.Plaintext)
	}

	absPath := filepath.Join(repo, filepath.FromSlash(shardPath))
	catAbs, err := Cat(ctx, testOptions(t, Options{ConfigPath: config}), absPath)
	if err != nil {
		t.Fatalf("Cat absolute: %v", err)
	}
	if string(catAbs.Plaintext) != string(cat.Plaintext) {
		t.Fatalf("absolute Cat plaintext mismatch")
	}

	manifest, gotRepo, shards, err := DecryptSnapshot(ctx, testOptions(t, Options{ConfigPath: config}))
	if err != nil {
		t.Fatalf("DecryptSnapshot: %v", err)
	}
	if gotRepo != repo || len(manifest.Shards) != 1 || len(shards) != 1 || string(shards[0].Plaintext) != string(cat.Plaintext) {
		t.Fatalf("unexpected decrypt snapshot repo=%s manifest=%+v shards=%+v", gotRepo, manifest, shards)
	}
}

func TestCatRejectsShardOutsideManifest(t *testing.T) {
	ctx, _, config, _ := initTestBackup(t)
	pushSingleShard(t, ctx, config, mustGmailMessageShard(t, "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", []map[string]string{{"id": "m1"}}))

	for _, ref := range []string{"../data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", "data/gmail/acct/messages/2026/05/part-0001.jsonl.gz.age"} {
		t.Run(ref, func(t *testing.T) {
			if _, err := Cat(ctx, testOptions(t, Options{ConfigPath: config}), ref); err == nil {
				t.Fatal("expected Cat to reject missing or escaping shard")
			}
		})
	}
}

func TestIdentityAndConfigArePrivate(t *testing.T) {
	_, _, config, identity := initTestBackup(t)

	for _, path := range []string{config, identity} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			got := info.Mode().Perm()
			t.Fatalf("%s mode = %v, want 0600", path, got)
		}
	}

	data, err := os.ReadFile(identity)
	if err != nil {
		t.Fatalf("read identity: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(data)), "AGE-SECRET-KEY-") {
		t.Fatalf("identity does not look like an age secret key")
	}
}

func TestManifestDoesNotContainPayloadPlaintext(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	shard := mustGmailMessageShard(t, "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", []map[string]string{{
		"id":      "msg-plain-marker",
		"subject": "very secret subject marker",
		"raw":     "private raw mime marker",
	}})

	if _, err := PushSnapshot(ctx, Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []PlainShard{shard},
	}, testOptions(t, Options{ConfigPath: config, Push: false})); err != nil {
		t.Fatalf("PushSnapshot: %v", err)
	}

	for _, name := range []string{"manifest.json", "README.md"} {
		data, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(data)
		for _, marker := range []string{"msg-plain-marker", "very secret subject marker", "private raw mime marker"} {
			if strings.Contains(text, marker) {
				t.Fatalf("%s contains private payload marker %q", name, marker)
			}
		}
	}
}

func TestVerifyDetectsTamperedCiphertext(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	shardPath := "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age"
	pushSingleShard(t, ctx, config, mustGmailMessageShard(t, shardPath, []map[string]string{{"id": "m1", "raw": "body"}}))

	path := filepath.Join(repo, filepath.FromSlash(shardPath))
	ciphertext, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ciphertext: %v", err)
	}
	ciphertext[len(ciphertext)-1] ^= 0xff
	if err := os.WriteFile(path, ciphertext, 0o600); err != nil {
		t.Fatalf("write tampered ciphertext: %v", err)
	}

	if _, err := Verify(ctx, testOptions(t, Options{ConfigPath: config})); err == nil {
		t.Fatal("expected verify to reject tampered ciphertext")
	}
}

func TestVerifyDetectsManifestHashMismatch(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	pushSingleShard(t, ctx, config, mustGmailMessageShard(t, "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", []map[string]string{{"id": "m1", "raw": "body"}}))

	manifest := readTestManifest(t, repo)
	manifest.Shards[0].SHA256 = strings.Repeat("0", 64)
	writeTestManifest(t, repo, manifest)
	commitTestRepo(t, ctx, repo, "test: tamper manifest hash")

	_, err := Verify(ctx, testOptions(t, Options{ConfigPath: config}))
	if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("Verify error = %v, want hash mismatch", err)
	}
}

func TestVerifyDetectsManifestRowCountMismatch(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	pushSingleShard(t, ctx, config, mustGmailMessageShard(t, "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age", []map[string]string{{"id": "m1", "raw": "body"}}))

	manifest := readTestManifest(t, repo)
	manifest.Shards[0].Rows = 2
	writeTestManifest(t, repo, manifest)
	commitTestRepo(t, ctx, repo, "test: tamper manifest rows")

	_, err := Verify(ctx, testOptions(t, Options{ConfigPath: config}))
	if err == nil || !strings.Contains(err.Error(), "row count mismatch") {
		t.Fatalf("Verify error = %v, want row count mismatch", err)
	}
}

func TestJSONLHelpersHandleLargeRows(t *testing.T) {
	large := strings.Repeat("x", 17*1024*1024)
	plaintext := []byte(`{"id":"large","raw":"` + large + "\"}\n")
	rows := countJSONLLines(plaintext)
	if rows != 1 {
		t.Fatalf("rows = %d, want 1", rows)
	}
	var decoded []map[string]string
	if err := DecodeJSONL(plaintext, &decoded); err != nil {
		t.Fatalf("DecodeJSONL: %v", err)
	}
	if len(decoded) != 1 || decoded[0]["raw"] != large {
		t.Fatalf("decoded large row mismatch: len=%d", len(decoded))
	}
}

func TestPushReusesEncryptedShardWhenPlaintextAndRecipientsMatch(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	shardPath := "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age"
	shard := mustGmailMessageShard(t, shardPath, []map[string]string{{"id": "m1", "raw": "body"}})

	first := pushSingleShard(t, ctx, config, shard)
	firstCiphertext := readFile(t, filepath.Join(repo, filepath.FromSlash(shardPath)))
	second := pushSingleShard(t, ctx, config, shard)
	secondCiphertext := readFile(t, filepath.Join(repo, filepath.FromSlash(shardPath)))

	if !first.Changed {
		t.Fatalf("first push changed = false, want true")
	}
	if second.Changed {
		t.Fatalf("second push changed = true, want false")
	}
	if string(firstCiphertext) != string(secondCiphertext) {
		t.Fatalf("ciphertext changed even though plaintext and recipients matched")
	}
}

func TestPushReencryptsShardWhenRecipientChanges(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	shardPath := "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age"
	shard := mustGmailMessageShard(t, shardPath, []map[string]string{{"id": "m1", "raw": "body"}})
	pushSingleShard(t, ctx, config, shard)
	firstCiphertext := readFile(t, filepath.Join(repo, filepath.FromSlash(shardPath)))

	secondIdentity := filepath.Join(t.TempDir(), "age.key")
	secondRecipient, err := EnsureIdentity(secondIdentity)
	if err != nil {
		t.Fatalf("EnsureIdentity second: %v", err)
	}
	if _, err := PushSnapshot(ctx, Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []PlainShard{shard},
	}, testOptions(t, Options{ConfigPath: config, Identity: secondIdentity, Recipients: []string{secondRecipient}, Push: false})); err != nil {
		t.Fatalf("PushSnapshot second recipient: %v", err)
	}
	secondCiphertext := readFile(t, filepath.Join(repo, filepath.FromSlash(shardPath)))
	if string(firstCiphertext) == string(secondCiphertext) {
		t.Fatal("ciphertext did not change after recipient rotation")
	}
	if _, err := Verify(ctx, testOptions(t, Options{ConfigPath: config, Identity: secondIdentity})); err != nil {
		t.Fatalf("Verify with rotated identity: %v", err)
	}
}

func TestPushRemovesStaleEncryptedShards(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	oldPath := "data/gmail/acct/messages/2026/03/part-0001.jsonl.gz.age"
	keepPath := "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age"
	oldShard := mustGmailMessageShard(t, oldPath, []map[string]string{{"id": "old"}})
	keepShard := mustGmailMessageShard(t, keepPath, []map[string]string{{"id": "keep"}})

	if _, err := PushSnapshot(ctx, Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"gmail.messages": 2},
		Shards:   []PlainShard{oldShard, keepShard},
	}, testOptions(t, Options{ConfigPath: config, Push: false})); err != nil {
		t.Fatalf("initial PushSnapshot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(oldPath))); err != nil {
		t.Fatalf("old shard should exist before pruning: %v", err)
	}

	pushSingleShard(t, ctx, config, keepShard)
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(oldPath))); !os.IsNotExist(err) {
		t.Fatalf("old shard still exists after pruning: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(keepPath))); err != nil {
		t.Fatalf("kept shard missing after pruning: %v", err)
	}
}

func TestPushPreservesUntouchedServices(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	gmailPath := "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age"
	calendarPath := "data/calendar/acct/events/part-0001.jsonl.gz.age"
	gmailShard := mustGmailMessageShard(t, gmailPath, []map[string]string{{"id": "m1", "raw": "body"}})
	calendarShard, err := NewJSONLShard("calendar", "events", "acct", calendarPath, []map[string]string{{"id": "event1"}})
	if err != nil {
		t.Fatalf("NewJSONLShard calendar: %v", err)
	}
	pushSingleShard(t, ctx, config, gmailShard)
	if _, err := PushSnapshot(ctx, Snapshot{
		Services: []string{"calendar"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"calendar.events": 1},
		Shards:   []PlainShard{calendarShard},
	}, testOptions(t, Options{ConfigPath: config, Push: false})); err != nil {
		t.Fatalf("PushSnapshot calendar: %v", err)
	}

	manifest := readTestManifest(t, repo)
	if _, ok := manifest.entry(gmailPath); !ok {
		t.Fatal("gmail shard was removed by calendar-only push")
	}
	if _, ok := manifest.entry(calendarPath); !ok {
		t.Fatal("calendar shard missing")
	}
	if manifest.Counts["gmail.messages"] != 1 || manifest.Counts["calendar.events"] != 1 {
		t.Fatalf("counts = %+v, want preserved gmail and new calendar", manifest.Counts)
	}
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(gmailPath))); err != nil {
		t.Fatalf("gmail shard file missing: %v", err)
	}
}

func TestPushSnapshotRejectsCorruptManifestWithoutPruning(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	gmailPath := "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age"
	calendarPath := "data/calendar/acct/events/part-0001.jsonl.gz.age"
	gmailShard := mustGmailMessageShard(t, gmailPath, []map[string]string{{"id": "m1", "raw": "body"}})
	calendarShard, err := NewJSONLShard("calendar", "events", "acct", calendarPath, []map[string]string{{"id": "event1"}})
	if err != nil {
		t.Fatalf("NewJSONLShard calendar: %v", err)
	}
	pushSingleShard(t, ctx, config, gmailShard)

	manifestPath := filepath.Join(repo, "manifest.json")
	err = os.WriteFile(manifestPath, []byte("{"), 0o600)
	if err != nil {
		t.Fatalf("corrupt manifest: %v", err)
	}

	_, err = PushSnapshot(ctx, Snapshot{
		Services: []string{"calendar"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"calendar.events": 1},
		Shards:   []PlainShard{calendarShard},
	}, testOptions(t, Options{ConfigPath: config, Push: false}))
	if err == nil {
		t.Fatal("expected corrupt manifest error")
	}
	if _, statErr := os.Stat(filepath.Join(repo, filepath.FromSlash(gmailPath))); statErr != nil {
		t.Fatalf("existing shard was pruned after corrupt manifest: %v", statErr)
	}
}

func TestPushSnapshotRejectsUnsupportedManifestWithoutPruning(t *testing.T) {
	ctx, repo, config, _ := initTestBackup(t)
	gmailPath := "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age"
	calendarPath := "data/calendar/acct/events/part-0001.jsonl.gz.age"
	gmailShard := mustGmailMessageShard(t, gmailPath, []map[string]string{{"id": "m1", "raw": "body"}})
	calendarShard, err := NewJSONLShard("calendar", "events", "acct", calendarPath, []map[string]string{{"id": "event1"}})
	if err != nil {
		t.Fatalf("NewJSONLShard calendar: %v", err)
	}
	pushSingleShard(t, ctx, config, gmailShard)

	manifest := readTestManifest(t, repo)
	manifest.Format = formatVersion + 1
	err = writeManifest(repo, manifest)
	if err != nil {
		t.Fatalf("write unsupported manifest: %v", err)
	}

	_, err = PushSnapshot(ctx, Snapshot{
		Services: []string{"calendar"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"calendar.events": 1},
		Shards:   []PlainShard{calendarShard},
	}, testOptions(t, Options{ConfigPath: config, Push: false}))
	if err == nil {
		t.Fatal("expected unsupported manifest error")
	}
	if _, statErr := os.Stat(filepath.Join(repo, filepath.FromSlash(gmailPath))); statErr != nil {
		t.Fatalf("existing shard was pruned after unsupported manifest: %v", statErr)
	}
}

func TestPushSnapshotRejectsSymlinkManifestBeforeMutatingShards(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	ctx, repo, config, _ := initTestBackup(t)
	gmailPath := "data/gmail/acct/messages/2026/04/part-0001.jsonl.gz.age"
	calendarPath := "data/calendar/acct/events/part-0001.jsonl.gz.age"
	gmailShard := mustGmailMessageShard(t, gmailPath, []map[string]string{{"id": "m1", "raw": "body"}})
	calendarShard, err := NewJSONLShard("calendar", "events", "acct", calendarPath, []map[string]string{{"id": "event1"}})
	if err != nil {
		t.Fatalf("NewJSONLShard calendar: %v", err)
	}
	pushSingleShard(t, ctx, config, gmailShard)

	manifestPath := filepath.Join(repo, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	outsideManifest := filepath.Join(t.TempDir(), "manifest.json")
	if writeErr := os.WriteFile(outsideManifest, manifestData, 0o600); writeErr != nil {
		t.Fatalf("write outside manifest: %v", writeErr)
	}
	if removeErr := os.Remove(manifestPath); removeErr != nil {
		t.Fatalf("remove manifest: %v", removeErr)
	}
	if symlinkErr := os.Symlink(outsideManifest, manifestPath); symlinkErr != nil {
		t.Fatalf("symlink manifest: %v", symlinkErr)
	}

	_, err = PushSnapshot(ctx, Snapshot{
		Services: []string{"calendar"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"calendar.events": 1},
		Shards:   []PlainShard{calendarShard},
	}, testOptions(t, Options{ConfigPath: config, Push: false}))
	if err == nil {
		t.Fatal("expected symlink manifest error")
	}
	if _, statErr := os.Stat(filepath.Join(repo, filepath.FromSlash(gmailPath))); statErr != nil {
		t.Fatalf("existing shard was pruned after symlink manifest: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(repo, filepath.FromSlash(calendarPath))); !os.IsNotExist(statErr) {
		t.Fatalf("new shard was written before symlink manifest rejection: %v", statErr)
	}
}

func TestRejectsInvalidShardPaths(t *testing.T) {
	_, _, config, _ := initTestBackup(t)
	for _, rel := range []string{
		"../nope.age",
		"/tmp/nope.age",
		"manifest.age",
		"data/gmail/acct/plain.jsonl",
		"data/../nope.age",
	} {
		t.Run(rel, func(t *testing.T) {
			shard := mustGmailMessageShard(t, rel, []map[string]string{{"id": "m1"}})
			_, err := PushSnapshot(context.Background(), Snapshot{Shards: []PlainShard{shard}}, testOptions(t, Options{
				ConfigPath: config,
				Push:       false,
			}))
			if err == nil {
				t.Fatal("expected invalid shard path error")
			}
		})
	}
}

func TestRejectsSymlinkedShardPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	ctx, repo, config, _ := initTestBackup(t)
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "data"), 0o700); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "data", "gmail")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	rel := "data/gmail/acct/messages/part-0001.jsonl.gz.age"
	shard := mustGmailMessageShard(t, rel, []map[string]string{{"id": "m1", "raw": "body"}})
	_, err := PushSnapshot(ctx, Snapshot{Shards: []PlainShard{shard}}, testOptions(t, Options{
		ConfigPath: config,
		Push:       false,
	}))
	if err == nil {
		t.Fatal("expected symlink path error")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "acct")); !os.IsNotExist(statErr) {
		t.Fatalf("outside symlink target was modified: %v", statErr)
	}
}

func TestRejectsSymlinkedShardPathOnReuse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	ctx, repo, config, _ := initTestBackup(t)
	rel := "data/gmail/acct/messages/part-0001.jsonl.gz.age"
	shard := mustGmailMessageShard(t, rel, []map[string]string{{"id": "m1", "raw": "body"}})
	pushSingleShard(t, ctx, config, shard)

	shardPath := filepath.Join(repo, filepath.FromSlash(rel))
	outside := filepath.Join(t.TempDir(), "part-0001.jsonl.gz.age")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside shard: %v", err)
	}
	if err := os.Remove(shardPath); err != nil {
		t.Fatalf("remove shard: %v", err)
	}
	if err := os.Symlink(outside, shardPath); err != nil {
		t.Fatalf("symlink shard: %v", err)
	}

	_, err := PushSnapshot(ctx, Snapshot{
		Services: []string{"gmail"},
		Accounts: []string{"acct"},
		Counts:   map[string]int{"gmail.messages": 1},
		Shards:   []PlainShard{shard},
	}, testOptions(t, Options{ConfigPath: config, Push: false}))
	if err == nil {
		t.Fatal("expected symlink reuse error")
	}
}

func TestWriteManifestRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	repo := t.TempDir()
	outside := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatalf("seed outside manifest: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "manifest.json")); err != nil {
		t.Fatalf("symlink manifest: %v", err)
	}

	err := writeManifest(repo, Manifest{Format: formatVersion, App: "gog", Encrypted: true})
	if err == nil {
		t.Fatal("expected symlink manifest error")
	}
	got, readErr := os.ReadFile(outside)
	if readErr != nil {
		t.Fatalf("read outside manifest: %v", readErr)
	}
	if string(got) != "keep" {
		t.Fatalf("outside manifest was overwritten: %q", got)
	}
}

func TestWriteBackupReadmeRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	repo := t.TempDir()
	outside := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatalf("seed outside readme: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "README.md")); err != nil {
		t.Fatalf("symlink readme: %v", err)
	}

	err := writeBackupReadme(repo)
	if err == nil {
		t.Fatal("expected symlink readme error")
	}
	got, readErr := os.ReadFile(outside)
	if readErr != nil {
		t.Fatalf("read outside readme: %v", readErr)
	}
	if string(got) != "keep" {
		t.Fatalf("outside readme was overwritten: %q", got)
	}
}

func TestEncryptDecryptRoundTripMultipleRecipients(t *testing.T) {
	dir := t.TempDir()
	firstIdentity := filepath.Join(dir, "first.age")
	firstRecipient, err := EnsureIdentity(firstIdentity)
	if err != nil {
		t.Fatalf("EnsureIdentity first: %v", err)
	}
	secondIdentity := filepath.Join(dir, "second.age")
	secondRecipient, err := EnsureIdentity(secondIdentity)
	if err != nil {
		t.Fatalf("EnsureIdentity second: %v", err)
	}

	encrypted, hash, err := encryptShard([]byte("secret jsonl\n"), []string{firstRecipient, secondRecipient})
	if err != nil {
		t.Fatalf("encryptShard: %v", err)
	}
	if hash != sha256Hex([]byte("secret jsonl\n")) {
		t.Fatalf("hash = %s, want plaintext sha256", hash)
	}
	for _, identity := range []string{firstIdentity, secondIdentity} {
		plaintext, err := decryptShard(encrypted, identity)
		if err != nil {
			t.Fatalf("decryptShard %s: %v", identity, err)
		}
		if string(plaintext) != "secret jsonl\n" {
			t.Fatalf("plaintext = %q", plaintext)
		}
	}
}

func initTestBackup(t *testing.T) (context.Context, string, string, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	identity := filepath.Join(dir, "age.key")
	config := filepath.Join(dir, "backup.json")

	recipient, err := EnsureIdentity(identity)
	if err != nil {
		t.Fatalf("EnsureIdentity: %v", err)
	}
	cfg := Config{
		Repo:       repo,
		Identity:   identity,
		Recipients: []string{recipient},
	}
	saveTestConfig(t, config, cfg)
	if err := ensureRepo(ctx, cfg); err != nil {
		t.Fatalf("ensureRepo: %v", err)
	}
	if err := writeBackupReadme(repo); err != nil {
		t.Fatalf("writeBackupReadme: %v", err)
	}
	if _, err := commitAndPush(ctx, cfg, "docs: describe encrypted gog backup", false); err != nil {
		t.Fatalf("commitAndPush: %v", err)
	}
	if cfg.Repo != repo || !strings.HasPrefix(recipient, "age1") {
		t.Fatalf("unexpected init cfg=%+v recipient=%q", cfg, recipient)
	}
	return ctx, repo, config, identity
}

func mustGmailMessageShard(t *testing.T, rel string, rows any) PlainShard {
	t.Helper()
	shard, err := NewJSONLShard("gmail", "messages", "acct", rel, rows)
	if err != nil {
		t.Fatalf("NewJSONLShard: %v", err)
	}
	return shard
}

func pushSingleShard(t *testing.T, ctx context.Context, config string, shard PlainShard) Result {
	t.Helper()
	result, err := PushSnapshot(ctx, Snapshot{
		Services: []string{shard.Service},
		Accounts: []string{shard.Account},
		Counts:   map[string]int{shard.Service + "." + shard.Kind: shard.Rows},
		Shards:   []PlainShard{shard},
	}, testOptions(t, Options{ConfigPath: config, Push: false}))
	if err != nil {
		t.Fatalf("PushSnapshot: %v", err)
	}
	return result
}

func containsProgress(lines []string, want string) bool {
	for _, line := range lines {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}

func readTestManifest(t *testing.T, repo string) Manifest {
	t.Helper()
	data := readFile(t, filepath.Join(repo, "manifest.json"))
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	return manifest
}

func writeTestManifest(t *testing.T, repo string, manifest Manifest) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(repo, "manifest.json"), data, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func commitTestRepo(t *testing.T, ctx context.Context, repo, message string) {
	t.Helper()
	if err := git(ctx, repo, "add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := git(ctx, repo, "-c", "commit.gpgsign=false", "commit", "-m", message); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
