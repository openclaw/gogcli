package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/googleauth"
)

func TestMCPExportStoreNewWaiterAfterAbandonment(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 128, Entries: 8, Fetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := mcpSnapshotKey{Object: "same"}
	started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	fetch := func(ctx context.Context) ([]byte, gmailExportMetadata, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			close(cancelled)
			<-release
			return nil, gmailExportMetadata{}, errors.New("export fetch failed")
		}
		return []byte("fresh"), gmailExportMetadata{MessageID: "m1"}, nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := make(chan error, 1)
	go func() { _, e := store.Acquire(ctx, key, 64, fetch); first <- e }()
	<-started
	cancel()
	if err = <-first; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	<-cancelled
	second := make(chan error, 1)
	go func() {
		lease, e := store.Acquire(t.Context(), key, 64, fetch)
		if lease != nil {
			lease.Release()
		}
		second <- e
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		store.mu.Lock()
		joined := store.pending[key] != nil && store.pending[key].waiters == 1
		store.mu.Unlock()
		if joined || time.Now().After(deadline) {
			close(release)
			if !joined {
				t.Fatal("new waiter failed to join the abandoned fetch")
			}
			break
		}
		runtime.Gosched()
	}
	if err = <-second; err != nil || calls.Load() != 2 {
		t.Fatal("live request inherited another caller's cancellation", err, calls.Load())
	}
}

func TestCalendarUpdateRejectsSourceTitleWithClear(t *testing.T) {
	_, _, err := buildCalendarUpdatePatch(calendarUpdateInput{SourceTitle: "retain this title"}, calendarUpdateFields{SourceURL: true, SourceTitle: true})
	if err == nil {
		t.Fatal("explicit title silently discarded while clearing source")
	}
}

func TestCalendarRespondDryRunValidatesNotificationMode(t *testing.T) {
	for _, mode := range []string{"bogus", "none", "all", "externalOnly"} {
		t.Run(mode, func(t *testing.T) {
			result := executeWithTestRuntime(t, []string{"--json", "--dry-run", "calendar", "respond", "primary", "e1", "--status=accepted", "--send-updates=" + mode}, &app.Runtime{Services: app.Services{Calendar: func(context.Context, string) (*calendar.Service, error) {
				t.Fatal("dry run opened the calendar service")
				return nil, errors.New("unexpected calendar service creation")
			}}})
			if mode == "bogus" {
				if result.err == nil {
					t.Fatal("invalid notification mode passed dry run")
				}
				return
			}
			var payload struct {
				Request map[string]any `json:"request"`
			}
			if result.err != nil || json.Unmarshal([]byte(result.stdout), &payload) != nil || payload.Request["send_updates"] != mode {
				t.Fatal("notification mode missing from dry run", result.stdout, result.err)
			}
		})
	}
}

func TestMCPAllDayRecurrenceScope(t *testing.T) {
	for _, name := range []string{"calendar_update_event", "calendar_cancel_event"} {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]any{"event_id": "e1", "scope": "single", "original_start": "2026-10-01"}
		if _, _, err := originalStartRange("2026-10-01"); err != nil {
			t.Fatal(err)
		}
		if _, err := findMCPTool(t, name).BuildArgs(req); err != nil {
			t.Errorf("%s rejects native all-day instance: %v", name, err)
		}
	}
}

func TestMCPLegacyToolsWithoutSnapshotStorage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TMPDIR", filepath.Join(home, "missing"))
	result := executeWithTestRuntime(t, []string{"--home", home, "mcp", "--allow-tool=gmail_search"}, nil)
	if result.err != nil {
		t.Errorf("legacy-only server rejected startup: %v", result.err)
	}
}

func TestMCPFutureCancelPartialReceipt(t *testing.T) { testMCPFutureActionPartialReceipt(t, "delete") }

func TestMCPFutureUpdatePartialReceipt(t *testing.T) { testMCPFutureActionPartialReceipt(t, "update") }

func testMCPFutureActionPartialReceipt(t *testing.T, operation string) {
	t.Helper()
	t.Setenv("GOG_HOME", t.TempDir())
	deletes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete || r.Method == http.MethodPatch:
			deletes++
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
			} else {
				_, _ = io.WriteString(w, `{"id":"instance1"}`)
			}
		case strings.HasSuffix(r.URL.Path, "/instances"):
			_, _ = io.WriteString(w, `{"items":[{"id":"instance1","originalStartTime":{"dateTime":"2026-01-02T00:00:00Z"}}]}`)
		case strings.Contains(r.URL.Path, "/events/parent1"):
			_, _ = io.WriteString(w, `{"id":"parent1","recurrence":["RDATE:20260102T000000Z,20260103T000000Z"]}`)
		default:
			_, _ = io.WriteString(w, `{"id":"primary","timeZone":"UTC"}`)
		}
	}))
	defer srv.Close()
	rt := &app.Runtime{Services: app.Services{Calendar: func(ctx context.Context, account string) (*calendar.Service, error) {
		client, err := googleapi.NewHTTPClient(ctx, googleauth.ServiceCalendar, account)
		if err != nil {
			return nil, err
		}
		return calendar.NewService(ctx, option.WithHTTPClient(client), option.WithEndpoint(srv.URL+"/"))
	}}}
	args := []string{"--json", "--mcp-receipt", "--no-input", "--force", "--account=fixture@example.invalid", "--access-token=synthetic-test-token", "calendar", operation, "primary", "parent1", "--scope=future", "--original-start=2026-01-02T00:00:00Z", "--send-updates=none"}
	if operation == "update" {
		args = append(args, "--summary=fixture")
	}
	result := executeWithTestRuntime(t, args, rt)
	if deletes != 1 || result.err == nil {
		t.Fatalf("unexpected fixture: deletes=%d, err=%v", deletes, result.err)
	}
	var receipt mcpMutationReceipt
	if err := json.Unmarshal([]byte(result.stdout), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != "partial" {
		t.Errorf("partial cancellation reported %s; stdout=%s; err=%v", receipt.Outcome, result.stdout, result.err)
	}
}

func TestMCPStorageFailureIsBoundedAndIsolated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GOG_HOME", home)
	missing := filepath.Join(home, "unavailable-private-location")
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, missing)
	}
	ctx := authclient.WithClientResolver(context.Background(), func(string, string) (string, error) { return "default", nil })
	rt, err := newMCPToolRuntime(ctx, "fixture", &McpCmd{TimeoutSeconds: 1, MaxOutputBytes: 4096}, &RootFlags{Account: "fixture@example.invalid", AccessToken: "synthetic-test-token"}, config.MCPPolicy{})
	if err != nil {
		t.Fatal("storage failure disabled ordinary runtime", err)
	}
	if rt.store != nil {
		defer rt.store.Close()
		t.Fatal("unexpected snapshot store")
	}
	calls := 0
	rt.runChild = func(context.Context, mcpToolSpec, []string) (mcpCommandResult, error) {
		calls++
		return mcpCommandResult{Stdout: map[string]any{"calendars": []any{}}}, nil
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	if got := rt.runSpecial(context.Background(), findMCPTool(t, "calendar_list_calendars"), req); got.IsError || calls != 1 {
		t.Fatal("non-snapshot read unavailable", got, calls)
	}
	req.Params.Arguments = map[string]any{"message_id": "m1"}
	got := rt.runSpecial(context.Background(), mcpExportTools()[0], req)
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsError || len(data) > 4096 || !strings.Contains(string(data), "snapshot_unavailable") || strings.Contains(string(data), missing) || calls != 1 {
		t.Fatal("unbounded or unredacted storage failure", got)
	}
}

func TestMCPAllDayActionsReachNativeHandlers(t *testing.T) {
	for _, operation := range []string{"update", "cancel"} {
		for _, scope := range []string{"single", "future"} {
			t.Run(operation+"/"+scope, func(t *testing.T) {
				t.Setenv("GOG_HOME", t.TempDir())
				writes := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodPatch || r.Method == http.MethodDelete {
						writes++
						if r.URL.Query().Get("sendUpdates") != "none" {
							t.Error("unexpected notification mode", r.URL)
						}
						if strings.HasSuffix(r.URL.Path, "/instance1") && operation == "update" {
							var event calendar.Event
							if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
								t.Error(err)
								w.WriteHeader(http.StatusBadRequest)
								return
							}
							if len(event.Attendees) != 1 || event.Attendees[0].Comment != "comma, stays" || len(event.Attachments) != 1 || event.Attachments[0].FileUrl != "https://example.invalid/a,b" {
								t.Error("literal array split in native dispatch", event)
							}
						}
						if r.Method == http.MethodDelete {
							w.WriteHeader(http.StatusNoContent)
						} else {
							_, _ = io.WriteString(w, `{"id":"instance1"}`)
						}
						return
					}
					switch {
					case strings.HasSuffix(r.URL.Path, "/instances"):
						_, _ = io.WriteString(w, `{"items":[{"id":"instance1","originalStartTime":{"date":"2026-10-01"}}]}`)
					case strings.Contains(r.URL.Path, "/events/parent1"):
						_, _ = io.WriteString(w, `{"id":"parent1","recurrence":["RRULE:FREQ=DAILY"],"start":{"date":"2026-09-01"},"end":{"date":"2026-09-02"}}`)
					default:
						_, _ = io.WriteString(w, `{"id":"primary","timeZone":"UTC"}`)
					}
				}))
				defer srv.Close()
				rt := &app.Runtime{Services: app.Services{Calendar: func(ctx context.Context, account string) (*calendar.Service, error) {
					client, err := googleapi.NewHTTPClient(ctx, googleauth.ServiceCalendar, account)
					if err != nil {
						return nil, err
					}
					return calendar.NewService(ctx, option.WithHTTPClient(client), option.WithEndpoint(srv.URL+"/"))
				}}}
				name := "calendar_cancel_event"
				if operation == "update" {
					name = "calendar_update_event"
				}
				req := mcp.CallToolRequest{}
				arguments := map[string]any{"event_id": "parent1", "scope": scope, "original_start": "2026-10-01"}
				if operation == "update" {
					arguments["summary"] = "fixture"
					arguments["attendees"] = []any{"fixture@example.invalid;comment=comma, stays"}
					arguments["attachment_urls"] = []any{"https://example.invalid/a,b"}
				}
				req.Params.Arguments = arguments
				args, err := findMCPTool(t, name).BuildArgs(req)
				if err != nil {
					t.Fatal(err)
				}
				root := make([]string, 0, 6+len(args))
				root = append(root, "--json", "--mcp-receipt", "--no-input", "--force", "--account=fixture@example.invalid", "--access-token=synthetic-test-token")
				result := executeWithTestRuntime(t, append(root, args...), rt)
				want := 1
				if scope == "future" {
					want = 2
				}
				if result.err != nil || writes != want || !strings.Contains(result.stdout, `"outcome":"committed"`) {
					t.Fatal("all-day action failed", writes, result.stdout, result.err)
				}
			})
		}
	}
}

func TestMCPFutureUpdateTrimKeepsCompletedOutcome(t *testing.T) {
	recorder := &mcpMutationRecorder{}
	ctx := context.WithValue(context.Background(), mcpMutationKey{}, recorder)
	ctx = googleapi.WithMutationObserver(ctx, recorder.observe)
	ctx = authclient.WithAccessToken(ctx, "synthetic-test-token")
	client, err := googleapi.NewHTTPClient(ctx, googleauth.ServiceCalendar, "fixture@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"parent1"}`)
	}))
	defer srv.Close()
	svc, err := calendar.NewService(ctx, option.WithHTTPClient(client), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	expectMCPMutationWrites(ctx)
	recorder.observe(googleapi.MutationAttempt{StatusCode: 200})
	if err = truncateParentRecurrence(ctx, svc, "primary", "parent1", []string{"RRULE:FREQ=DAILY"}, "2026-01-02T00:00:00Z", "none"); err != nil {
		t.Fatal(err)
	}
	receipt := recorder.receipt(errors.New("optional output failed"), false)
	if receipt.Outcome != "committed" || receipt.KnownSteps != 2 {
		t.Fatal("two confirmed writes misreported", receipt)
	}
}
