package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
)

func runGmailBulkDryRun(t *testing.T, cmd any, args []string) map[string]any {
	t.Helper()

	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		err := runKong(t, cmd, args, ctx, &RootFlags{DryRun: true})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 0 {
			t.Fatalf("expected dry-run exit code 0, got: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	return got
}

func requestStringSlice(t *testing.T, req map[string]any, key string) []string {
	t.Helper()

	raw, ok := req[key].([]any)
	if !ok {
		t.Fatalf("expected request.%s array, got %T", key, req[key])
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("expected string in request.%s, got %T", key, v)
		}
		out = append(out, s)
	}
	return out
}

func requireRequestMap(t *testing.T, got map[string]any) map[string]any {
	t.Helper()

	req, ok := got["request"].(map[string]any)
	if !ok {
		t.Fatalf("expected request object, got %T", got["request"])
	}
	return req
}

func TestGmailBulkOps_DryRun_UsesSpecificOpsAndLabels(t *testing.T) {
	tests := []struct {
		name       string
		cmd        any
		args       []string
		wantOp     string
		wantAdd    []string
		wantRemove []string
	}{
		{
			name:       "archive",
			cmd:        &GmailArchiveCmd{},
			args:       []string{"msg1"},
			wantOp:     "gmail.archive",
			wantAdd:    []string{},
			wantRemove: []string{"INBOX"},
		},
		{
			name:       "read",
			cmd:        &GmailReadCmd{},
			args:       []string{"msg1"},
			wantOp:     "gmail.read",
			wantAdd:    []string{},
			wantRemove: []string{"UNREAD"},
		},
		{
			name:       "unread",
			cmd:        &GmailUnreadCmd{},
			args:       []string{"msg1"},
			wantOp:     "gmail.unread",
			wantAdd:    []string{"UNREAD"},
			wantRemove: []string{},
		},
		{
			name:       "trash",
			cmd:        &GmailTrashMsgCmd{},
			args:       []string{"msg1"},
			wantOp:     "gmail.trash",
			wantAdd:    []string{"TRASH"},
			wantRemove: []string{"INBOX"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runGmailBulkDryRun(t, tt.cmd, tt.args)

			op, ok := got["op"].(string)
			if !ok || op != tt.wantOp {
				t.Fatalf("expected op=%q, got=%v", tt.wantOp, got["op"])
			}

			req := requireRequestMap(t, got)
			messageIDs := requestStringSlice(t, req, "message_ids")
			if len(messageIDs) != 1 || messageIDs[0] != "msg1" {
				t.Fatalf("unexpected request.message_ids: %v", messageIDs)
			}

			added := requestStringSlice(t, req, "added_labels")
			if len(added) != len(tt.wantAdd) {
				t.Fatalf("unexpected request.added_labels len: got=%v want=%v", added, tt.wantAdd)
			}
			for i := range tt.wantAdd {
				if added[i] != tt.wantAdd[i] {
					t.Fatalf("unexpected request.added_labels: got=%v want=%v", added, tt.wantAdd)
				}
			}

			removed := requestStringSlice(t, req, "removed_labels")
			if len(removed) != len(tt.wantRemove) {
				t.Fatalf("unexpected request.removed_labels len: got=%v want=%v", removed, tt.wantRemove)
			}
			for i := range tt.wantRemove {
				if removed[i] != tt.wantRemove[i] {
					t.Fatalf("unexpected request.removed_labels: got=%v want=%v", removed, tt.wantRemove)
				}
			}
		})
	}
}

func TestGmailArchiveCmd_DryRun_QueryMode_NoAccountRequired(t *testing.T) {
	got := runGmailBulkDryRun(t, &GmailArchiveCmd{}, []string{"--query", "is:unread", "--max", "25"})

	if op, _ := got["op"].(string); op != "gmail.archive" {
		t.Fatalf("expected op gmail.archive, got %v", got["op"])
	}

	req := requireRequestMap(t, got)
	if q, _ := req["query"].(string); q != "is:unread" {
		t.Fatalf("unexpected query: %v", req["query"])
	}
	if limit, ok := req["max"].(float64); !ok || int(limit) != 25 {
		t.Fatalf("unexpected max: %v", req["max"])
	}
	if ids := requestStringSlice(t, req, "message_ids"); len(ids) != 0 {
		t.Fatalf("expected empty message_ids, got %v", ids)
	}
}

func TestGmailArchiveCmd_DryRun_ThreadMode(t *testing.T) {
	got := runGmailBulkDryRun(t, &GmailArchiveCmd{}, []string{
		"--thread",
		"https://mail.google.com/mail/u/0/#inbox/18abc123def45678",
		"18def456abc12345",
	})

	if op, _ := got["op"].(string); op != "gmail.archive" {
		t.Fatalf("expected op gmail.archive, got %v", got["op"])
	}
	req := requireRequestMap(t, got)
	threadIDs := requestStringSlice(t, req, "thread_ids")
	if len(threadIDs) != 2 || threadIDs[0] != "18abc123def45678" || threadIDs[1] != "18def456abc12345" {
		t.Fatalf("unexpected request.thread_ids: %v", threadIDs)
	}
	if resource, _ := req["resource"].(string); resource != "thread" {
		t.Fatalf("unexpected resource: %v", req["resource"])
	}
}

func TestGmailArchiveCmd_ThreadModeRejectsQuery(t *testing.T) {
	err := runKong(t, &GmailArchiveCmd{}, []string{"--thread", "--query", "in:inbox"}, context.Background(), &RootFlags{DryRun: true})
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "--thread cannot be used with --query") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGmailArchiveCmd_ArchivesWholeThreads(t *testing.T) {
	var modified []string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		if r.Method != http.MethodPost || !strings.HasSuffix(path, "/modify") {
			http.NotFound(w, r)
			return
		}

		var req gmail.ModifyThreadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode modify request: %v", err)
		}
		if len(req.RemoveLabelIds) != 1 || req.RemoveLabelIds[0] != "INBOX" {
			t.Fatalf("unexpected remove labels: %v", req.RemoveLabelIds)
		}
		parts := strings.Split(path, "/")
		modified = append(modified, parts[len(parts)-2])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	})
	defer cleanup()
	stubGmailServiceForTest(t, svc)

	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
	out := captureStdout(t, func() {
		if err := runKong(t, &GmailArchiveCmd{}, []string{"--thread", "thread1", "thread2"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("archive threads: %v", err)
		}
	})
	if strings.Join(modified, ",") != "thread1,thread2" {
		t.Fatalf("modified threads = %v", modified)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if count, _ := got["count"].(float64); count != 2 {
		t.Fatalf("count = %v, want 2", got["count"])
	}
	if resource, _ := got["resource"].(string); resource != "thread" {
		t.Fatalf("resource = %v, want thread", got["resource"])
	}
	results, ok := got["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("results = %#v, want two entries", got["results"])
	}
}

func TestGmailArchiveCmd_ReportsPartialThreadFailures(t *testing.T) {
	var modified []string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		parts := strings.Split(path, "/")
		threadID := parts[len(parts)-2]
		modified = append(modified, threadID)
		if threadID == "thread2" {
			http.Error(w, `{"error":{"code":404,"message":"not found"}}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	})
	defer cleanup()
	stubGmailServiceForTest(t, svc)

	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
	var runErr error
	out := captureStdout(t, func() {
		runErr = runKong(t, &GmailArchiveCmd{}, []string{"--thread", "thread1", "thread2", "thread3"}, ctx, &RootFlags{Account: "a@b.com"})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "archived 2 of 3 threads; 1 failed") {
		t.Fatalf("unexpected error: %v", runErr)
	}
	if strings.Join(modified, ",") != "thread1,thread2,thread3" {
		t.Fatalf("modified threads = %v", modified)
	}

	var got struct {
		Count   int `json:"count"`
		Failed  int `json:"failed"`
		Results []struct {
			ThreadID string `json:"threadId"`
			Success  bool   `json:"success"`
			Error    string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got.Count != 2 || got.Failed != 1 || len(got.Results) != 3 {
		t.Fatalf("unexpected partial result: %#v", got)
	}
	if got.Results[1].ThreadID != "thread2" || got.Results[1].Success || got.Results[1].Error == "" {
		t.Fatalf("missing thread2 failure: %#v", got.Results[1])
	}
}

func TestGmailBulkOps_QueryInvalidMaxFailsBeforeDryRun(t *testing.T) {
	testCases := []struct {
		name string
		cmd  any
		args []string
	}{
		{name: "archive zero", cmd: &GmailArchiveCmd{}, args: []string{"--query", "is:unread", "--max", "0"}},
		{name: "archive negative", cmd: &GmailArchiveCmd{}, args: []string{"--query", "is:unread", "--max=-1"}},
		{name: "trash zero", cmd: &GmailTrashMsgCmd{}, args: []string{"--query", "is:unread", "--max", "0"}},
		{name: "read zero", cmd: &GmailReadCmd{}, args: []string{"--query", "is:unread", "--max", "0"}},
		{name: "unread zero", cmd: &GmailUnreadCmd{}, args: []string{"--query", "is:unread", "--max", "0"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
			out := captureStdout(t, func() {
				err := runKong(t, tc.cmd, tc.args, ctx, &RootFlags{DryRun: true})
				if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "--max must be > 0") {
					t.Fatalf("unexpected err: %v", err)
				}
			})
			if strings.TrimSpace(out) != "" {
				t.Fatalf("expected no dry-run output, got %q", out)
			}
		})
	}
}
