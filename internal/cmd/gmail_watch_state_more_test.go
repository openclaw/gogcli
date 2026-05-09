package cmd

import (
	"encoding/json"
	"os"
	"testing"
)

func TestIsStaleHistoryID(t *testing.T) {
	stale, err := isStaleHistoryID("5", "4")
	if err != nil {
		t.Fatalf("isStaleHistoryID: %v", err)
	}
	if !stale {
		t.Fatalf("expected stale for older history id")
	}

	stale, err = isStaleHistoryID("5", "6")
	if err != nil {
		t.Fatalf("isStaleHistoryID: %v", err)
	}
	if stale {
		t.Fatalf("expected non-stale for newer history id")
	}

	stale, err = isStaleHistoryID("", "")
	if err != nil {
		t.Fatalf("isStaleHistoryID empty: %v", err)
	}
	if stale {
		t.Fatalf("expected non-stale for empty ids")
	}

	if _, err := isStaleHistoryID("bad", "5"); err == nil {
		t.Fatalf("expected error for invalid history id")
	}
}

func TestGmailWatchStoreUpdateReloadsDiskState(t *testing.T) {
	setWatchTestConfigHome(t)

	store, err := newGmailWatchStore("me@example.com")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if updateErr := store.Update(func(s *gmailWatchState) error {
		*s = gmailWatchState{
			Account:              "me@example.com",
			Topic:                "projects/p/topics/t",
			HistoryID:            "100",
			ExpirationMs:         1000,
			ProviderExpirationMs: 1000,
			RenewAfterMs:         900,
		}
		return nil
	}); updateErr != nil {
		t.Fatalf("initial update: %v", updateErr)
	}

	renewed := store.Get()
	renewed.ExpirationMs = 2000
	renewed.ProviderExpirationMs = 2000
	renewed.RenewAfterMs = 1900
	writeGmailWatchStateFile(t, store.path, renewed)

	if updateErr := store.Update(func(s *gmailWatchState) error {
		s.LastDeliveryStatus = "ok"
		s.LastDeliveryAtMs = 1234
		return nil
	}); updateErr != nil {
		t.Fatalf("delivery update: %v", updateErr)
	}

	got := readGmailWatchStateFile(t, store.path)
	if got.ExpirationMs != 2000 || got.ProviderExpirationMs != 2000 || got.RenewAfterMs != 1900 {
		t.Fatalf("renewal fields were clobbered: %#v", got)
	}
	if got.LastDeliveryStatus != "ok" || got.LastDeliveryAtMs != 1234 {
		t.Fatalf("delivery fields not updated: %#v", got)
	}
}

func writeGmailWatchStateFile(t *testing.T, path string, state gmailWatchState) {
	t.Helper()

	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

func readGmailWatchStateFile(t *testing.T, path string) gmailWatchState {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var state gmailWatchState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	return state
}
