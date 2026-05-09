package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/ui"
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

func TestDocsWriteUpdate_WithTab(t *testing.T) {
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

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--tab", "Second"}, ctx, flags); err != nil {
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

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "world", "--append", "--tab", "t.second"}, ctx, flags); err != nil {
		t.Fatalf("write append: %v", err)
	}
	if got := batchRequests[1][0].InsertText.Location; got.TabId != "t.second" || got.Index != 19 {
		t.Fatalf("unexpected append insert location: %#v", got)
	}

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "**markdown**", "--append", "--markdown", "--tab", "Second"}, ctx, flags); err != nil {
		t.Fatalf("write markdown append: %v", err)
	}
	if got := batchRequests[2][0].InsertText.Location; got.TabId != "t.second" || got.Index != 19 {
		t.Fatalf("unexpected markdown append insert location: %#v", got)
	}

	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "!", "--tab", "t.second"}, ctx, flags); err != nil {
		t.Fatalf("update append: %v", err)
	}
	if got := batchRequests[3][0].InsertText.Location; got.TabId != "t.second" || got.Index != 19 {
		t.Fatalf("unexpected update insert location: %#v", got)
	}

	if err := runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "?", "--index", "5", "--tab", "t.second"}, ctx, flags); err != nil {
		t.Fatalf("update explicit index: %v", err)
	}
	if got := batchRequests[4][0].InsertText.Location; got.TabId != "t.second" || got.Index != 5 {
		t.Fatalf("unexpected indexed update location: %#v", got)
	}

	if includeTabsCalls != 5 {
		t.Fatalf("expected 5 tab-aware GET calls, got %d", includeTabsCalls)
	}
}

func TestDocsWriteUpdate_WithTab_TabNotFound(t *testing.T) {
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

	err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--tab", "t.missing"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `tab not found: "t.missing"`) {
		t.Fatalf("unexpected write error: %v", err)
	}

	err = runKong(t, &DocsUpdateCmd{}, []string{"doc1", "--text", "hello", "--tab", "t.missing"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `tab not found: "t.missing"`) {
		t.Fatalf("unexpected update error: %v", err)
	}
}

func TestDocsEditingCommands_WithTab(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
			return
		}
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

	if err := runKong(t, &DocsInsertCmd{}, []string{"doc1", "hello", "--index", "5", "--tab", "Second"}, ctx, flags); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got := batchRequests[0][0].InsertText.Location; got.TabId != "t.second" || got.Index != 5 {
		t.Fatalf("unexpected insert location: %#v", got)
	}

	if err := runKong(t, &DocsDeleteCmd{}, []string{"doc1", "--start", "2", "--end", "7", "--tab", "Second"}, ctx, flags); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := batchRequests[1][0].DeleteContentRange.Range; got.TabId != "t.second" || got.StartIndex != 2 || got.EndIndex != 7 {
		t.Fatalf("unexpected delete range: %#v", got)
	}

	if err := runKong(t, &DocsFindReplaceCmd{}, []string{"doc1", "old", "new", "--tab", "Second"}, ctx, flags); err != nil {
		t.Fatalf("find-replace: %v", err)
	}
	req := batchRequests[2][0].ReplaceAllText
	if req == nil || req.TabsCriteria == nil || len(req.TabsCriteria.TabIds) != 1 || req.TabsCriteria.TabIds[0] != "t.second" {
		t.Fatalf("unexpected tabs criteria: %#v", req)
	}

	if err := runKong(t, &DocsFindReplaceCmd{}, []string{"doc1", "old", "**new**", "--format", "markdown", "--tab", "Second"}, ctx, flags); err != nil {
		t.Fatalf("find-replace markdown tab: %v", err)
	}
}

func TestDocsWriteCmd_DeprecatedTabIDFlag(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	var stderrBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &stderrBuf, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &DocsWriteCmd{}, []string{"doc1", "--text", "hello", "--tab-id", "t.second"}, ctx, flags); err != nil {
		t.Fatalf("write with --tab-id: %v", err)
	}
	if !strings.Contains(stderrBuf.String(), "--tab-id is deprecated") {
		t.Errorf("expected deprecation warning in stderr, got: %q", stderrBuf.String())
	}
}
