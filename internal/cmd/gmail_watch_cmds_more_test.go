package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGmailWatchRenewAndStop_JSON(t *testing.T) {
	setWatchTestConfigHome(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/watch"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId":  "123",
				"expiration": "1730000000000",
			})
			return
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/stop"):
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)

	store := newGmailWatchTestStore(t, "a@b.com")
	_ = store.Update(func(s *gmailWatchState) error {
		*s = gmailWatchState{
			Account:      "a@b.com",
			Topic:        "projects/p/topics/t",
			Labels:       []string{"INBOX"},
			HistoryID:    "100",
			RenewAfterMs: time.Now().Add(10 * time.Minute).UnixMilli(),
			ExpirationMs: time.Now().Add(20 * time.Minute).UnixMilli(),
		}
		return nil
	})

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &GmailWatchRenewCmd{}, []string{"--ttl", "3600"}, ctx, flags); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if err := runKong(t, &GmailWatchStopCmd{}, []string{}, ctx, flags); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if _, statErr := os.Stat(store.Path()); !os.IsNotExist(statErr) {
		t.Fatalf("expected watch state removed, err=%v", statErr)
	}
}

func TestGmailWatchStatusAndStop_Text(t *testing.T) {
	setWatchTestConfigHome(t)

	store := newGmailWatchTestStore(t, "a@b.com")
	_ = store.Update(func(s *gmailWatchState) error {
		*s = gmailWatchState{
			Account:   "a@b.com",
			Topic:     "projects/p/topics/t",
			HistoryID: "100",
			Hook:      &gmailWatchHook{URL: "http://example.com/hook"},
		}
		return nil
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/gmail/v1/users/me/stop") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)

	flags := &RootFlags{Account: "a@b.com", Force: true}
	var out bytes.Buffer
	ctx := withGmailTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)
	if err := runKong(t, &GmailWatchStatusCmd{}, []string{}, ctx, flags); err != nil {
		t.Fatalf("status: %v", err)
	}
	if err := runKong(t, &GmailWatchStopCmd{}, []string{}, ctx, flags); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if !strings.Contains(out.String(), "account") || !strings.Contains(out.String(), "stopped") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestGmailWatchStatusCmd_ReadOnlyStateAccess(t *testing.T) {
	t.Run("missing state", func(t *testing.T) {
		setWatchTestConfigHome(t)
		layout := gmailWatchTestLayout(t)
		ctx := newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard)

		err := runKong(t, &GmailWatchStatusCmd{}, nil, ctx, &RootFlags{
			Account: "a@b.com",
			DryRun:  true,
			NoInput: true,
		})
		if err == nil || ExitCode(err) != 1 || err.Error() != "watch state not found; run gmail watch start" {
			t.Fatalf("missing state error = %v", err)
		}
		for _, path := range []string{layout.PrimaryGmailWatchDir(), layout.LegacyGmailWatchDir()} {
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("status touched watch state path %q: %v", path, statErr)
			}
		}
	})

	t.Run("existing state", func(t *testing.T) {
		setWatchTestConfigHome(t)
		layout := gmailWatchTestLayout(t)
		statePath := filepath.Join(layout.GmailWatchDir(), sanitizeAccountForPath("a@b.com")+".json")
		if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
			t.Fatalf("mkdir watch state: %v", err)
		}
		stateBytes, marshalErr := json.Marshal(gmailWatchState{
			Account:   "a@b.com",
			Topic:     "projects/p/topics/t",
			HistoryID: "100",
		})
		if marshalErr != nil {
			t.Fatalf("marshal watch state: %v", marshalErr)
		}
		if writeErr := os.WriteFile(statePath, stateBytes, 0o600); writeErr != nil {
			t.Fatalf("write watch state: %v", writeErr)
		}

		var stdout bytes.Buffer
		ctx := newCmdRuntimeJSONOutputContext(t, &stdout, io.Discard)
		if runErr := runKong(t, &GmailWatchStatusCmd{}, nil, ctx, &RootFlags{
			Account: "a@b.com",
			DryRun:  true,
			NoInput: true,
		}); runErr != nil {
			t.Fatalf("status: %v", runErr)
		}
		if !strings.Contains(stdout.String(), `"account": "a@b.com"`) {
			t.Fatalf("unexpected status output: %s", stdout.String())
		}
		if _, statErr := os.Stat(filepath.Join(filepath.Dir(statePath), ".lock")); !os.IsNotExist(statErr) {
			t.Fatalf("status created watch lock: %v", statErr)
		}
		after, readErr := os.ReadFile(statePath)
		if readErr != nil {
			t.Fatalf("read watch state: %v", readErr)
		}
		if !bytes.Equal(after, stateBytes) {
			t.Fatalf("status changed watch state:\nwant=%s\ngot=%s", stateBytes, after)
		}
	})
}
