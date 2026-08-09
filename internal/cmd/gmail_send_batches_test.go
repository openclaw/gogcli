package cmd

import (
	"context"
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
