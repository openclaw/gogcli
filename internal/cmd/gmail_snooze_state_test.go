package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestSnoozeStore creates a snoozeStore pointing at a temp directory,
// bypassing config.EnsureGmailSnoozeDir so tests are fully self-contained.
func newTestSnoozeStore(t *testing.T) *snoozeStore {
	t.Helper()
	dir := t.TempDir()
	return &snoozeStore{path: filepath.Join(dir, "test.json")}
}

func TestSnoozeStoreUpsert(t *testing.T) {
	s := newTestSnoozeStore(t)

	e1 := snoozeEntry{ThreadID: "thread1", Account: "a@example.com", UntilMs: 1000, CreatedAtMs: 500}
	e2 := snoozeEntry{ThreadID: "thread2", Account: "a@example.com", UntilMs: 2000, CreatedAtMs: 600}

	if err := s.Upsert(e1); err != nil {
		t.Fatalf("Upsert e1: %v", err)
	}
	if err := s.Upsert(e2); err != nil {
		t.Fatalf("Upsert e2: %v", err)
	}

	got := s.Get()
	if len(got.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got.Entries))
	}
}

func TestSnoozeStoreUpsertReplace(t *testing.T) {
	s := newTestSnoozeStore(t)

	e1 := snoozeEntry{ThreadID: "thread1", Account: "a@example.com", UntilMs: 1000, Subject: "original"}
	if err := s.Upsert(e1); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	e1Updated := snoozeEntry{ThreadID: "thread1", Account: "a@example.com", UntilMs: 9999, Subject: "updated"}
	if err := s.Upsert(e1Updated); err != nil {
		t.Fatalf("Upsert updated: %v", err)
	}

	got := s.Get()
	if len(got.Entries) != 1 {
		t.Fatalf("expected 1 entry after upsert-replace, got %d", len(got.Entries))
	}
	if got.Entries[0].Subject != "updated" {
		t.Errorf("expected subject 'updated', got %q", got.Entries[0].Subject)
	}
	if got.Entries[0].UntilMs != 9999 {
		t.Errorf("expected UntilMs 9999, got %d", got.Entries[0].UntilMs)
	}
}

func TestSnoozeStoreRemove(t *testing.T) {
	s := newTestSnoozeStore(t)

	e1 := snoozeEntry{ThreadID: "thread1", Account: "a@example.com", UntilMs: 1000}
	e2 := snoozeEntry{ThreadID: "thread2", Account: "a@example.com", UntilMs: 2000}

	if err := s.Upsert(e1); err != nil {
		t.Fatalf("Upsert e1: %v", err)
	}
	if err := s.Upsert(e2); err != nil {
		t.Fatalf("Upsert e2: %v", err)
	}

	if err := s.Remove("thread1", "a@example.com"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	got := s.Get()
	if len(got.Entries) != 1 {
		t.Fatalf("expected 1 entry after remove, got %d", len(got.Entries))
	}
	if got.Entries[0].ThreadID != "thread2" {
		t.Errorf("expected remaining entry to be thread2, got %q", got.Entries[0].ThreadID)
	}
}

func TestSnoozeStoreDueEntries(t *testing.T) {
	s := newTestSnoozeStore(t)

	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	past := now.Add(-1 * time.Hour).UnixMilli()
	future := now.Add(1 * time.Hour).UnixMilli()

	pastEntry := snoozeEntry{ThreadID: "past", Account: "a@example.com", UntilMs: past}
	futureEntry := snoozeEntry{ThreadID: "future", Account: "a@example.com", UntilMs: future}
	otherAccount := snoozeEntry{ThreadID: "other", Account: "b@example.com", UntilMs: past}

	for _, e := range []snoozeEntry{pastEntry, futureEntry, otherAccount} {
		if err := s.Upsert(e); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	// Due entries for account a
	due := s.DueEntries(now, "a@example.com")
	if len(due) != 1 {
		t.Fatalf("expected 1 due entry for a@example.com, got %d", len(due))
	}
	if due[0].ThreadID != "past" {
		t.Errorf("expected 'past' thread, got %q", due[0].ThreadID)
	}

	// Due entries for all accounts (empty account)
	dueAll := s.DueEntries(now, "")
	if len(dueAll) != 2 {
		t.Fatalf("expected 2 due entries across all accounts, got %d", len(dueAll))
	}
}

func TestSnoozeStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snooze.json")

	s1 := &snoozeStore{path: path}
	entry := snoozeEntry{
		ThreadID:       "roundtrip-thread",
		Subject:        "Hello round trip",
		Account:        "user@example.com",
		UntilMs:        123456789,
		SnoozedLabelID: "Label_42",
		CreatedAtMs:    987654321,
	}
	if err := s1.Upsert(entry); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Load from the same path via a fresh store instance — read and unmarshal manually
	// to mirror what loadSnoozeStore does without touching the filesystem config dir.
	s2 := &snoozeStore{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := json.Unmarshal(data, &s2.state); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	got := s2.Get()
	if len(got.Entries) != 1 {
		t.Fatalf("expected 1 entry after round-trip, got %d", len(got.Entries))
	}
	e := got.Entries[0]
	if e.ThreadID != entry.ThreadID {
		t.Errorf("ThreadID: got %q, want %q", e.ThreadID, entry.ThreadID)
	}
	if e.Subject != entry.Subject {
		t.Errorf("Subject: got %q, want %q", e.Subject, entry.Subject)
	}
	if e.UntilMs != entry.UntilMs {
		t.Errorf("UntilMs: got %d, want %d", e.UntilMs, entry.UntilMs)
	}
	if e.SnoozedLabelID != entry.SnoozedLabelID {
		t.Errorf("SnoozedLabelID: got %q, want %q", e.SnoozedLabelID, entry.SnoozedLabelID)
	}
}
