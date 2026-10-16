package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/googleauth"
)

func TestMCPMutationReceiptOutcomes(t *testing.T) {
	for _, row := range []struct {
		statuses []int
		fail     bool
		want     string
	}{{nil, true, "failed"}, {[]int{200}, false, "committed"}, {[]int{200}, true, "committed"}, {[]int{400}, true, "failed"}, {[]int{500}, true, "outcome_unknown"}, {[]int{200, 400}, true, "partial"}, {[]int{200, 500}, true, "outcome_unknown"}} {
		recorder := &mcpMutationRecorder{}
		for _, status := range row.statuses {
			recorder.observe(googleapi.MutationAttempt{Method: "PATCH", Path: "/calendars/primary/events/event", StatusCode: status, Unknown: status >= 500})
		}
		var err error
		if row.fail {
			err = errors.New("private provider error with body")
		}
		got := recorder.receipt(err, false)
		if got.Outcome != row.want || got.RetrySafe {
			t.Fatal(row, got)
		}
	}
}

func TestMCPMutationReceiptDryRun(t *testing.T) {
	t.Setenv("GOG_HOME", t.TempDir())
	var err error
	stdout := captureStdout(t, func() {
		err = Execute([]string{"--json", "--mcp-receipt", "--dry-run", "calendar", "create", "primary", "--summary=fixture", "--from=2026-01-01T00:00:00Z", "--to=2026-01-01T01:00:00Z"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"outcome":"not_attempted"`) || !strings.Contains(stdout, `"dry_run":true`) {
		t.Fatal(stdout)
	}
	if strings.Contains(stdout, "fixture") {
		t.Fatal("full mutation output was not suppressed")
	}
}

func TestMCPMutationReceiptPartialBeforeSecondWrite(t *testing.T) {
	recorder := &mcpMutationRecorder{}
	ctx := context.WithValue(context.Background(), mcpMutationKey{}, recorder)
	expectMCPMutationWrites(ctx)
	recorder.observe(googleapi.MutationAttempt{StatusCode: 200})
	receipt := recorder.receipt(errors.New("read failed before second write"), false)
	if receipt.Outcome != "partial" || receipt.KnownSteps != 1 || receipt.AttemptedSteps != 1 {
		t.Fatal("incomplete action claimed committed", receipt)
	}
}

func TestMCPMutationParentTrimRecordsConfirmedID(t *testing.T) {
	recorder := &mcpMutationRecorder{}
	ctx := context.WithValue(context.Background(), mcpMutationKey{}, recorder)
	svc, closeServer := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Query().Get("sendUpdates") != "none" {
			t.Error(r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"id":"parent1"}`)
	}), calendar.NewService)
	defer closeServer()
	err := truncateParentRecurrence(ctx, svc, "primary", "parent1", []string{"RRULE:FREQ=DAILY"}, "2026-01-02T00:00:00Z", "none")
	if err != nil {
		t.Fatal(err)
	}
	if len(recorder.ids) != 1 || recorder.ids[0] != "parent1" {
		t.Fatal("confirmed parent ID lost", recorder.ids)
	}
}

func TestMCPMutationReceiptLargeResult(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Setenv("GOG_HOME", t.TempDir())
			description := strings.Repeat("synthetic", 7680)
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"primary","timeZone":"UTC"}`)
					return
				}
				attempts.Add(1)
				if r.Method != http.MethodPost || r.URL.Query().Get("sendUpdates") != "none" {
					t.Errorf("unexpected write: %s %s", r.Method, r.URL)
				}
				var event calendar.Event
				if err := json.NewDecoder(r.Body).Decode(&event); err != nil || event.Description != description {
					t.Errorf("description lost: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == http.StatusOK {
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "event1", "description": description})
				} else {
					_, _ = io.WriteString(w, `{"error":{"code":500,"message":"synthetic failure"}}`)
				}
			}))
			defer srv.Close()
			runtime := &app.Runtime{Services: app.Services{Calendar: func(ctx context.Context, account string) (*calendar.Service, error) {
				client, err := googleapi.NewHTTPClient(ctx, googleauth.ServiceCalendar, account)
				if err != nil {
					return nil, err
				}
				return calendar.NewService(ctx, option.WithHTTPClient(client), option.WithEndpoint(srv.URL+"/"))
			}}}
			result := executeWithTestRuntime(t, []string{"--json", "--mcp-receipt", "--no-input", "--account=fixture@example.invalid", "--access-token=synthetic-test-token", "calendar", "create", "primary", "--summary=fixture", "--description=" + description, "--from=2026-01-01T00:00:00Z", "--to=2026-01-01T01:00:00Z", "--send-updates=none"}, runtime)
			if attempts.Load() != 1 {
				t.Fatalf("write replayed: %d attempts", attempts.Load())
			}
			if len(result.stdout) > 4096 || strings.Contains(result.stdout, "synthetic") {
				t.Fatalf("provider output leaked into receipt: %d bytes", len(result.stdout))
			}
			want := "committed"
			if status >= 500 {
				want = "outcome_unknown"
			}
			if !strings.Contains(result.stdout, `"outcome":"`+want+`"`) || !strings.Contains(result.stdout, `"attempted_steps":1`) || !strings.Contains(result.stdout, `"retry_safe":false`) {
				t.Fatalf("missing bounded receipt: %s, %v", result.stdout, result.err)
			}
			if status == http.StatusOK && (result.err != nil || !strings.Contains(result.stdout, `"ids":["event1"]`)) {
				t.Fatal(result.stdout, result.err)
			}
			if status >= 500 && result.err == nil {
				t.Fatal("uncertain write reported success")
			}
		})
	}
}
