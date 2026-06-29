package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/tracking"
	"github.com/openclaw/gogcli/internal/ui"
)

func TestSendGmailBatches_WithTracking(t *testing.T) {
	var sendCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			sendCount++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       fmt.Sprintf("m%d", sendCount),
				"threadId": "t1",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   "https://example.com",
		TrackingKey: mustTrackingKey(t),
	}

	batches := buildSendBatches(
		[]string{"a@example.com"},
		[]string{"b@example.com"},
		nil,
		true,
		true,
	)
	results, err := sendGmailBatches(context.Background(), svc, sendMessageOptions{
		FromAddr:    "me@example.com",
		Subject:     "Hello",
		BodyHTML:    "<html><body>Hi</body></html>",
		Track:       true,
		TrackingCfg: cfg,
	}, batches)
	if err != nil {
		t.Fatalf("sendGmailBatches: %v", err)
	}
	if len(results) != len(batches) {
		t.Fatalf("expected %d results, got %d", len(batches), len(results))
	}
	for _, res := range results {
		if res.MessageID == "" || res.TrackingID == "" {
			t.Fatalf("missing result fields: %#v", res)
		}
	}
}

// TestSendGmailBatches_TrackSplitBareTrackingRecipient proves a track-split
// send correlates each tracking pixel to the bare email address even when the
// recipient was typed as a formatted "Name" <a@x.com> mailbox: the message's
// To header and the reported result keep the formatted mailbox, while the
// encrypted pixel payload carries only the address, so opens correlate
// regardless of how the recipient was typed.
func TestSendGmailBatches_TrackSplitBareTrackingRecipient(t *testing.T) {
	var raws []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/users/me/messages/send") {
			http.NotFound(w, r)
			return
		}
		var msg gmail.Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("decode sent message: %v", err)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(msg.Raw)
		if err != nil {
			t.Errorf("decode raw: %v", err)
		}
		raws = append(raws, string(decoded))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": fmt.Sprintf("m%d", len(raws)), "threadId": "t1"})
	}))
	defer srv.Close()

	key := mustTrackingKey(t)
	batches := buildSendBatches(
		[]string{`"Smith, John" <john@example.com>`, "other@example.com"},
		nil,
		nil,
		true,
		true,
	)
	results, err := sendGmailBatches(context.Background(), newGmailServiceFromServer(t, srv), sendMessageOptions{
		FromAddr: "me@example.com",
		Subject:  "Hello",
		BodyHTML: "<html><body>Hi</body></html>",
		Track:    true,
		TrackingCfg: &tracking.Config{
			Enabled:     true,
			WorkerURL:   "https://example.com",
			TrackingKey: key,
		},
	}, batches)
	if err != nil {
		t.Fatalf("sendGmailBatches: %v", err)
	}
	if len(results) != 2 || len(raws) != 2 {
		t.Fatalf("expected 2 results and 2 sent messages, got %d and %d", len(results), len(raws))
	}

	want := []struct {
		formatted string
		header    wantAddr
		bare      string
	}{
		{
			formatted: `"Smith, John" <john@example.com>`,
			header:    wantAddr{name: "Smith, John", address: "john@example.com"},
			bare:      "john@example.com",
		},
		{
			formatted: "other@example.com",
			header:    wantAddr{address: "other@example.com"},
			bare:      "other@example.com",
		},
	}
	for i, w := range want {
		payload, decErr := tracking.Decrypt(results[i].TrackingID, key)
		if decErr != nil {
			t.Fatalf("decrypt tracking blob %d: %v", i, decErr)
		}
		if payload.Recipient != w.bare {
			t.Errorf("tracking recipient[%d] = %q, want bare %q", i, payload.Recipient, w.bare)
		}
		if results[i].To != w.formatted {
			t.Errorf("results[%d].To = %q, want formatted %q", i, results[i].To, w.formatted)
		}
		assertHeaderRecipients(t, raws[i], "To", []wantAddr{w.header})
	}
}

// TestSendGmailBatches_NonSplitTrackedBarePixelRecipient proves the non-split
// tracked path (single recipient, no --track-split) also bakes the bare email
// address into the encrypted pixel payload when the recipient was typed as a
// formatted mailbox.
func TestSendGmailBatches_NonSplitTrackedBarePixelRecipient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/users/me/messages/send") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "m1", "threadId": "t1"})
	}))
	defer srv.Close()

	key := mustTrackingKey(t)
	batches := buildSendBatches([]string{`"Smith, John" <john@example.com>`}, nil, nil, true, false)
	results, err := sendGmailBatches(context.Background(), newGmailServiceFromServer(t, srv), sendMessageOptions{
		FromAddr: "me@example.com",
		Subject:  "Hello",
		BodyHTML: "<html><body>Hi</body></html>",
		Track:    true,
		TrackingCfg: &tracking.Config{
			Enabled:     true,
			WorkerURL:   "https://example.com",
			TrackingKey: key,
		},
	}, batches)
	if err != nil {
		t.Fatalf("sendGmailBatches: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	payload, err := tracking.Decrypt(results[0].TrackingID, key)
	if err != nil {
		t.Fatalf("decrypt tracking blob: %v", err)
	}
	if payload.Recipient != "john@example.com" {
		t.Fatalf("pixel recipient = %q, want bare john@example.com", payload.Recipient)
	}
}

// TestBuildSendBatches_TrackSplitDedupesFormattedAndBare proves the
// track-split dedup keys on the canonical bare address across to/cc/bcc:
// "Bob" <a@x.com> in --to and A@X.COM in --cc are the same person and yield
// one batch, keeping the first-seen spelling.
func TestBuildSendBatches_TrackSplitDedupesFormattedAndBare(t *testing.T) {
	batches := buildSendBatches([]string{`"Bob" <a@x.com>`}, []string{"A@X.COM"}, nil, true, true)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch after cross-field dedup, got %d: %#v", len(batches), batches)
	}
	if got := batches[0].To; len(got) != 1 || got[0] != `"Bob" <a@x.com>` {
		t.Fatalf("expected first-seen formatted mailbox, got %#v", got)
	}
	if batches[0].TrackingRecipient != "a@x.com" {
		t.Fatalf("tracking recipient = %q, want bare a@x.com", batches[0].TrackingRecipient)
	}
}

func TestReplyHeaders_Message(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/messages/m1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<m1>"},
						{"name": "References", "value": "<ref1>"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	inReplyTo, references, threadID, err := replyHeaders(context.Background(), svc, "m1")
	if err != nil {
		t.Fatalf("replyHeaders: %v", err)
	}
	if inReplyTo != "<m1>" || references == "" || threadID != "t1" {
		t.Fatalf("unexpected reply headers: %q %q %q", inReplyTo, references, threadID)
	}
}

func TestWriteSendResults_TextMultiple(t *testing.T) {
	out := captureStdout(t, func() {
		u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if err != nil {
			t.Fatalf("ui.New: %v", err)
		}
		ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: false})

		if err := writeSendResults(ctx, u, "from@example.com", []sendResult{
			{MessageID: "m1", ThreadID: "t1", TrackingID: "trk1", To: "a@example.com"},
			{MessageID: "m2", ThreadID: "t2", TrackingID: "trk2", To: "b@example.com"},
		}, nil); err != nil {
			t.Fatalf("writeSendResults: %v", err)
		}
	})
	if !strings.Contains(out, "message_id") || !strings.Contains(out, "tracking_id") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func mustTrackingKey(t *testing.T) string {
	t.Helper()
	key, err := tracking.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return key
}
