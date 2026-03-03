package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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
