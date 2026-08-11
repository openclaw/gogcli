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
	pages   [][]map[string]any
	tokens  []string
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
	idx := 0
	for i, tok := range h.tokens {
		if tok != "" && tok == q["pageToken"] {
			idx = i + 1
		}
	}
	var items []map[string]any
	if idx < len(h.pages) {
		items = h.pages[idx]
	}
	next := ""
	if idx < len(h.tokens) {
		next = h.tokens[idx]
	}
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "nextPageToken": next})
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
	h := &capturingEventsHandler{pages: [][]map[string]any{{event("e1", "2026-09-26")}}, tokens: []string{""}}
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

// The second defect in the report: when --max truncates the first page, a later
// event can be missing from the output. The caller must be able to detect that,
// and --all-pages must recover the event.
func TestListCalendarEvents_MaxTruncationIsDetectableAndAllPagesRecovers(t *testing.T) {
	newHandler := func() *capturingEventsHandler {
		return &capturingEventsHandler{
			pages: [][]map[string]any{
				{event("early", "2026-08-29")},
				{event("wanted", "2026-09-27")},
			},
			tokens: []string{"page2", ""},
		}
	}

	// Single page with --max 1: the wanted event is truncated away, but the
	// response must carry a nextPageToken so the caller can tell.
	h := newHandler()
	svc, closeServer := newCalendarServiceForTest(t, h)
	defer closeServer()

	var out bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &out, io.Discard)
	if err := listCalendarEvents(ctx, svc, "cal1", "2026-08-01T00:00:00Z", "2026-10-01T00:00:00Z", 1, "", false, false, "", "", "", "", nil, false, false, "", "", nil); err != nil {
		t.Fatalf("listCalendarEvents: %v", err)
	}
	var parsed struct {
		Events []map[string]any `json:"events"`
		Next   string           `json:"nextPageToken"`
	}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if strings.Contains(out.String(), "wanted") {
		t.Fatalf("test setup wrong: the truncated event should not be on page 1")
	}
	if parsed.Next == "" {
		t.Fatalf("truncated result advertised no nextPageToken, so truncation is undetectable")
	}

	// --all-pages must return the event that page 1 truncated away.
	h2 := newHandler()
	svc2, closeServer2 := newCalendarServiceForTest(t, h2)
	defer closeServer2()

	var out2 bytes.Buffer
	ctx2 := newCmdRuntimeJSONOutputContext(t, &out2, io.Discard)
	if err := listCalendarEvents(ctx2, svc2, "cal1", "2026-08-01T00:00:00Z", "2026-10-01T00:00:00Z", 1, "", true, false, "", "", "", "", nil, false, false, "", "", nil); err != nil {
		t.Fatalf("listCalendarEvents --all-pages: %v", err)
	}
	if !strings.Contains(out2.String(), "wanted") {
		t.Fatalf("--all-pages did not recover the truncated event: %s", out2.String())
	}
}

// Case C from the report: the --query path is what a caller should reach for
// when it needs a specific later-dated event, so the query must actually be
// forwarded to the API rather than filtered client-side.
func TestListCalendarEvents_QueryIsForwardedToAPI(t *testing.T) {
	h := &capturingEventsHandler{pages: [][]map[string]any{{event("Alice at ping pong tournament", "2026-09-27")}}, tokens: []string{""}}
	svc, closeServer := newCalendarServiceForTest(t, h)
	defer closeServer()

	var out bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &out, io.Discard)
	if err := listCalendarEvents(ctx, svc, "cal1", "2026-08-01T00:00:00Z", "2026-11-01T00:00:00Z", 80, "", false, false, "ping", "", "", "", nil, false, false, "", "", nil); err != nil {
		t.Fatalf("listCalendarEvents: %v", err)
	}

	if got := h.lastQuery(t)["q"]; got != "ping" {
		t.Fatalf("query param q = %q, want %q", got, "ping")
	}
	if !strings.Contains(out.String(), "ping pong") {
		t.Fatalf("query result missing from output: %s", out.String())
	}
}
