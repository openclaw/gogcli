package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// newBatchModifyServer creates a test server that handles the Gmail batch modify endpoint.
// It calls validate with the decoded request body so the test can assert on it.
func newBatchModifyServer(t *testing.T, validate func(t *testing.T, req gmail.BatchModifyMessagesRequest)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		// Labels list (needed by label command for name->ID resolution)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/users/me/labels") || strings.HasSuffix(r.URL.Path, "/gmail/v1/users/me/labels")):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
					{"id": "UNREAD", "name": "UNREAD", "type": "system"},
					{"id": "TRASH", "name": "TRASH", "type": "system"},
					{"id": "Label_1", "name": "Work", "type": "user"},
					{"id": "Label_2", "name": "Personal", "type": "user"},
				},
			})
		// Batch modify
		case r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/batchModify") || strings.HasSuffix(r.URL.Path, "/messages/batchModify")):
			var body gmail.BatchModifyMessagesRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			if validate != nil {
				validate(t, body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
}

func stubGmailServiceFromServer(t *testing.T, srv *httptest.Server) {
	t.Helper()
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }
}

func makeTestContext(t *testing.T, jsonMode bool) (context.Context, *RootFlags) {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: jsonMode})
	return ctx, &RootFlags{Account: "test@example.com"}
}

// ── archive tests ────────────────────────────────────────────────────────────

func TestGmailArchiveCmd_JSON(t *testing.T) {
	srv := newBatchModifyServer(t, func(t *testing.T, req gmail.BatchModifyMessagesRequest) {
		t.Helper()
		if len(req.Ids) != 2 || req.Ids[0] != "msg1" || req.Ids[1] != "msg2" {
			t.Fatalf("unexpected ids: %v", req.Ids)
		}
		if len(req.RemoveLabelIds) != 1 || req.RemoveLabelIds[0] != "INBOX" {
			t.Fatalf("expected RemoveLabelIds=[INBOX], got %v", req.RemoveLabelIds)
		}
		if len(req.AddLabelIds) != 0 {
			t.Fatalf("expected no AddLabelIds, got %v", req.AddLabelIds)
		}
	})
	defer srv.Close()
	stubGmailServiceFromServer(t, srv)

	ctx, flags := makeTestContext(t, true)
	out := captureStdout(t, func() {
		cmd := &GmailArchiveCmd{}
		if err := runKong(t, cmd, []string{"msg1", "msg2"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Archived []string `json:"archived"`
		Count    int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Count != 2 {
		t.Fatalf("expected count=2, got %d", parsed.Count)
	}
	if len(parsed.Archived) != 2 || parsed.Archived[0] != "msg1" {
		t.Fatalf("unexpected archived: %v", parsed.Archived)
	}
}

func TestGmailArchiveCmd_Text(t *testing.T) {
	srv := newBatchModifyServer(t, nil)
	defer srv.Close()
	stubGmailServiceFromServer(t, srv)

	var buf strings.Builder
	u, err := ui.New(ui.Options{Stdout: &buf, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})
	flags := &RootFlags{Account: "test@example.com"}

	cmd := &GmailArchiveCmd{}
	if err := runKong(t, cmd, []string{"msg1"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(buf.String(), "Archived 1 messages") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}

// ── delete tests ─────────────────────────────────────────────────────────────

func TestGmailDeleteCmd_JSON(t *testing.T) {
	srv := newBatchModifyServer(t, func(t *testing.T, req gmail.BatchModifyMessagesRequest) {
		t.Helper()
		if len(req.Ids) != 1 || req.Ids[0] != "msg1" {
			t.Fatalf("unexpected ids: %v", req.Ids)
		}
		if len(req.AddLabelIds) != 1 || req.AddLabelIds[0] != "TRASH" {
			t.Fatalf("expected AddLabelIds=[TRASH], got %v", req.AddLabelIds)
		}
	})
	defer srv.Close()
	stubGmailServiceFromServer(t, srv)

	ctx, flags := makeTestContext(t, true)
	out := captureStdout(t, func() {
		cmd := &GmailDeleteCmd{}
		if err := runKong(t, cmd, []string{"msg1"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Trashed []string `json:"trashed"`
		Count   int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Count != 1 || parsed.Trashed[0] != "msg1" {
		t.Fatalf("unexpected: %+v", parsed)
	}
}

func TestGmailDeleteCmd_Text(t *testing.T) {
	srv := newBatchModifyServer(t, nil)
	defer srv.Close()
	stubGmailServiceFromServer(t, srv)

	var buf strings.Builder
	u, err := ui.New(ui.Options{Stdout: &buf, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})
	flags := &RootFlags{Account: "test@example.com"}

	cmd := &GmailDeleteCmd{}
	if err := runKong(t, cmd, []string{"msg1", "msg2", "msg3"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(buf.String(), "Moved 3 messages to Trash") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}

// ── label tests ──────────────────────────────────────────────────────────────

func TestGmailLabelCmd_JSON(t *testing.T) {
	srv := newBatchModifyServer(t, func(t *testing.T, req gmail.BatchModifyMessagesRequest) {
		t.Helper()
		if len(req.Ids) != 2 {
			t.Fatalf("expected 2 ids, got %d", len(req.Ids))
		}
		// "Work" should resolve to "Label_1"
		if len(req.AddLabelIds) != 1 || req.AddLabelIds[0] != "Label_1" {
			t.Fatalf("expected AddLabelIds=[Label_1], got %v", req.AddLabelIds)
		}
		if len(req.RemoveLabelIds) != 1 || req.RemoveLabelIds[0] != "INBOX" {
			t.Fatalf("expected RemoveLabelIds=[INBOX], got %v", req.RemoveLabelIds)
		}
	})
	defer srv.Close()
	stubGmailServiceFromServer(t, srv)

	ctx, flags := makeTestContext(t, true)
	out := captureStdout(t, func() {
		cmd := &GmailLabelCmd{}
		if err := runKong(t, cmd, []string{"msg1", "msg2", "--add", "Work", "--remove", "INBOX"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Modified      []string `json:"modified"`
		Count         int      `json:"count"`
		AddedLabels   []string `json:"addedLabels"`
		RemovedLabels []string `json:"removedLabels"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Count != 2 {
		t.Fatalf("expected count=2, got %d", parsed.Count)
	}
	if len(parsed.AddedLabels) != 1 || parsed.AddedLabels[0] != "Label_1" {
		t.Fatalf("unexpected addedLabels: %v", parsed.AddedLabels)
	}
}

func TestGmailLabelCmd_MissingFlags(t *testing.T) {
	srv := newBatchModifyServer(t, nil)
	defer srv.Close()
	stubGmailServiceFromServer(t, srv)

	ctx, flags := makeTestContext(t, false)
	cmd := &GmailLabelCmd{MessageIDs: []string{"msg1"}}
	err := cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error when no --add or --remove")
	}
	if !strings.Contains(err.Error(), "must specify --add and/or --remove") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── mark-read tests ──────────────────────────────────────────────────────────

func TestGmailMarkReadCmd_Read_JSON(t *testing.T) {
	srv := newBatchModifyServer(t, func(t *testing.T, req gmail.BatchModifyMessagesRequest) {
		t.Helper()
		if len(req.RemoveLabelIds) != 1 || req.RemoveLabelIds[0] != "UNREAD" {
			t.Fatalf("expected RemoveLabelIds=[UNREAD], got %v", req.RemoveLabelIds)
		}
		if len(req.AddLabelIds) != 0 {
			t.Fatalf("expected no AddLabelIds, got %v", req.AddLabelIds)
		}
	})
	defer srv.Close()
	stubGmailServiceFromServer(t, srv)

	ctx, flags := makeTestContext(t, true)
	out := captureStdout(t, func() {
		cmd := &GmailMarkReadCmd{}
		if err := runKong(t, cmd, []string{"msg1", "msg2"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Marked []string `json:"marked"`
		Count  int      `json:"count"`
		Status string   `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Status != "read" {
		t.Fatalf("expected status=read, got %q", parsed.Status)
	}
	if parsed.Count != 2 {
		t.Fatalf("expected count=2, got %d", parsed.Count)
	}
}

func TestGmailMarkReadCmd_Unread_JSON(t *testing.T) {
	srv := newBatchModifyServer(t, func(t *testing.T, req gmail.BatchModifyMessagesRequest) {
		t.Helper()
		if len(req.AddLabelIds) != 1 || req.AddLabelIds[0] != "UNREAD" {
			t.Fatalf("expected AddLabelIds=[UNREAD], got %v", req.AddLabelIds)
		}
		if len(req.RemoveLabelIds) != 0 {
			t.Fatalf("expected no RemoveLabelIds, got %v", req.RemoveLabelIds)
		}
	})
	defer srv.Close()
	stubGmailServiceFromServer(t, srv)

	ctx, flags := makeTestContext(t, true)
	out := captureStdout(t, func() {
		cmd := &GmailMarkReadCmd{}
		if err := runKong(t, cmd, []string{"msg1", "--unread"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Status != "unread" {
		t.Fatalf("expected status=unread, got %q", parsed.Status)
	}
}

func TestGmailMarkReadCmd_Text(t *testing.T) {
	srv := newBatchModifyServer(t, nil)
	defer srv.Close()
	stubGmailServiceFromServer(t, srv)

	var buf strings.Builder
	u, err := ui.New(ui.Options{Stdout: &buf, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})
	flags := &RootFlags{Account: "test@example.com"}

	cmd := &GmailMarkReadCmd{}
	if err := runKong(t, cmd, []string{"msg1"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(buf.String(), "Marked 1 messages as read") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}
