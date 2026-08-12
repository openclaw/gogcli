package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests pin the interaction between the anchored window flags
// (--from/--to) and the convenience window flags (--days/--today/--tomorrow/
// --week).
//
// The regression they exist for: --days sat in a switch arm that ran before
// --from was ever read, so `--from 2026-09-25 --days 5` silently discarded
// --from and returned a today-anchored window instead. The command exited 0
// and printed a well-formed table, so a caller received a confident wrong
// answer with nothing to detect it by.

func TestResolveTimeRange_FutureFromWithDays_HonoursFrom(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")
	flags := TimeRangeFlags{From: "2026-09-25", Days: 5}

	tr, err := ResolveTimeRangeWithDefaults(context.Background(), svc, flags, TimeRangeDefaults{})
	if err != nil {
		t.Fatalf("ResolveTimeRangeWithDefaults: %v", err)
	}

	wantFrom := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	if !tr.From.Equal(wantFrom) {
		t.Fatalf("--from was discarded: From = %v, want %v", tr.From, wantFrom)
	}
	if !tr.To.Equal(wantTo) {
		t.Fatalf("window end not anchored to --from: To = %v, want %v", tr.To, wantTo)
	}
}

// A --from expressed as an exact instant must not be rounded back to midnight,
// because that would be the same class of silent input discard.
func TestResolveTimeRange_ExactFromWithDays_KeepsTimeOfDay(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")
	flags := TimeRangeFlags{From: "2026-09-25T14:30:00Z", Days: 2}

	tr, err := ResolveTimeRangeWithDefaults(context.Background(), svc, flags, TimeRangeDefaults{})
	if err != nil {
		t.Fatalf("ResolveTimeRangeWithDefaults: %v", err)
	}

	wantFrom := time.Date(2026, 9, 25, 14, 30, 0, 0, time.UTC)
	wantTo := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	if !tr.From.Equal(wantFrom) {
		t.Fatalf("From = %v, want %v", tr.From, wantFrom)
	}
	if !tr.To.Equal(wantTo) {
		t.Fatalf("To = %v, want %v", tr.To, wantTo)
	}
}

func TestResolveTimeRange_FromWithDaysSpansDST(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}
	svc := newCalendarServiceWithTimezone(t, loc.String())

	tr, err := ResolveTimeRangeWithDefaults(context.Background(), svc, TimeRangeFlags{
		From: "2026-10-31",
		Days: 3,
	}, TimeRangeDefaults{})
	if err != nil {
		t.Fatalf("ResolveTimeRangeWithDefaults: %v", err)
	}

	wantFrom := time.Date(2026, 10, 31, 0, 0, 0, 0, loc)
	wantTo := time.Date(2026, 11, 3, 0, 0, 0, 0, loc)
	if !tr.From.Equal(wantFrom) || !tr.To.Equal(wantTo) {
		t.Fatalf("range = %v -> %v, want %v -> %v", tr.From, tr.To, wantFrom, wantTo)
	}
	if got := tr.To.Sub(tr.From); got != 73*time.Hour {
		t.Fatalf("DST-spanning duration = %v, want 73h", got)
	}
}

func TestResolveTimeRange_FromWithoutDaysKeepsDefaultEnd(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")

	tr, err := ResolveTimeRange(context.Background(), svc, TimeRangeFlags{From: "2026-09-25T14:30:00Z"})
	if err != nil {
		t.Fatalf("ResolveTimeRange: %v", err)
	}

	wantFrom := time.Date(2026, 9, 25, 14, 30, 0, 0, time.UTC)
	wantTo := wantFrom.Add(7 * 24 * time.Hour)
	if !tr.From.Equal(wantFrom) || !tr.To.Equal(wantTo) {
		t.Fatalf("range = %v -> %v, want %v -> %v", tr.From, tr.To, wantFrom, wantTo)
	}
}

// Case B from the bug report: the same future window expressed with an
// explicit end date was never broken and must stay correct.
func TestResolveTimeRange_FutureFromWithTo_Unaffected(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")
	flags := TimeRangeFlags{From: "2026-09-25", To: "2026-09-30"}

	tr, err := ResolveTimeRangeWithDefaults(context.Background(), svc, flags, TimeRangeDefaults{})
	if err != nil {
		t.Fatalf("ResolveTimeRangeWithDefaults: %v", err)
	}

	wantFrom := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if !tr.From.Equal(wantFrom) || !tr.To.Equal(wantTo) {
		t.Fatalf("range = %v -> %v, want %v -> %v", tr.From, tr.To, wantFrom, wantTo)
	}
}

// --days on its own keeps its documented "next N days" meaning.
func TestResolveTimeRange_DaysWithoutFrom_StaysTodayAnchored(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")

	tr, err := ResolveTimeRangeWithDefaults(context.Background(), svc, TimeRangeFlags{Days: 3}, TimeRangeDefaults{})
	if err != nil {
		t.Fatalf("ResolveTimeRangeWithDefaults: %v", err)
	}

	now := time.Now().In(time.UTC)
	wantFrom := startOfDay(now)
	wantTo := startOfDay(now.AddDate(0, 0, 3))
	if !tr.From.Equal(wantFrom) || !tr.To.Equal(wantTo) {
		t.Fatalf("range = %v -> %v, want %v -> %v", tr.From, tr.To, wantFrom, wantTo)
	}
}

// --days and --to are two different ways of specifying the window end. There
// is no reading that honours both, so the combination must be refused rather
// than resolved by a silent precedence rule.
func TestResolveTimeRange_DaysWithTo_Rejected(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")
	flags := TimeRangeFlags{From: "2026-09-25", To: "2026-09-30", Days: 5}

	_, err := ResolveTimeRangeWithDefaults(context.Background(), svc, flags, TimeRangeDefaults{})
	if err == nil {
		t.Fatalf("expected --days with --to to be rejected")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
	}
	for _, want := range []string{"--days", "--to"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error must name the conflicting flag %q, got %q", want, err.Error())
		}
	}
}

// --today/--tomorrow/--week are fixed windows with no anchoring interpretation,
// so pairing them with --from/--to/--days is a genuine conflict.
func TestResolveTimeRange_FixedPresetWithAnchor_Rejected(t *testing.T) {
	cases := []struct {
		name  string
		flags TimeRangeFlags
		names []string
	}{
		{"today with from", TimeRangeFlags{Today: true, From: "2026-09-25"}, []string{"--today", "--from"}},
		{"today with to", TimeRangeFlags{Today: true, To: "2026-09-25"}, []string{"--today", "--to"}},
		{"today with days", TimeRangeFlags{Today: true, Days: 5}, []string{"--today", "--days"}},
		{"tomorrow with from", TimeRangeFlags{Tomorrow: true, From: "2026-09-25"}, []string{"--tomorrow", "--from"}},
		{"week with from", TimeRangeFlags{Week: true, From: "2026-09-25"}, []string{"--week", "--from"}},
		{"today with week", TimeRangeFlags{Today: true, Week: true}, []string{"--today", "--week"}},
	}

	svc := newCalendarServiceWithTimezone(t, "UTC")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveTimeRangeWithDefaults(context.Background(), svc, tc.flags, TimeRangeDefaults{})
			if err == nil {
				t.Fatalf("expected %s to be rejected", tc.name)
			}
			if got := ExitCode(err); got != 2 {
				t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
			}
			for _, want := range tc.names {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error must name %q, got %q", want, err.Error())
				}
			}
		})
	}
}

// A negative --days previously fell through to the default branch, which is the
// same silent-discard failure wearing a different hat.
func TestResolveTimeRange_NegativeDays_Rejected(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")

	_, err := ResolveTimeRangeWithDefaults(context.Background(), svc, TimeRangeFlags{Days: -3}, TimeRangeDefaults{})
	if err == nil {
		t.Fatalf("expected a negative --days to be rejected")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
	}
}

// capturingEventsHandler records the query parameters the events list call
// actually sends, and serves pages so pagination behaviour can be asserted.
type capturingEventsHandler struct {
	mu      sync.Mutex
	queries []map[string]string
	items   []map[string]any
}

func (h *capturingEventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if (r.URL.Path == "/calendars/cal1" || r.URL.Path == "/calendars/primary") && r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": strings.TrimPrefix(r.URL.Path, "/calendars/"), "timeZone": "UTC"})
		return
	}
	if !strings.Contains(r.URL.Path, "/calendars/cal1/events") || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	q := map[string]string{}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			q[k] = v[0]
		}
	}

	h.mu.Lock()
	h.queries = append(h.queries, q)
	items := h.items
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (h *capturingEventsHandler) lastQuery(t *testing.T) map[string]string {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.queries) == 0 {
		t.Fatalf("no events list request was made")
	}
	return h.queries[len(h.queries)-1]
}

func event(id, day string) map[string]any {
	return map[string]any{"id": id, "summary": id, "start": map[string]any{"date": day}, "end": map[string]any{"date": day}}
}

// End-to-end over both halves: the window the user asked for must be the window
// that reaches the API as timeMin/timeMax.
func TestListCalendarEvents_ResolvedWindowReachesAPI(t *testing.T) {
	h := &capturingEventsHandler{items: []map[string]any{event("e1", "2026-09-26")}}
	svc, closeServer := newCalendarServiceForTest(t, h)
	defer closeServer()

	tr, err := ResolveTimeRangeWithDefaults(context.Background(), svc, TimeRangeFlags{From: "2026-09-25", Days: 5}, TimeRangeDefaults{})
	if err != nil {
		t.Fatalf("ResolveTimeRangeWithDefaults: %v", err)
	}
	from, to := tr.FormatRFC3339()

	var out bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &out, io.Discard)
	if err := listCalendarEvents(ctx, svc, "cal1", from, to, 10, "", false, false, "", "", "", "", nil, false, false, "", "", nil); err != nil {
		t.Fatalf("listCalendarEvents: %v", err)
	}

	q := h.lastQuery(t)
	if !strings.HasPrefix(q["timeMin"], "2026-09-25") {
		t.Fatalf("timeMin = %q, want the requested 2026-09-25 window start", q["timeMin"])
	}
	if !strings.HasPrefix(q["timeMax"], "2026-09-30") {
		t.Fatalf("timeMax = %q, want the requested 2026-09-30 window end", q["timeMax"])
	}
}
