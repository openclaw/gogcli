package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
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

	store := newGmailWatchTestStore(t, "me@example.com")
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
	writeGmailWatchStateFile(t, store.Path(), renewed)

	if updateErr := store.Update(func(s *gmailWatchState) error {
		s.LastDeliveryStatus = "ok"
		s.LastDeliveryAtMs = 1234
		return nil
	}); updateErr != nil {
		t.Fatalf("delivery update: %v", updateErr)
	}

	got := readGmailWatchStateFile(t, store.Path())
	if got.ExpirationMs != 2000 || got.ProviderExpirationMs != 2000 || got.RenewAfterMs != 1900 {
		t.Fatalf("renewal fields were clobbered: %#v", got)
	}
	if got.LastDeliveryStatus != "ok" || got.LastDeliveryAtMs != 1234 {
		t.Fatalf("delivery fields not updated: %#v", got)
	}
}

func TestGmailWatchStoreSerializesConcurrentInstances(t *testing.T) {
	setWatchTestConfigHome(t)

	stores := []*gmailWatchStore{
		newGmailWatchTestStore(t, "me@example.com"),
		newGmailWatchTestStore(t, "me@example.com"),
	}
	if err := stores[0].Update(func(state *gmailWatchState) error {
		*state = gmailWatchState{Account: "me@example.com"}
		return nil
	}); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	const updates = 20
	errs := make(chan error, updates)
	var wg sync.WaitGroup
	for index := range updates {
		wg.Add(1)
		go func(store *gmailWatchStore) {
			defer wg.Done()
			errs <- store.Update(func(state *gmailWatchState) error {
				time.Sleep(time.Millisecond)
				state.LastDeliveryAtMs++
				return nil
			})
		}(stores[index%len(stores)])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent update: %v", err)
		}
	}

	state := loadGmailWatchTestStore(t, "me@example.com").Get()
	if state.LastDeliveryAtMs != updates {
		t.Fatalf("updates = %d, want %d", state.LastDeliveryAtMs, updates)
	}
}

func TestGmailWatchStoreUsesInjectedRuntimeLayout(t *testing.T) {
	root := t.TempDir()
	runtimeConfigDir := filepath.Join(root, "runtime-config")
	runtimeStateDir := filepath.Join(root, "runtime-state")
	ambientConfigDir := filepath.Join(root, "ambient-config")
	ambientStateDir := filepath.Join(root, "ambient-state")
	t.Setenv("GOG_CONFIG_DIR", ambientConfigDir)
	t.Setenv("GOG_STATE_DIR", ambientStateDir)

	ctx := withTestRuntime(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), func(runtime *app.Runtime) {
		runtime.Layout = config.Layout{
			ConfigDir:      runtimeConfigDir,
			StateDir:       runtimeStateDir,
			ExplicitConfig: true,
			ExplicitState:  true,
		}
	})
	store, err := newGmailWatchStore(ctx, "me@example.com")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.Update(func(state *gmailWatchState) error {
		*state = gmailWatchState{Account: "me@example.com", HistoryID: "1"}
		return nil
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	expected := filepath.Join(runtimeStateDir, "gmail-watch", "me_example_com.json")
	if store.Path() != expected {
		t.Fatalf("path = %q, want %q", store.Path(), expected)
	}
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("runtime state file: %v", err)
	}
	for _, path := range []string{ambientConfigDir, ambientStateDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("ambient path unexpectedly touched: %s (%v)", path, err)
		}
	}
}

func TestGmailWatchStatePathFallsBackToLegacyAccountFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdg-state"))

	stateDir := filepath.Join(home, "xdg-state", config.AppName, "gmail-watch")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}

	legacyDir := filepath.Join(home, "xdg-config", config.AppName, "state", "gmail-watch")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("mkdir legacy state dir: %v", err)
	}
	legacyPath := filepath.Join(legacyDir, sanitizeAccountForPath("me@example.com")+".json")
	writeGmailWatchStateFile(t, legacyPath, gmailWatchState{Account: "me@example.com", HistoryID: "1"})

	got, err := gmailWatchStatePath(gmailWatchTestLayout(t), "me@example.com")
	if err != nil {
		t.Fatalf("gmailWatchStatePath: %v", err)
	}
	if got != legacyPath {
		t.Fatalf("got %q, want legacy path %q", got, legacyPath)
	}
}

func TestGmailWatchStatePathExplicitStateSkipsLegacyAccountFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_STATE_DIR", filepath.Join(home, "isolated-state"))

	legacyDir := filepath.Join(home, "xdg-config", config.AppName, "state", "gmail-watch")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("mkdir legacy state dir: %v", err)
	}
	legacyPath := filepath.Join(legacyDir, sanitizeAccountForPath("me@example.com")+".json")
	writeGmailWatchStateFile(t, legacyPath, gmailWatchState{Account: "me@example.com", HistoryID: "1"})

	got, err := gmailWatchStatePath(gmailWatchTestLayout(t), "me@example.com")
	if err != nil {
		t.Fatalf("gmailWatchStatePath: %v", err)
	}
	if got == legacyPath {
		t.Fatalf("expected explicit state dir to skip legacy path %q", legacyPath)
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
