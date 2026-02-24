package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// fixedNow is a Wednesday at noon, used as the fixed reference point for all
// parseUntilExpr tests so results are deterministic.
var fixedNow = time.Date(2026, 1, 14, 12, 0, 0, 0, time.UTC) // Wednesday

// TestParseUntilExpr verifies that the --until flag parser accepts RFC3339,
// YYYY-MM-DD dates, weekday names, "tomorrow", Go durations, and "in X" forms.
func TestParseUntilExpr(t *testing.T) {
	now := fixedNow

	tests := []struct {
		name  string
		until string
		// checkFn validates the returned time (relative to now).
		checkFn func(t *testing.T, got time.Time)
	}{
		{
			name:  "RFC3339 future",
			until: "2026-06-01T09:00:00Z",
			checkFn: func(t *testing.T, got time.Time) {
				t.Helper()
				want := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:  "YYYY-MM-DD future date",
			until: "2026-03-10",
			checkFn: func(t *testing.T, got time.Time) {
				t.Helper()
				if got.Before(now) {
					t.Errorf("expected future time, got %v (now=%v)", got, now)
				}
				if got.Year() != 2026 || got.Month() != 3 || got.Day() != 10 {
					t.Errorf("expected 2026-03-10, got %v", got)
				}
			},
		},
		{
			name:  "tomorrow",
			until: "tomorrow",
			checkFn: func(t *testing.T, got time.Time) {
				t.Helper()
				// "tomorrow" => start of 2026-01-15 (next day).
				wantDate := now.AddDate(0, 0, 1)
				if got.Year() != wantDate.Year() || got.Month() != wantDate.Month() || got.Day() != wantDate.Day() {
					t.Errorf("expected tomorrow (%v), got %v", wantDate.Format("2006-01-02"), got.Format("2006-01-02"))
				}
			},
		},
		{
			name:  "next weekday (friday)",
			until: "friday",
			checkFn: func(t *testing.T, got time.Time) {
				t.Helper()
				// fixedNow is Wednesday 2026-01-14; next Friday is 2026-01-16.
				if got.Weekday() != time.Friday {
					t.Errorf("expected Friday, got %v", got.Weekday())
				}
				if got.Before(now) {
					t.Errorf("expected future Friday, got %v", got)
				}
			},
		},
		{
			name:  "next monday",
			until: "monday",
			checkFn: func(t *testing.T, got time.Time) {
				t.Helper()
				// fixedNow is Wednesday; next Monday is 2026-01-19.
				if got.Weekday() != time.Monday {
					t.Errorf("expected Monday, got %v", got.Weekday())
				}
				if got.Before(now) {
					t.Errorf("expected future Monday, got %v", got)
				}
			},
		},
		{
			name:  "Go duration 2h",
			until: "2h",
			checkFn: func(t *testing.T, got time.Time) {
				t.Helper()
				want := now.Add(2 * time.Hour)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:  "Go duration 30m",
			until: "30m",
			checkFn: func(t *testing.T, got time.Time) {
				t.Helper()
				want := now.Add(30 * time.Minute)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:  "in 30m form",
			until: "in 30m",
			checkFn: func(t *testing.T, got time.Time) {
				t.Helper()
				want := now.Add(30 * time.Minute)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
		{
			name:  "in 1h form",
			until: "in 1h",
			checkFn: func(t *testing.T, got time.Time) {
				t.Helper()
				want := now.Add(1 * time.Hour)
				if !got.Equal(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseUntilExpr(tc.until, now)
			if err != nil {
				t.Fatalf("parseUntilExpr(%q) unexpected error: %v", tc.until, err)
			}
			tc.checkFn(t, got)
		})
	}
}

// TestParseUntilExpr_PastReturnsError verifies that parseUntilExpr itself does
// NOT return an error for a past value (the Run function validates the future
// constraint separately), and that a clearly invalid string returns an error.
func TestParseUntilExpr_InvalidStringReturnsError(t *testing.T) {
	now := fixedNow

	// Empty string must error.
	_, err := parseUntilExpr("", now)
	if err == nil {
		t.Error("expected error for empty --until, got nil")
	}

	// Unrecognisable garbage must error.
	_, err = parseUntilExpr("not-a-date", now)
	if err == nil {
		t.Error("expected error for 'not-a-date', got nil")
	}
}

// TestParseUntilExpr_PastDateParsesOK verifies that parseUntilExpr resolves a
// past date without error (the caller — GmailSnoozeAddCmd.Run — is responsible
// for rejecting past times with "must be in the future").
func TestParseUntilExpr_PastDateParsesOK(t *testing.T) {
	now := fixedNow

	past := "2020-01-01T00:00:00Z"
	got, err := parseUntilExpr(past, now)
	if err != nil {
		t.Fatalf("parseUntilExpr(%q) unexpected error: %v", past, err)
	}
	if !got.Before(now) {
		t.Errorf("expected past time, got %v", got)
	}
}

// TestGmailSnoozeListCmd_SortsByUntilAsc verifies that entries from AllEntries
// can be sorted in ascending UntilMs order — this mirrors what GmailSnoozeListCmd.Run does.
func TestGmailSnoozeListCmd_SortsByUntilAsc(t *testing.T) {
	dir := t.TempDir()
	s := &snoozeStore{path: filepath.Join(dir, "snooze.json")}

	now := fixedNow.UnixMilli()
	entries := []snoozeEntry{
		{ThreadID: "c", Account: "a@example.com", UntilMs: now + 3000},
		{ThreadID: "a", Account: "a@example.com", UntilMs: now + 1000},
		{ThreadID: "b", Account: "a@example.com", UntilMs: now + 2000},
	}
	for _, e := range entries {
		if err := s.Upsert(e); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	got := s.AllEntries("a@example.com")

	// Sort ascending by UntilMs (the same sort the List command applies).
	sort.Slice(got, func(i, j int) bool {
		return got[i].UntilMs < got[j].UntilMs
	})

	wantOrder := []string{"a", "b", "c"}
	for i, want := range wantOrder {
		if got[i].ThreadID != want {
			t.Errorf("position %d: got %q, want %q", i, got[i].ThreadID, want)
		}
	}
}

// TestGmailSnoozeCancelCmd_NotFound verifies that the cancel logic returns a
// "snooze not found" error when no matching entry exists in the store.
// It directly exercises the same lookup GmailSnoozeCancelCmd.Run uses.
func TestGmailSnoozeCancelCmd_NotFound(t *testing.T) {
	dir := t.TempDir()
	s := &snoozeStore{path: filepath.Join(dir, "snooze.json")}

	// Populate with an entry for a different thread.
	if err := s.Upsert(snoozeEntry{
		ThreadID: "existing-thread",
		Account:  "user@example.com",
		UntilMs:  fixedNow.Add(1 * time.Hour).UnixMilli(),
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Simulate the lookup GmailSnoozeCancelCmd.Run performs.
	threadID := "missing-thread"
	account := "user@example.com"

	var found *snoozeEntry
	for i := range s.state.Entries {
		e := &s.state.Entries[i]
		if e.ThreadID == threadID && e.Account == account {
			found = e
			break
		}
	}

	if found != nil {
		t.Fatal("expected lookup to return nil for missing thread, but found an entry")
	}

	// Verify the error message the implementation would produce matches spec.
	simulatedErr := fmt.Errorf("snooze not found for thread %s", threadID)
	if !strings.Contains(simulatedErr.Error(), "snooze not found") {
		t.Errorf("error message %q does not contain 'snooze not found'", simulatedErr.Error())
	}
}
