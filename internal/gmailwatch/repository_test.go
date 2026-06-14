package gmailwatch

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
)

var (
	errCallbackFailure = errors.New("callback failure")
	errInjectedWrite   = errors.New("injected write failure")
)

func TestRepositoryLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "account.json")
	repository := New(path, Options{Now: func() time.Time { return now }})

	if err := repository.Update(func(state *State) error {
		*state = State{
			Account:   "user@example.com",
			Topic:     "projects/p/topics/t",
			HistoryID: "100",
			Labels:    []string{"INBOX"},
			Hook:      &Hook{URL: "https://example.com/hook", Token: "secret"},
		}

		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	loaded, err := Load(path, Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	state := loaded.Get()
	if state.Account != "user@example.com" || state.HistoryID != "100" {
		t.Fatalf("state = %#v", state)
	}

	state.Labels[0] = "MUTATED"
	state.Hook.Token = "mutated"

	unchanged := loaded.Get()
	if unchanged.Labels[0] != "INBOX" || unchanged.Hook.Token != "secret" {
		t.Fatalf("Get returned shared state: %#v", unchanged)
	}

	if err := loaded.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := Load(path, Options{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load after Remove error = %v", err)
	}
}

func TestRepositoryStartHistoryID(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	repository := New(filepath.Join(t.TempDir(), "account.json"), Options{
		Now: func() time.Time { return now },
	})

	startID, err := repository.StartHistoryID("101")
	if err != nil {
		t.Fatalf("StartHistoryID initial: %v", err)
	}

	if startID != 101 {
		t.Fatalf("initial start ID = %d", startID)
	}

	if state := repository.Get(); state.HistoryID != "101" || state.UpdatedAtMs != now.UnixMilli() {
		t.Fatalf("initial state = %#v", state)
	}

	startID, err = repository.StartHistoryID("")
	if err != nil {
		t.Fatalf("StartHistoryID stored: %v", err)
	}

	if startID != 101 {
		t.Fatalf("stored start ID = %d", startID)
	}

	startID, err = repository.StartHistoryID("100")
	if err != nil {
		t.Fatalf("StartHistoryID stale: %v", err)
	}

	if startID != 0 {
		t.Fatalf("stale start ID = %d", startID)
	}

	startID, err = repository.StartHistoryID("bad")
	if err != nil {
		t.Fatalf("StartHistoryID invalid push: %v", err)
	}

	if startID != 101 {
		t.Fatalf("invalid push start ID = %d", startID)
	}
}

func TestRepositoryStartHistoryIDRejectsInvalidInitialPush(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "account.json")
	repository := New(path, Options{})

	if _, err := repository.StartHistoryID("bad"); err == nil {
		t.Fatal("StartHistoryID accepted invalid initial history")
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid initial history wrote state: %v", err)
	}
}

func TestRepositoryUpdatePreservesStateOnFailure(t *testing.T) {
	t.Parallel()

	failWrites := false
	path := filepath.Join(t.TempDir(), "account.json")
	repository := New(path, Options{
		WriteFile: func(path string, data []byte, mode os.FileMode) error {
			if failWrites {
				return errInjectedWrite
			}

			return config.WriteFileAtomic(path, data, mode)
		},
	})

	if err := repository.Update(func(state *State) error {
		state.HistoryID = "100"

		return nil
	}); err != nil {
		t.Fatalf("seed Update: %v", err)
	}

	if err := repository.Update(func(state *State) error {
		state.HistoryID = "200"

		return errCallbackFailure
	}); !errors.Is(err, errCallbackFailure) {
		t.Fatalf("callback error = %v", err)
	}

	if state := repository.Get(); state.HistoryID != "100" {
		t.Fatalf("state changed after callback failure: %#v", state)
	}

	failWrites = true

	if err := repository.Update(func(state *State) error {
		state.HistoryID = "300"

		return nil
	}); !errors.Is(err, errInjectedWrite) {
		t.Fatalf("write error = %v", err)
	}

	if state := repository.Get(); state.HistoryID != "100" {
		t.Fatalf("state changed after write failure: %#v", state)
	}
}

func TestRepositorySerializesConcurrentInstances(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "account.json")
	repositories := []*Repository{
		New(path, Options{}),
		New(path, Options{}),
	}

	if err := repositories[0].Update(func(state *State) error {
		state.Account = "user@example.com"

		return nil
	}); err != nil {
		t.Fatalf("seed Update: %v", err)
	}

	const updates = 20

	var waitGroup sync.WaitGroup
	errs := make(chan error, updates)

	for index := range updates {
		waitGroup.Add(1)

		go func(repository *Repository) {
			defer waitGroup.Done()

			errs <- repository.Update(func(state *State) error {
				state.LastDeliveryAtMs++

				return nil
			})
		}(repositories[index%len(repositories)])
	}

	waitGroup.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Update: %v", err)
		}
	}

	loaded, err := Load(path, Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := loaded.Get().LastDeliveryAtMs; got != updates {
		t.Fatalf("updates = %d, want %d", got, updates)
	}
}

func TestReadOptional(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "missing.json")
	second := filepath.Join(dir, "state.json")

	repository := New(second, Options{})
	if err := repository.Update(func(state *State) error {
		state.HistoryID = "100"

		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	state, found, err := ReadOptional(first, second)
	if err != nil {
		t.Fatalf("ReadOptional: %v", err)
	}

	if !found || state.HistoryID != "100" {
		t.Fatalf("state = %#v, found = %t", state, found)
	}

	state, found, err = ReadOptional(filepath.Join(dir, "absent.json"))
	if err != nil {
		t.Fatalf("ReadOptional absent: %v", err)
	}

	if found || state.HistoryID != "" || state.Hook != nil || len(state.Labels) != 0 {
		t.Fatalf("absent state = %#v, found = %t", state, found)
	}
}

func TestHistoryTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	state := State{HistoryID: "100", LastPushMessageID: "before", RateLimitedUntilMs: 123}

	if err := AdvanceHistory(&state, "200", "push", now); err != nil {
		t.Fatalf("AdvanceHistory: %v", err)
	}

	if state.HistoryID != "200" || state.LastPushMessageID != "push" || state.UpdatedAtMs != now.UnixMilli() {
		t.Fatalf("advanced state = %#v", state)
	}

	if state.RateLimitedUntilMs != 123 {
		t.Fatalf("unrelated state changed: %#v", state)
	}

	before := State{HistoryID: "100", LastPushMessageID: "before"}
	if !RestoreProgress(&state, before, "200", "push") {
		t.Fatal("RestoreProgress did not restore matching progress")
	}

	if state.HistoryID != "100" || state.LastPushMessageID != "before" {
		t.Fatalf("restored state = %#v", state)
	}

	state.HistoryID = "300"
	if RestoreProgress(&state, before, "200", "push") {
		t.Fatal("RestoreProgress replaced newer progress")
	}

	stale, err := IsStaleHistoryID("300", "200")
	if err != nil {
		t.Fatalf("IsStaleHistoryID: %v", err)
	}

	if !stale {
		t.Fatal("older history was not stale")
	}
}
