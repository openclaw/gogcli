package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openclaw/gogcli/internal/outfmt"
)

func TestDocsSuggestionsBatchInput(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		`null`, `[]`, `[null]`, `[{}]`, `[[]]`,
		`{"requests":[{"insertText":{}}],"writeControl":{"writeMode":"EDIT"}}`,
		`[{"insertText":null}]`,
		`[{"insertText":{},"insertText":{}}]`,
		`[{"insertText":{},"deleteContentRange":{}}]`,
		`[{"insertText":{"location":{"index":1,"Index":2}}}]`,
		`[{"insertText":{"unsupported":true}}]`,
		`[{"InsertText":{}}]`,
		`[{"insertText":{}}] [{"insertText":{}}]`,
		`[{"insertText":{}},{"addDocumentTab":{}}]`,
		`[{"createNamedRange":{}}]`, `[{"deleteNamedRange":{}}]`,
		`[{"deleteHeader":{}}]`, `[{"deleteFooter":{}}]`, `[{"deleteTab":{}}]`,
		`[{"updateTableColumnProperties":{}}]`, `[{"updateDocumentTabProperties":{}}]`,
		`[{"acceptSuggestion":{}}]`, `[{"rejectSuggestion":{}}]`, `[{"deleteSuggestion":{}}]`,
		`[{"updateDocumentStyle":{"fields":"*"}}]`,
		`[{"updateSectionStyle":{"fields":"*"}}]`,
		`[{"updateSectionStyle":{"fields":"columnProperties,useFirstPageHeaderFooter"}}]`,
		`[{"updateDocumentStyle":{"fields":"marginTop,documentFormat"}}]`,
		`[{"updateDocumentStyle":{"fields":"useFirstPageHeaderFooter"}}]`,
		`[` + strings.Repeat(`{"insertText":{}},`, docsBatchUpdateRequestCap) + `{"insertText":{}}]`,
	} {
		if got, err := parseDocsSuggestionsBatch([]byte(input)); err == nil || got != nil {
			t.Errorf("accepted %d bytes of unsafe input starting with %.85s", len(input), input)
		}
	}
}

func TestDocsSuggestionsBatchPreservesJSONAndNeverFollowsRedirect(t *testing.T) {
	t.Parallel()
	const input = `[{"updateTextStyle":{"range":{"startIndex":0,"endIndex":3,"tabId":"t.review","segmentId":"f1"},"fields":"bold","textStyle":{"bold":false}}},{"pinTableHeaderRows":{"tableStartLocation":{"index":10,"tabId":"t.review"},"pinnedHeaderRowsCount":0}}]`
	for _, redirect := range []bool{false, true} {
		name := "saved"
		if redirect {
			name = "redirect"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			filename := filepath.Join(t.TempDir(), "requests.json")
			if err := os.WriteFile(filename, []byte(input), 0o600); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/v1/documents/doc1:batchUpdate" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				var req struct {
					Requests     json.RawMessage `json:"requests"`
					WriteControl json.RawMessage `json:"writeControl"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil || string(req.Requests) != input || string(req.WriteControl) != `{"requiredRevisionId":"r1","writeMode":"SUGGEST"}` {
					t.Errorf("raw suggestion request mismatch: %+v / %v", req, err)
				}
				if redirect {
					w.Header().Set("Location", "/not-the-target")
					w.WriteHeader(http.StatusTemporaryRedirect)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"documentId":"doc1","commentUpdateState":"ALL_SAVED","suggestionResponses":[{"createdSuggestionIds":["suggest.z"]},{"createdSuggestionIds":["suggest.a"]}]}`)
			}))
			t.Cleanup(server.Close)
			var out bytes.Buffer
			ctx := withRawTestHTTP(t, newCmdRuntimeOutputContext(t, &out, io.Discard), server.URL)
			ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
			err := runKong(t, &DocsSuggestionsBatchCmd{}, []string{"doc1", "--file", filename, "--revision=r1"}, ctx, &RootFlags{Account: "a@b.com"})
			if calls.Load() != 1 {
				t.Fatalf("HTTP calls = %d; want one POST", calls.Load())
			}
			if redirect {
				if err == nil || out.Len() > 0 {
					t.Fatalf("redirect was not rejected: %v / %q", err, out.String())
				}
			} else if err != nil || !strings.Contains(out.String(), "suggest.a") || !strings.Contains(out.String(), "suggest.z") {
				t.Fatalf("submission: %v / %q", err, out.String())
			}
		})
	}
}

func TestDocsRawSuggestionViews(t *testing.T) {
	t.Parallel()
	for _, view := range []struct{ flag, api string }{
		{"inline", docsSuggestionsInline}, {"without", "PREVIEW_WITHOUT_SUGGESTIONS"}, {"accepted", "PREVIEW_SUGGESTIONS_ACCEPTED"},
	} {
		t.Run(view.flag, func(t *testing.T) {
			t.Parallel()
			args := []string{"doc1", "--all-tabs", "--suggestions-view=" + view.flag}
			if view.flag == "inline" {
				args = append(args, "--suggestion-threads")
			}
			server := newDocsRawTestServerWithRequest(t, 0, fullDocResponse("doc1"), func(r *http.Request) {
				q := r.URL.Query()
				wantComments := ""
				if view.flag == "inline" {
					wantComments = "COMMENTS_VIEW_MODE_INCLUDED"
				}
				if q.Get("suggestionsViewMode") != view.api || q.Get("commentsViewMode") != wantComments || q.Get("includeTabsContent") != "true" {
					t.Errorf("unexpected query: %v", q)
				}
			})
			t.Cleanup(server.Close)
			ctx, _ := docsRawTestContext(t, newMockDocsService(t, server))
			if err := runKong(t, &DocsRawCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
