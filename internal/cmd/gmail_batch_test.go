package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/errfmt"
)

// gmailBatchLimitServer mimics Gmail's 1000-ID limit on batchModify
// and records the IDs of every accepted request. Requests whose 1-based index is
// failOn are rejected with HTTP 500.
func gmailBatchLimitServer(t *testing.T, failOn int) (*httptest.Server, *[][]string) {
	t.Helper()
	var requests [][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{{"id": "INBOX", "name": "INBOX", "type": "system"}},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/messages/batchModify"):
			var body struct {
				IDs []string `json:"ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "application/json")
			if len(body.IDs) > 1000 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Invalid ids: too many (max 1000)"}}`))
				return
			}
			if len(requests)+1 == failOn {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"code":500,"message":"backend down"}}`))
				return
			}
			requests = append(requests, body.IDs)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func manyMessageIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("msg%d", i)
	}
	return ids
}

func assertGmailBatchChunks(t *testing.T, got [][]string, ids []string, wantSizes ...int) {
	t.Helper()
	if len(got) != len(wantSizes) {
		t.Fatalf("got %d requests, want %d", len(got), len(wantSizes))
	}
	var flat []string
	for i, req := range got {
		if len(req) != wantSizes[i] {
			t.Fatalf("request %d has %d IDs, want %d", i, len(req), wantSizes[i])
		}
		flat = append(flat, req...)
	}
	if strings.Join(flat, ",") != strings.Join(ids, ",") {
		t.Fatal("chunks do not cover all IDs exactly once, in order")
	}
}

func TestGmailBatchModifyCmd_ChunksAt1000(t *testing.T) {
	for _, tc := range []struct {
		n     int
		sizes []int
	}{
		{1000, []int{1000}},
		{1001, []int{1000, 1}},
		{2500, []int{1000, 1000, 500}},
	} {
		t.Run(fmt.Sprint(tc.n), func(t *testing.T) {
			srv, requests := gmailBatchLimitServer(t, 0)
			ids := manyMessageIDs(tc.n)
			args := append([]string{"--json", "--account", "a@b.com", "gmail", "batch", "modify"}, ids...)
			args = append(args, "--remove", "INBOX")

			result := executeWithGmailTestService(t, args, newGmailServiceFromServer(t, srv))
			if result.err != nil {
				t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
			}
			assertGmailBatchChunks(t, *requests, ids, tc.sizes...)

			var parsed struct {
				Modified []string `json:"modified"`
				Count    int      `json:"count"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
				t.Fatalf("json parse: %v", err)
			}
			if parsed.Count != tc.n || len(parsed.Modified) != tc.n {
				t.Fatalf("count=%d modified=%d, want %d", parsed.Count, len(parsed.Modified), tc.n)
			}
		})
	}
}

func TestGmailBatchModifyCmd_LaterChunkFailureReportsOffset(t *testing.T) {
	srv, requests := gmailBatchLimitServer(t, 2)
	args := append([]string{"--json", "--account", "a@b.com", "gmail", "batch", "modify"}, manyMessageIDs(1500)...)
	args = append(args, "--remove", "INBOX")

	result := executeWithGmailTestService(t, args, newGmailServiceFromServer(t, srv))
	if result.err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(errfmt.Format(result.err), "offset 1000") || !strings.Contains(errfmt.Format(result.err), "backend down") {
		t.Fatalf("unexpected error: %v", result.err)
	}
	if len(*requests) != 1 {
		t.Fatalf("got %d accepted requests, want 1", len(*requests))
	}
	if strings.Contains(result.stdout, `"modified"`) {
		t.Fatalf("success output on failure: %q", result.stdout)
	}
}

func TestGmailBatchModifyCmd_FirstChunkFailureKeepsError(t *testing.T) {
	srv, _ := gmailBatchLimitServer(t, 1)
	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "batch", "modify", "msg1", "--remove", "INBOX"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err == nil || strings.Contains(result.err.Error(), "offset") {
		t.Fatalf("want the bare API error, got: %v", result.err)
	}
}
