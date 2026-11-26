package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
)

func TestDocsSuggestionsInsertWire(t *testing.T) {
	t.Parallel()
	const wantRequest = `{"writeControl":{"requiredRevisionId":"opaque-revision","writeMode":"SUGGEST"},"requests":[{"insertText":{"location":{"index":7},"text":" proposed 😀 "}}]}`
	for _, tc := range []struct {
		name, response, wantError string
		status                    int
	}{
		{name: "created", status: 200, response: `{"documentId":"doc1","commentUpdateState":"ALL_SAVED","suggestionResponses":[{"createdSuggestionIds":["suggest.123"]}]}`},
		{name: "unconfirmed", status: 200, response: `{"documentId":"doc1","replies":[{}]}`, wantError: "did not confirm creation"},
		{name: "empty creation", status: 200, response: `{"documentId":"doc1","commentUpdateState":"ALL_SAVED","suggestionResponses":[{}]}`, wantError: "did not confirm creation"},
		{name: "wrong document", status: 200, response: `{"documentId":"other","commentUpdateState":"ALL_SAVED","suggestionResponses":[{"createdSuggestionIds":["suggest.123"]}]}`, wantError: "did not confirm creation"},
		{name: "thread failed", status: 200, response: `{"documentId":"doc1","commentUpdateState":"ALL_FAILED_UNKNOWN_REASON","suggestionResponses":[{"createdSuggestionIds":["suggest.123"]}]}`, wantError: "did not confirm creation"},
		{name: "missing thread state", status: 200, response: `{"documentId":"doc1","suggestionResponses":[{"createdSuggestionIds":["suggest.123"]}]}`, wantError: "did not confirm creation"},
		{name: "server error", status: 503, response: `{"error":{"code":503,"message":"unavailable"}}`, wantError: "inspect the document before retrying"},
		{name: "unsupported", status: 400, response: `{"error":{"code":400,"message":"SUGGEST is not supported"}}`, wantError: "SUGGEST is not supported"},
		{name: "stale revision", status: 400, response: `{"error":{"code":400,"message":"required revision does not match"}}`, wantError: "required revision does not match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/v1/documents/doc1:batchUpdate" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				var got, want any
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if err := json.Unmarshal([]byte(wantRequest), &want); err != nil {
					t.Errorf("decode fixture: %v", err)
				}
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(want)
				if !bytes.Equal(gotJSON, wantJSON) {
					t.Errorf("request = %s, want %s", gotJSON, wantJSON)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.response)
			}))
			t.Cleanup(server.Close)
			client := server.Client()
			client.Transport = googleapi.NewRetryTransport(client.Transport)
			svc, err := docs.NewService(t.Context(), option.WithHTTPClient(client), option.WithEndpoint(server.URL+"/"))
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			ctx := withDocsTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)
			ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
			err = runKong(t, &DocsSuggestionsInsertCmd{}, []string{"doc1", " proposed 😀 ", "--index=7", "--revision=opaque-revision"}, ctx, &RootFlags{Account: "a@b.com"})
			if calls.Load() != 1 {
				t.Fatalf("HTTP calls = %d, want 1", calls.Load())
			}
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) || out.Len() != 0 {
					t.Fatalf("error = %v, output = %q; want %q and no output", err, out.String(), tc.wantError)
				}
				return
			}
			var result struct {
				DocumentID    string   `json:"documentId"`
				SuggestionIDs []string `json:"suggestionIds"`
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.DocumentID != "doc1" || len(result.SuggestionIDs) != 1 || result.SuggestionIDs[0] != "suggest.123" {
				t.Fatalf("decode error = %v, output = %q", err, out.String())
			}
		})
	}
}

func TestDocsSuggestionsInsertDryRunAndValidation(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	ctx := outfmt.WithMode(newCmdRuntimeOutputContext(t, &out, io.Discard), outfmt.Mode{JSON: true})
	flags := &RootFlags{DryRun: true}
	for _, args := range [][]string{
		{"doc1", "text", "--index=1"},
		{"doc1", "text", "--revision=r1"},
		{"doc1", "text", "--index=0", "--revision=r1"},
		{"doc1", "text", "--index=1", "--revision="},
		{"doc1", "", "--index=1", "--revision=r1"},
		{"doc1", "text", "--index=1", "--revision=r1", "--batch=b1"},
	} {
		err := runKong(t, &DocsSuggestionsInsertCmd{}, args, ctx, flags)
		if err == nil || ExitCode(err) == 0 || out.Len() != 0 {
			t.Fatalf("args %v: error = %v, output = %q; want validation failure", args, err, out.String())
		}
	}
	err := runKong(t, &DocsSuggestionsInsertCmd{}, []string{"doc1", "text", "--index=1", "--revision=r1"}, ctx, flags)
	var plan struct {
		DryRun  bool `json:"dry_run"`
		Request struct {
			Body struct {
				WriteControl struct {
					WriteMode          string `json:"writeMode"`
					RequiredRevisionID string `json:"requiredRevisionId"`
				} `json:"writeControl"`
			} `json:"body"`
		} `json:"request"`
	}
	if ExitCode(err) != 0 {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil || !plan.DryRun || plan.Request.Body.WriteControl.WriteMode != "SUGGEST" || plan.Request.Body.WriteControl.RequiredRevisionID != "r1" {
		t.Fatalf("decode error = %v, dry-run output = %q", err, out.String())
	}
}
