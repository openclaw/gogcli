package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/drive/v3"
)

func TestQuotedCommentDryRunWarning(t *testing.T) {
	for _, command := range [][]string{{"docs", "comments", "add"}, {"drive", "comments", "create"}} {
		for _, quote := range []string{"", " \t", "Quoted text"} {
			t.Run(strings.Join(command, "/")+"/"+quote, func(t *testing.T) {
				args := append([]string{"--json", "--dry-run"}, command...)
				args = append(args, "file1", "Comment", "--quoted", quote)
				result := executeWithDriveTestServiceFactory(t, args, func(context.Context, string) (*drive.Service, error) {
					t.Error("dry run requested authentication")
					return nil, errors.New("unexpected authentication")
				})
				if result.err != nil || !json.Valid([]byte(result.stdout)) {
					t.Fatalf("err=%v, stdout=%q, stderr=%q", result.err, result.stdout, result.stderr)
				}
				warns := strings.Contains(result.stderr, "Google Docs, Sheets, and Slides do not render Drive API comments anchored to content")
				if want := strings.TrimSpace(quote) != ""; warns != want {
					t.Fatalf("warning present=%t, want %t; stderr=%q", warns, want, result.stderr)
				}
			})
		}
	}
}

func TestQuotedCommentWarningPreservesCreateRequest(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command []string
		quote   string
		anchor  string
	}{
		{"docs quoted", []string{"docs", "comments", "add"}, "Quote", ""},
		{"docs quoted anchored", []string{"docs", "comments", "add"}, "Quote", `{"r":"head"}`},
		{"docs unquoted", []string{"docs", "comments", "add"}, "", ""},
		{"drive quoted", []string{"drive", "comments", "create"}, "Quote", ""},
		{"drive unquoted", []string{"drive", "comments", "create"}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/files/file1/comments") {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				var comment drive.Comment
				if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
					t.Error(err)
					return
				}
				if comment.Content != "Comment" || comment.Anchor != tc.anchor {
					t.Errorf("unexpected comment: %#v", comment)
				}
				if tc.quote == "" {
					if comment.QuotedFileContent != nil {
						t.Error("unexpected quoted content")
					}
				} else if comment.QuotedFileContent == nil || comment.QuotedFileContent.Value != tc.quote {
					t.Errorf("quoted content = %#v", comment.QuotedFileContent)
				}
				comment.Id = "c1"
				_ = json.NewEncoder(w).Encode(comment)
			}))
			t.Cleanup(server.Close)
			args := append([]string{"--json", "--account", "user@example.com"}, tc.command...)
			args = append(args, "file1", "Comment")
			if tc.quote != "" {
				args = append(args, "--quoted", tc.quote)
			}
			if tc.anchor != "" {
				args = append(args, "--anchor", tc.anchor)
			}
			result := executeWithDriveTestService(t, args, driveServiceFromServer(t, server))
			if result.err != nil || requests.Load() != 1 || !json.Valid([]byte(result.stdout)) {
				t.Fatalf("err=%v, requests=%d, stdout=%q, stderr=%q", result.err, requests.Load(), result.stdout, result.stderr)
			}
			wantWarnings := 0
			if tc.quote != "" {
				wantWarnings = 1
			}
			if got := strings.Count(result.stderr, "warning: --quoted"); got != wantWarnings {
				t.Fatalf("warning count=%d, want %d; stderr=%q", got, wantWarnings, result.stderr)
			}
		})
	}
}
