package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func tabsDocWithEndIndex() map[string]any {
	return map[string]any{
		"documentId": "doc1",
		"title":      "Multi-Tab Doc",
		"tabs": []any{
			map[string]any{
				"tabProperties": map[string]any{"tabId": "t.first", "title": "First", "index": 0},
				"documentTab": map[string]any{
					"body": map[string]any{
						"content": []any{
							map[string]any{"endIndex": 10},
						},
					},
				},
			},
			map[string]any{
				"tabProperties": map[string]any{"tabId": "t.second", "title": "Second", "index": 1},
				"documentTab": map[string]any{
					"body": map[string]any{
						"content": []any{
							map[string]any{"endIndex": 20},
						},
					},
				},
			},
		},
	}
}

func TestDocsWriteUpdate_WithTabID(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request
	var includeTabsCalls int

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/documents/"), ":batchUpdate")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": id})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			if strings.Contains(r.URL.RawQuery, "includeTabsContent=true") {
				includeTabsCalls++
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsCmdContext(t)

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--tab-id", "t.second"}, ctx, flags); err != nil {
		t.Fatalf("write replace: %v", err)
	}
	if got := batchRequests[0]; len(got) != 2 || got[0].DeleteContentRange == nil || got[1].InsertText == nil {
		t.Fatalf("unexpected write requests: %#v", got)
	}
	if got := batchRequests[0][0].DeleteContentRange.Range; got.TabId != "t.second" || got.EndIndex != 19 {
		t.Fatalf("unexpected delete range: %#v", got)
	}
	if got := batchRequests[0][1].InsertText.Location; got.TabId != "t.second" || got.Index != 1 {
		t.Fatalf("unexpected write insert location: %#v", got)
	}

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "world", "--append", "--tab-id", "t.second"}, ctx, flags); err != nil {
		t.Fatalf("write append: %v", err)
	}
	if got := batchRequests[1][0].InsertText.Location; got.TabId != "t.second" || got.Index != 19 {
		t.Fatalf("unexpected append insert location: %#v", got)
	}

	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "!", "--tab-id", "t.second"}, ctx, flags); err != nil {
		t.Fatalf("update append: %v", err)
	}
	if got := batchRequests[2][0].InsertText.Location; got.TabId != "t.second" || got.Index != 19 {
		t.Fatalf("unexpected update insert location: %#v", got)
	}

	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "?", "--index", "5", "--tab-id", "t.second"}, ctx, flags); err != nil {
		t.Fatalf("update explicit index: %v", err)
	}
	if got := batchRequests[3][0].InsertText.Location; got.TabId != "t.second" || got.Index != 5 {
		t.Fatalf("unexpected indexed update location: %#v", got)
	}

	if includeTabsCalls != 3 {
		t.Fatalf("expected 3 tab-aware GET calls, got %d", includeTabsCalls)
	}
}

func TestDocsWriteUpdate_WithTabID_TabNotFound(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsCmdContext(t)

	err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--tab-id", "t.missing"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "tab not found: t.missing") {
		t.Fatalf("unexpected write error: %v", err)
	}

	err = runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "hello", "--tab-id", "t.missing"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "tab not found: t.missing") {
		t.Fatalf("unexpected update error: %v", err)
	}
}

func TestDocsEditingCommands_WithTabID(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate") {
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies":    []any{map[string]any{"replaceAllText": map[string]any{"occurrencesChanged": 1}}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsCmdContext(t)

	if err := runKong(t, &DocsInsertCmd{}, []string{"doc1", "hello", "--index", "5", "--tab-id", "t.abc"}, ctx, flags); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got := batchRequests[0][0].InsertText.Location; got.TabId != "t.abc" || got.Index != 5 {
		t.Fatalf("unexpected insert location: %#v", got)
	}

	if err := runKong(t, &DocsDeleteCmd{}, []string{"doc1", "--start", "2", "--end", "7", "--tab-id", "t.abc"}, ctx, flags); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := batchRequests[1][0].DeleteContentRange.Range; got.TabId != "t.abc" || got.StartIndex != 2 || got.EndIndex != 7 {
		t.Fatalf("unexpected delete range: %#v", got)
	}

	if err := runKong(t, &DocsFindReplaceCmd{}, []string{"doc1", "old", "new", "--tab-id", "t.abc"}, ctx, flags); err != nil {
		t.Fatalf("find-replace: %v", err)
	}
	req := batchRequests[2][0].ReplaceAllText
	if req == nil || req.TabsCriteria == nil || len(req.TabsCriteria.TabIds) != 1 || req.TabsCriteria.TabIds[0] != "t.abc" {
		t.Fatalf("unexpected tabs criteria: %#v", req)
	}
}
