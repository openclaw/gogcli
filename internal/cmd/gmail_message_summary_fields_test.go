package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A `fields` partial-response mask silently drops anything it does not name.
// The field arrives zeroed with no API error, so a test that hands the command
// a fully-populated gmail.Message cannot see the omission — it only shows up
// against live Gmail, as an absent output field.
//
// This stub therefore honors the mask the way Gmail does: internalDate is
// returned only when the selector asks for it. Drop internalDate from
// gmailMessageSummaryFields and this test fails.
func TestGmailMessagesSearch_RequestsInternalDateForISOOutput(t *testing.T) {
	// Derived, not hand-computed: a hardcoded epoch is one typo away from
	// silently testing a different calendar day than the one named here.
	internalDateMillis := strconv.FormatInt(
		time.Date(2026, 7, 28, 3, 36, 0, 0, time.UTC).UnixMilli(), 10)

	var sawFieldsMask string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/messages") && !strings.Contains(path, "/users/me/messages/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{{"id": "m1", "threadId": "t1"}},
			})
			return
		case strings.Contains(path, "/users/me/messages/m1"):
			mask := r.URL.Query().Get("fields")
			sawFieldsMask = mask
			msg := map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "Example <no-reply@example.com>"},
						{"name": "Subject", "value": "Receipt"},
						{"name": "Date", "value": "Tue, 28 Jul 2026 03:36:00 +0000"},
					},
				},
			}
			// Gmail returns internalDate only when the mask names it.
			if strings.Contains(mask, "internalDate") {
				msg["internalDate"] = internalDateMillis
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(msg)
			return
		case strings.Contains(path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{{"id": "INBOX", "name": "INBOX", "type": "system"}},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "messages", "search", "from:example.com", "--timezone", "America/New_York"},
		svc,
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		Messages []struct {
			Date            string `json:"date"`
			InternalDateISO string `json:"internalDateIso"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("decode search json: %v\nstdout=%q", err, result.stdout)
	}
	if len(parsed.Messages) != 1 {
		t.Fatalf("expected one message, got %#v", parsed.Messages)
	}

	// The reported shape: 03:36 UTC is 11:36 PM Eastern on the PREVIOUS day.
	const want = "2026-07-27T23:36:00-04:00"
	if got := parsed.Messages[0].InternalDateISO; got != want {
		t.Fatalf("internalDateIso = %q, want %q (fields mask sent: %q)", got, want, sawFieldsMask)
	}
	// The legacy human column is unchanged.
	if got := parsed.Messages[0].Date; got != "2026-07-27 23:36" {
		t.Fatalf("date = %q, want the existing layout unchanged", got)
	}
}
