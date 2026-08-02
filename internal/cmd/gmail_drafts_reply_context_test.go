package cmd

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

// draftReplyContextServer stands up a Gmail stub for the drafts-update reply
// context tests. existingHeaders describes the draft as it is stored today;
// threads maps a thread id to the messages the API would return for it.
type draftReplyContextServer struct {
	existingHeaders []map[string]any
	existingThread  string
	threads         map[string][]map[string]any

	posted        gmail.Draft
	threadFetches []string
	messageFetch  map[string]bool
}

func newDraftReplyContextServer(t *testing.T, cfg *draftReplyContextServer) *httptest.Server {
	t.Helper()
	cfg.messageFetch = map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "d1",
				"message": map[string]any{
					"id":       "mDraft",
					"threadId": cfg.existingThread,
					"labelIds": []string{"DRAFT"},
					"payload":  map[string]any{"headers": cfg.existingHeaders},
				},
			})
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/") && r.Method == http.MethodGet:
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			cfg.threadFetches = append(cfg.threadFetches, id)
			msgs, ok := cfg.threads[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "messages": msgs})
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") && r.Method == http.MethodGet:
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			cfg.messageFetch[id] = true
			for _, msgs := range cfg.threads {
				for _, m := range msgs {
					if m["id"] == id {
						_ = json.NewEncoder(w).Encode(m)
						return
					}
				}
			}
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/gmail/v1/users/me/drafts/d1") && r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &cfg.posted); err != nil {
				t.Fatalf("unmarshal posted draft: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "d1",
				"message": map[string]any{"id": "mNew", "threadId": cfg.existingThread},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (c *draftReplyContextServer) rawPosted(t *testing.T) string {
	t.Helper()
	if c.posted.Message == nil {
		t.Fatal("no draft posted")
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.posted.Message.Raw)
	if err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	return string(raw)
}

func runReplyCtxUpdate(t *testing.T, srv *httptest.Server, stdout io.Writer, args ...string) error {
	t.Helper()
	svc := newGmailServiceFromServer(t, srv)
	flags := &RootFlags{Account: "me@example.com"}
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, stdout, io.Discard), svc)
	return runKong(t, &GmailDraftsUpdateCmd{}, args, ctx, flags)
}

// headerLines returns the values of every occurrence of a header in a raw
// RFC822 message, so a test can assert on absence as well as exact content.
func headerLines(raw, name string) []string {
	var out []string
	prefix := name + ": "
	for _, line := range strings.Split(raw, "\r\n") {
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(line, prefix) {
			out = append(out, strings.TrimPrefix(line, prefix))
		}
	}
	return out
}

// A draft that is not a reply must not gain reply headers when it is updated.
// Regression: the update path fed the draft's own threadId back in as a reply
// target, so In-Reply-To pointed at the draft's own previous revision — a
// message that was never sent.
func TestGmailDraftsUpdateCmd_NonReplyDraftGainsNoReplyHeaders(t *testing.T) {
	cfg := &draftReplyContextServer{
		existingThread: "tSolo",
		existingHeaders: []map[string]any{
			{"name": "Message-ID", "value": "<self@mail.gmail.com>"},
			{"name": "To", "value": "a@example.com"},
			{"name": "Subject", "value": "Hi"},
		},
		threads: map[string][]map[string]any{
			"tSolo": {{
				"id": "mDraft", "threadId": "tSolo", "internalDate": "1000",
				"labelIds": []string{"DRAFT"},
				"payload": map[string]any{"headers": []map[string]any{
					{"name": "Message-ID", "value": "<self@mail.gmail.com>"},
				}},
			}},
		},
	}
	srv := newDraftReplyContextServer(t, cfg)

	if err := runReplyCtxUpdate(t, srv, io.Discard, "d1", "--subject", "Hi", "--body", "Updated"); err != nil {
		t.Fatalf("execute: %v", err)
	}

	raw := cfg.rawPosted(t)
	if got := headerLines(raw, "In-Reply-To"); len(got) != 0 {
		t.Fatalf("non-reply draft gained In-Reply-To %q:\n%s", got, raw)
	}
	if got := headerLines(raw, "References"); len(got) != 0 {
		t.Fatalf("non-reply draft gained References %q:\n%s", got, raw)
	}
	if cfg.posted.Message.ThreadId != "tSolo" {
		t.Fatalf("thread continuity lost: want tSolo, got %q", cfg.posted.Message.ThreadId)
	}
	if len(cfg.threadFetches) != 0 {
		t.Fatalf("update derived reply context from the draft's own thread: fetched %v", cfg.threadFetches)
	}
}

// Repeated updates must stay clean and keep the same thread — no chaining onto
// each successive revision.
func TestGmailDraftsUpdateCmd_RepeatedUpdatesStayClean(t *testing.T) {
	for i := range 3 {
		cfg := &draftReplyContextServer{
			existingThread: "tSolo",
			existingHeaders: []map[string]any{
				// Each round the stored draft is the previous revision, with a
				// fresh Message-Id, exactly as Gmail rewrites it.
				{"name": "Message-ID", "value": "<rev" + string(rune('0'+i)) + "@mail.gmail.com>"},
				{"name": "To", "value": "a@example.com"},
			},
			threads: map[string][]map[string]any{},
		}
		srv := newDraftReplyContextServer(t, cfg)
		if err := runReplyCtxUpdate(t, srv, io.Discard, "d1", "--subject", "Hi", "--body", "Updated"); err != nil {
			t.Fatalf("round %d: execute: %v", i, err)
		}
		raw := cfg.rawPosted(t)
		if got := headerLines(raw, "In-Reply-To"); len(got) != 0 {
			t.Fatalf("round %d gained In-Reply-To %q", i, got)
		}
		if got := headerLines(raw, "References"); len(got) != 0 {
			t.Fatalf("round %d gained References %q", i, got)
		}
		if cfg.posted.Message.ThreadId != "tSolo" {
			t.Fatalf("round %d thread drifted: %q", i, cfg.posted.Message.ThreadId)
		}
	}
}

// A draft created as a genuine reply keeps that reply context across an update
// that does not re-specify it — and keeps pointing at the real parent, not the
// draft's own prior revision.
func TestGmailDraftsUpdateCmd_GenuineReplyContextPreserved(t *testing.T) {
	cfg := &draftReplyContextServer{
		existingThread: "tReal",
		existingHeaders: []map[string]any{
			{"name": "Message-ID", "value": "<self@mail.gmail.com>"},
			{"name": "In-Reply-To", "value": "<orig@example.com>"},
			{"name": "References", "value": "<orig@example.com>"},
			{"name": "To", "value": "alice@example.com"},
		},
		threads: map[string][]map[string]any{},
	}
	srv := newDraftReplyContextServer(t, cfg)

	if err := runReplyCtxUpdate(t, srv, io.Discard, "d1", "--subject", "Re: hi", "--body", "Updated"); err != nil {
		t.Fatalf("execute: %v", err)
	}

	raw := cfg.rawPosted(t)
	if got := headerLines(raw, "In-Reply-To"); len(got) != 1 || got[0] != "<orig@example.com>" {
		t.Fatalf("genuine reply context not preserved: In-Reply-To %q\n%s", got, raw)
	}
	if got := headerLines(raw, "References"); len(got) != 1 || got[0] != "<orig@example.com>" {
		t.Fatalf("References not preserved verbatim: %q\n%s", got, raw)
	}
	if strings.Contains(raw, "<self@mail.gmail.com>") {
		t.Fatalf("draft referenced its own Message-Id:\n%s", raw)
	}
}

// Updating a real reply repeatedly must not grow References.
func TestGmailDraftsUpdateCmd_ReferencesDoNotAccumulate(t *testing.T) {
	const refs = "<a@example.com> <b@example.com>"
	for i := range 3 {
		cfg := &draftReplyContextServer{
			existingThread: "tReal",
			existingHeaders: []map[string]any{
				{"name": "Message-ID", "value": "<rev" + string(rune('0'+i)) + "@mail.gmail.com>"},
				{"name": "In-Reply-To", "value": "<b@example.com>"},
				{"name": "References", "value": refs},
				{"name": "To", "value": "alice@example.com"},
			},
			threads: map[string][]map[string]any{},
		}
		srv := newDraftReplyContextServer(t, cfg)
		if err := runReplyCtxUpdate(t, srv, io.Discard, "d1", "--subject", "Re: hi", "--body", "Updated"); err != nil {
			t.Fatalf("round %d: execute: %v", i, err)
		}
		raw := cfg.rawPosted(t)
		if got := headerLines(raw, "References"); len(got) != 1 || got[0] != refs {
			t.Fatalf("round %d: References drifted to %q, want %q", i, got, refs)
		}
		if got := headerLines(raw, "In-Reply-To"); len(got) != 1 || got[0] != "<b@example.com>" {
			t.Fatalf("round %d: In-Reply-To drifted to %q", i, got)
		}
	}
}

// --clear-reply-context strips reply headers from an existing draft in place,
// so recovering a mis-threaded draft does not require delete-and-recreate.
func TestGmailDraftsUpdateCmd_ClearReplyContext(t *testing.T) {
	cfg := &draftReplyContextServer{
		existingThread: "tReal",
		existingHeaders: []map[string]any{
			{"name": "Message-ID", "value": "<self@mail.gmail.com>"},
			{"name": "In-Reply-To", "value": "<orig@example.com>"},
			{"name": "References", "value": "<orig@example.com>"},
			{"name": "To", "value": "alice@example.com"},
		},
		threads: map[string][]map[string]any{},
	}
	srv := newDraftReplyContextServer(t, cfg)

	if err := runReplyCtxUpdate(t, srv, io.Discard,
		"d1", "--subject", "Standalone", "--body", "Updated", "--clear-reply-context"); err != nil {
		t.Fatalf("execute: %v", err)
	}

	raw := cfg.rawPosted(t)
	if got := headerLines(raw, "In-Reply-To"); len(got) != 0 {
		t.Fatalf("--clear-reply-context left In-Reply-To %q", got)
	}
	if got := headerLines(raw, "References"); len(got) != 0 {
		t.Fatalf("--clear-reply-context left References %q", got)
	}
	if cfg.posted.Message.ThreadId != "tReal" {
		t.Fatalf("--clear-reply-context should keep the thread, got %q", cfg.posted.Message.ThreadId)
	}
}

func TestGmailDraftsUpdateCmd_ClearReplyContextConflictsWithReplyTarget(t *testing.T) {
	for _, target := range [][]string{
		{"--reply-to-message-id", "m1"},
		{"--thread-id", "t1"},
		{"--quote"},
	} {
		args := append([]string{"d1", "--subject", "S", "--body", "B", "--clear-reply-context"}, target...)
		flags := &RootFlags{Account: "me@example.com"}
		ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
		err := runKong(t, &GmailDraftsUpdateCmd{}, args, ctx, flags)
		if err == nil || !strings.Contains(err.Error(), "--clear-reply-context") {
			t.Fatalf("%v: expected mutual-exclusion error, got %v", target, err)
		}
	}
}

// An explicit --reply-to-message-id pointing at a draft is refused rather than
// silently producing a reference to an unsent message.
func TestGmailDraftsUpdateCmd_ReplyToDraftMessageRejected(t *testing.T) {
	cfg := &draftReplyContextServer{
		existingThread: "tReal",
		existingHeaders: []map[string]any{
			{"name": "Message-ID", "value": "<self@mail.gmail.com>"},
			{"name": "To", "value": "alice@example.com"},
		},
		threads: map[string][]map[string]any{
			"tOther": {{
				"id": "mOtherDraft", "threadId": "tOther", "internalDate": "2000",
				"labelIds": []string{"DRAFT"},
				"payload": map[string]any{"headers": []map[string]any{
					{"name": "Message-ID", "value": "<otherdraft@mail.gmail.com>"},
				}},
			}},
		},
	}
	srv := newDraftReplyContextServer(t, cfg)

	err := runReplyCtxUpdate(t, srv, io.Discard,
		"d1", "--subject", "S", "--body", "B", "--reply-to-message-id", "mOtherDraft")
	if err == nil || !strings.Contains(err.Error(), "draft") {
		t.Fatalf("expected refusal to reply to a draft, got %v", err)
	}
}

// A caller-supplied --thread-id whose newest message is a draft must anchor to
// the newest real message instead of the draft.
func TestGmailDraftsUpdateCmd_ThreadIDSkipsDraftMessages(t *testing.T) {
	cfg := &draftReplyContextServer{
		existingThread: "tOld",
		existingHeaders: []map[string]any{
			{"name": "Message-ID", "value": "<self@mail.gmail.com>"},
			{"name": "To", "value": "alice@example.com"},
		},
		threads: map[string][]map[string]any{
			"tMixed": {
				{
					"id": "mReal", "threadId": "tMixed", "internalDate": "1000",
					"labelIds": []string{"INBOX"},
					"payload": map[string]any{"headers": []map[string]any{
						{"name": "Message-ID", "value": "<real@example.com>"},
					}},
				},
				{
					"id": "mDraftInThread", "threadId": "tMixed", "internalDate": "9000",
					"labelIds": []string{"DRAFT"},
					"payload": map[string]any{"headers": []map[string]any{
						{"name": "Message-ID", "value": "<threaddraft@mail.gmail.com>"},
					}},
				},
			},
		},
	}
	srv := newDraftReplyContextServer(t, cfg)

	if err := runReplyCtxUpdate(t, srv, io.Discard,
		"d1", "--subject", "S", "--body", "B", "--thread-id", "tMixed"); err != nil {
		t.Fatalf("execute: %v", err)
	}

	raw := cfg.rawPosted(t)
	if got := headerLines(raw, "In-Reply-To"); len(got) != 1 || got[0] != "<real@example.com>" {
		t.Fatalf("anchored to a draft instead of the newest real message: %q\n%s", got, raw)
	}
	if strings.Contains(raw, "<threaddraft@mail.gmail.com>") {
		t.Fatalf("referenced a draft Message-Id:\n%s", raw)
	}
}

// A thread containing nothing but drafts has no valid reply target.
func TestGmailDraftsUpdateCmd_ThreadIDWithOnlyDraftsRejected(t *testing.T) {
	cfg := &draftReplyContextServer{
		existingThread: "tOld",
		existingHeaders: []map[string]any{
			{"name": "Message-ID", "value": "<self@mail.gmail.com>"},
			{"name": "To", "value": "alice@example.com"},
		},
		threads: map[string][]map[string]any{
			"tDraftsOnly": {{
				"id": "mOnlyDraft", "threadId": "tDraftsOnly", "internalDate": "1000",
				"labelIds": []string{"DRAFT"},
				"payload": map[string]any{"headers": []map[string]any{
					{"name": "Message-ID", "value": "<onlydraft@mail.gmail.com>"},
				}},
			}},
		},
	}
	srv := newDraftReplyContextServer(t, cfg)

	err := runReplyCtxUpdate(t, srv, io.Discard,
		"d1", "--subject", "S", "--body", "B", "--thread-id", "tDraftsOnly")
	if err == nil || !strings.Contains(err.Error(), "draft") {
		t.Fatalf("expected refusal on a drafts-only thread, got %v", err)
	}
}

// The JSON result reports the effective reply context so a caller can verify
// threading without re-fetching raw headers.
func TestGmailDraftsUpdateCmd_ResultReportsReplyContext(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		cfg := &draftReplyContextServer{
			existingThread: "tSolo",
			existingHeaders: []map[string]any{
				{"name": "Message-ID", "value": "<self@mail.gmail.com>"},
				{"name": "To", "value": "a@example.com"},
			},
			threads: map[string][]map[string]any{},
		}
		srv := newDraftReplyContextServer(t, cfg)
		var out strings.Builder
		if err := runReplyCtxUpdate(t, srv, &out, "d1", "--subject", "S", "--body", "B"); err != nil {
			t.Fatalf("execute: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
			t.Fatalf("decode result %q: %v", out.String(), err)
		}
		if v, ok := got["inReplyTo"]; !ok || v != nil {
			t.Fatalf("want explicit null inReplyTo, got %#v (present=%v)", v, ok)
		}
		if v, ok := got["references"]; !ok || v != nil {
			t.Fatalf("want explicit null references, got %#v (present=%v)", v, ok)
		}
		if v, ok := got["replyContextSource"]; !ok || v != nil {
			t.Fatalf("want explicit null replyContextSource, got %#v (present=%v)", v, ok)
		}
	})

	t.Run("carried", func(t *testing.T) {
		cfg := &draftReplyContextServer{
			existingThread: "tReal",
			existingHeaders: []map[string]any{
				{"name": "Message-ID", "value": "<self@mail.gmail.com>"},
				{"name": "In-Reply-To", "value": "<orig@example.com>"},
				{"name": "References", "value": "<orig@example.com>"},
				{"name": "To", "value": "a@example.com"},
			},
			threads: map[string][]map[string]any{},
		}
		srv := newDraftReplyContextServer(t, cfg)
		var out strings.Builder
		if err := runReplyCtxUpdate(t, srv, &out, "d1", "--subject", "S", "--body", "B"); err != nil {
			t.Fatalf("execute: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
			t.Fatalf("decode result %q: %v", out.String(), err)
		}
		if got["inReplyTo"] != "<orig@example.com>" {
			t.Fatalf("want carried inReplyTo, got %#v", got["inReplyTo"])
		}
		if got["references"] != "<orig@example.com>" {
			t.Fatalf("want carried references, got %#v", got["references"])
		}
		if got["replyContextSource"] != "carried" {
			t.Fatalf("want replyContextSource carried, got %#v", got["replyContextSource"])
		}
	})
}
