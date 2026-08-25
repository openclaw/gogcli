package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/googleapi"
)

// writeQuoteThreadT1 serves thread t1's metadata: one non-draft message m1,
// which a quoting reply must then fetch again in full format.
func writeQuoteThreadT1(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":       "t1",
		"messages": []map[string]any{{"id": "m1", "threadId": "t1", "internalDate": "1000"}},
	})
}

// newQuoteFetchFailHandler serves thread t1's metadata successfully but fails
// the follow-up full-format fetch of its message m1 that quoting requires. Any
// POST fails the test, proving no message or draft was composed. The POST case
// must stay first: /messages/send also matches the message-fetch prefix.
func newQuoteFetchFailHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			t.Errorf("unexpected POST %s after quote fetch failure", r.URL.Path)
			http.Error(w, "unexpected compose", http.StatusInternalServerError)
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/threads/"):
			writeQuoteThreadT1(w)
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/"):
			if r.URL.Query().Get("format") != "full" {
				t.Errorf("expected message format=full, got %q", r.URL.RawQuery)
			}
			http.Error(w, `{"error":{"code":500,"message":"backend failed"}}`, http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}
}

// A requested quote must fail closed: if the full-format fetch of the reply
// target fails, fetchReplyInfo must error rather than silently fall back to
// the metadata-only message (which has no body to quote).
func TestFetchReplyInfo_ThreadIDQuote_FullFetchFailurePropagates(t *testing.T) {
	svc, cleanup := newGmailServiceForTest(t, newQuoteFetchFailHandler(t))
	defer cleanup()

	_, err := fetchReplyInfo(context.Background(), svc, "", "t1", true)
	if err == nil {
		t.Fatal("expected error when full-format fetch fails")
	}
	if !strings.Contains(err.Error(), "for quoting") {
		t.Fatalf("error should identify the quote fetch: %v", err)
	}
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) || apiErr.Code != http.StatusInternalServerError {
		t.Fatalf("expected wrapped googleapi 500 as the cause, got %v", err)
	}
}

func TestGmailSendCmd_ThreadIDQuote_FullFetchFailureAbortsSend(t *testing.T) {
	svc, cleanup := newGmailServiceForTest(t, newQuoteFetchFailHandler(t))
	defer cleanup()

	cmd := &GmailSendCmd{To: "a@example.com", Body: "Hello", ThreadID: "t1", Quote: true}
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"})
	if err == nil {
		t.Fatal("expected send to fail when the quote source cannot be fetched")
	}
	if !strings.Contains(err.Error(), "for quoting") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The generated client can return (nil, nil) when a proxy answers 200 with a
// literal null body; both reply-target paths must treat that as an error, not
// fall back to a message with no body to quote.
func TestFetchReplyInfo_Quote_NullResponseFailsClosed(t *testing.T) {
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/threads/"):
			writeQuoteThreadT1(w)
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("null"))
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	cases := []struct {
		name      string
		messageID string
		threadID  string
	}{
		{"message-id path", "m1", ""},
		{"thread-id path", "", "t1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fetchReplyInfo(context.Background(), svc, tc.messageID, tc.threadID, true)
			if err == nil || !strings.Contains(err.Error(), "empty response") {
				t.Fatalf("expected empty-response error, got %v", err)
			}
		})
	}
}

// Forwarding fetches the original message the same way; a null response must
// error, not panic on the nil message.
func TestBuildForwardComposeMessage_NullResponseFailsClosed(t *testing.T) {
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("null"))
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()

	opts := &GmailForwardOptions{To: "a@example.com"}
	ctx := newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard)
	_, err := opts.buildForwardComposeMessage(ctx, svc, "a@b.com", forwardComposeInputs{messageID: "m1"})
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("expected empty-response error, got %v", err)
	}
}

// When the original fetches fine but yields no quotable text (e.g. an
// attachment-only message), the compose proceeds but must say so on stderr —
// and only on stderr, keeping JSON stdout parseable — instead of dropping the
// requested quote silently.
func TestGmailSendCmd_ThreadIDQuote_NoQuotableTextWarns(t *testing.T) {
	var rawSent string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/messages/send"):
			rawSent, _ = handleFinalizeRaw(t, w, r, "/gmail/v1/users/me/messages/send")
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/threads/"):
			writeQuoteThreadT1(w)
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "m1", "threadId": "t1",
				"payload": map[string]any{
					"mimeType": "multipart/mixed",
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<id1@example.com>"},
						{"name": "From", "value": "sender@example.com"},
					},
					"parts": []map[string]any{{
						"mimeType": "application/pdf",
						"filename": "report.pdf",
						"body":     map[string]any{"attachmentId": "att1"},
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	var out, errOut bytes.Buffer
	cmd := &GmailSendCmd{To: "a@example.com", Body: "Hello", ThreadID: "t1", Quote: true}
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, &out, &errOut), svc)
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errOut.String(), "quotable text") {
		t.Fatalf("expected no-quotable-text warning on stderr, got %q", errOut.String())
	}
	if strings.Contains(out.String(), "quotable text") {
		t.Fatalf("warning leaked into stdout: %q", out.String())
	}
	if rawSent == "" {
		t.Fatal("expected message to be sent")
	}
	if strings.Contains(rawSent, "wrote:") {
		t.Fatalf("unexpected quote block in sent message:\n%s", rawSent)
	}
}

// The warning must stay silent when the original has quotable text: the quote
// appears in the sent message and stderr stays clean.
func TestGmailSendCmd_ThreadIDQuote_QuotesWithoutWarning(t *testing.T) {
	var rawSent string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/messages/send"):
			rawSent, _ = handleFinalizeRaw(t, w, r, "/gmail/v1/users/me/messages/send")
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/threads/"):
			writeQuoteThreadT1(w)
		case strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "m1", "threadId": "t1",
				"payload": map[string]any{
					"mimeType": "text/plain",
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<id1@example.com>"},
						{"name": "From", "value": "sender@example.com"},
						{"name": "Date", "value": "Mon, 1 Jan 2024 00:00:00 +0000"},
					},
					"body": map[string]any{
						"data": base64.RawURLEncoding.EncodeToString([]byte("original text")),
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	var errOut bytes.Buffer
	cmd := &GmailSendCmd{To: "a@example.com", Body: "Hello", ThreadID: "t1", Quote: true}
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, &errOut), svc)
	if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(errOut.String(), "quotable text") {
		t.Fatalf("unexpected warning for quotable original: %q", errOut.String())
	}
	if !strings.Contains(rawSent, "wrote:") || !strings.Contains(rawSent, "> original text") {
		t.Fatalf("expected quoted original in sent message:\n%s", rawSent)
	}
}

func TestGmailDraftsCreateCmd_ThreadIDQuote_FullFetchFailureAbortsCreate(t *testing.T) {
	svc, cleanup := newGmailServiceForTest(t, newQuoteFetchFailHandler(t))
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	err := runKong(t, &GmailDraftsCreateCmd{}, []string{
		"--to", "a@example.com", "--body", "Hello", "--thread-id", "t1", "--quote",
	}, ctx, flags)
	if err == nil {
		t.Fatal("expected draft create to fail when the quote source cannot be fetched")
	}
	if !strings.Contains(err.Error(), "for quoting") {
		t.Fatalf("unexpected error: %v", err)
	}
}
