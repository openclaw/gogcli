package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/outfmt"
)

// freeBusyErrorsServer answers freeBusy.query the way Google does when one of
// the requested calendars cannot be read: that calendar has errors and no busy
// periods. The response lists calendars in a different order than requested.
func runFreeBusyWithErrors(t *testing.T, mode outfmt.Mode, args ...string) (string, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/freeBusy") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"kind": "calendar#freeBusy",
			"calendars": {
				"a@example.com": {"busy": [{"start": "2026-11-10T17:00:00Z", "end": "2026-11-10T18:00:00Z"}]},
				"b@example.com": {"busy": [{"start": "2026-11-10T19:00:00Z", "end": "2026-11-10T20:00:00Z"}]},
				"c@example.com": {"errors": [{"domain": "global", "reason": "notFound"}], "busy": []}
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	var stdout, stderr bytes.Buffer
	ctx := withCalendarTestService(outfmt.WithMode(newCmdRuntimeOutputContext(t, &stdout, &stderr), mode), newCalendarServiceFromServer(t, srv))
	if err := runKong(t, &CalendarFreeBusyCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("freebusy: %v", err)
	}
	return stdout.String(), stderr.String()
}

func TestCalendarFreeBusyCmd_WarnsWhenACalendarHasErrors(t *testing.T) {
	_, stderr := runFreeBusyWithErrors(t, outfmt.Mode{}, "a@example.com,c@example.com,b@example.com",
		"--from", "2026-11-10T16:00:00Z", "--to", "2026-11-10T22:00:00Z")
	if !strings.Contains(stderr, "c@example.com") || !strings.Contains(stderr, "notFound") {
		t.Fatalf("stderr = %q, want a warning naming c@example.com and notFound", stderr)
	}
	if strings.Contains(stderr, "a@example.com") || strings.Contains(stderr, "b@example.com") {
		t.Fatalf("stderr = %q, warned about calendars without errors", stderr)
	}
}

func TestCalendarFreeBusyCmd_PlainRowsFollowRequestedOrder(t *testing.T) {
	for range 20 {
		stdout, _ := runFreeBusyWithErrors(t, outfmt.Mode{Plain: true}, "a@example.com,c@example.com,b@example.com",
			"--from", "2026-11-10T16:00:00Z", "--to", "2026-11-10T22:00:00Z")
		want := "CALENDAR\tSTART\tEND\n" +
			"a@example.com\t2026-11-10T17:00:00Z\t2026-11-10T18:00:00Z\n" +
			"b@example.com\t2026-11-10T19:00:00Z\t2026-11-10T20:00:00Z\n"
		if stdout != want {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
}

func TestCalendarFreeBusyCmd_JSONKeepsErrors(t *testing.T) {
	stdout, _ := runFreeBusyWithErrors(t, outfmt.Mode{JSON: true}, "c@example.com",
		"--from", "2026-11-10T16:00:00Z", "--to", "2026-11-10T22:00:00Z")
	var parsed struct {
		Calendars map[string]struct {
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"calendars"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("json: %v (%q)", err, stdout)
	}
	if got := parsed.Calendars["c@example.com"].Errors; len(got) != 1 || got[0].Reason != "notFound" {
		t.Fatalf("errors = %#v", got)
	}
}
