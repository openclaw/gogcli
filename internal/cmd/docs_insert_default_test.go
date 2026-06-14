package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

// docBodyWithEndIndex returns a Get-response payload whose body endIndex matches
// the provided value, so tests can assert that the insert path resolved
// end-of-doc correctly when --index is omitted.
func docBodyWithEndIndex(end int64) map[string]any {
	return map[string]any{
		"documentId": "doc1",
		"body": map[string]any{
			"content": []any{
				map[string]any{"startIndex": 0, "endIndex": end},
			},
		},
	}
}

func TestDocsInsertCmd_DefaultsToEndOfDoc(t *testing.T) {
	t.Parallel()

	var batchRequests [][]*docs.Request
	var getCalls int

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			getCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(docBodyWithEndIndex(42))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)

	if err := runKong(t, &DocsInsertCmd{}, []string{"doc1", "hello"}, ctx, flags); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if getCalls != 1 {
		t.Fatalf("expected 1 GET to resolve end-index, got %d", getCalls)
	}
	if len(batchRequests) != 1 || len(batchRequests[0]) != 1 || batchRequests[0][0].InsertText == nil {
		t.Fatalf("unexpected requests: %#v", batchRequests)
	}
	loc := batchRequests[0][0].InsertText.Location
	if loc == nil {
		t.Fatalf("expected Location, got nil")
	}
	// endIndex = 42 -> docsedit.AppendIndex(42) = 41.
	if loc.Index != 41 {
		t.Fatalf("expected insert at end-1 (41), got %d", loc.Index)
	}
}

func TestDocsInsertCmd_ExplicitIndexSkipsGet(t *testing.T) {
	t.Parallel()

	var batchRequests [][]*docs.Request
	var getCalls int

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			getCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(docBodyWithEndIndex(42))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)

	if err := runKong(t, &DocsInsertCmd{}, []string{"doc1", "hello", "--index", "7"}, ctx, flags); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if getCalls != 0 {
		t.Fatalf("explicit --index should not GET the doc, but GET was called %d times", getCalls)
	}
	if got := batchRequests[0][0].InsertText.Location; got.Index != 7 {
		t.Fatalf("expected explicit index 7, got %d", got.Index)
	}
}

func TestDocsInsertCmd_ExplicitIndexBelowOneRejected(t *testing.T) {
	t.Parallel()

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), docSvc)

	err := runKong(t, &DocsInsertCmd{}, []string{"doc1", "hello", "--index", "0"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "--index must be >= 1") {
		t.Fatalf("expected --index >= 1 validation error, got %v", err)
	}
}
