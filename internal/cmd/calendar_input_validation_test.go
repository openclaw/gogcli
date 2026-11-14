package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCalendarUpdateRejectsMalformedAttendeeClear(t *testing.T) {
	for _, value := range []string{";optional", ";resource", ",,", ";optional,;resource"} {
		t.Run(value, func(t *testing.T) {
			patch, _, err := buildCalendarUpdatePatch(calendarUpdateInput{Attendees: value}, calendarUpdateFields{Attendees: true})
			if err == nil {
				b, _ := json.Marshal(patch)
				t.Fatalf("malformed attendees accepted: %s", b)
			}
		})
	}
	for _, value := range []string{"", "   "} {
		patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInput{Attendees: value}, calendarUpdateFields{Attendees: true})
		if err != nil || !changed {
			t.Fatalf("explicit clear: changed=%v err=%v", changed, err)
		}
		b, _ := json.Marshal(patch)
		if string(b) != `{"attendees":[]}` {
			t.Fatalf("clear patch=%s", b)
		}
	}
}

func TestCalendarFreeBusyRejectsInvalidInstantWindow(t *testing.T) {
	for _, to := range []string{"2027-01-01T00:00:00Z", "2027-01-02T01:00:00+01:00"} {
		c := CalendarFreeBusyCmd{From: "2027-01-02T00:00:00Z", To: to}
		if _, _, err := c.resolveFreeBusyRange(context.Background(), nil); err == nil {
			t.Fatalf("accepted invalid window %s to %s", c.From, c.To)
		}
	}
	c := CalendarFreeBusyCmd{From: "2027-01-02T00:00:00.123Z", To: "2027-01-02T01:00:00+00:00"}
	from, to, err := c.resolveFreeBusyRange(context.Background(), nil)
	if err != nil || from != c.From || to != c.To {
		t.Fatalf("valid instants changed: %q %q %v", from, to, err)
	}
}

func TestCalendarFreeBusyResolvesNameContainingAt(t *testing.T) {
	var ids []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/calendarList") {
			_, _ = io.WriteString(w, `{"items":[{"id":"work@group.calendar.google.com","summary":"Work @ Home"}]}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/freeBusy") {
			var req struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			for _, item := range req.Items {
				ids = append(ids, item.ID)
			}
			_, _ = io.WriteString(w, `{"calendars":{}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	res := executeWithCalendarTestService(t, []string{"--home=" + t.TempDir(), "--account=synthetic@example.invalid", "--json", "calendar", "freebusy", "--cal=Work @ Home", "--from=2027-01-01T00:00:00Z", "--to=2027-01-02T00:00:00Z"}, newCalendarServiceFromServer(t, srv))
	if res.err != nil {
		t.Fatal(res.err)
	}
	if len(ids) != 1 || ids[0] != "work@group.calendar.google.com" {
		t.Fatalf("calendar IDs=%v", ids)
	}
}
