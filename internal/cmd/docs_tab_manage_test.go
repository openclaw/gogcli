package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsAddRenameDeleteTab(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var batchRequests [][]*docs.Request
	var includeTabsCalls int

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			if strings.Contains(r.URL.RawQuery, "includeTabsContent=true") {
				includeTabsCalls++
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies": []any{
					map[string]any{
						"addDocumentTab": map[string]any{
							"tabProperties": map[string]any{"tabId": "t.third", "title": "Third", "index": 2},
						},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := newDocsCmdContext(t)

	idx := int64(2)
	if err := runKong(t, &DocsAddTabCmd{}, []string{"doc1", "--title", "Third", "--index", "2"}, ctx, flags); err != nil {
		t.Fatalf("add-tab: %v", err)
	}
	addReq := batchRequests[0][0].AddDocumentTab
	if addReq == nil || addReq.TabProperties == nil {
		t.Fatalf("unexpected add request: %#v", batchRequests[0][0])
	}
	if addReq.TabProperties.Title != "Third" || addReq.TabProperties.Index != idx {
		t.Fatalf("unexpected add props: %#v", addReq.TabProperties)
	}

	if err := runKong(t, &DocsRenameTabCmd{}, []string{"doc1", "--tab", "Second", "--title", "TWO"}, ctx, flags); err != nil {
		t.Fatalf("rename-tab: %v", err)
	}
	renameReq := batchRequests[1][0].UpdateDocumentTabProperties
	if renameReq == nil || renameReq.TabProperties == nil {
		t.Fatalf("unexpected rename request: %#v", batchRequests[1][0])
	}
	if renameReq.Fields != "title" || renameReq.TabProperties.TabId != "t.second" || renameReq.TabProperties.Title != "TWO" {
		t.Fatalf("unexpected rename props: %#v", renameReq)
	}

	if err := runKong(t, &DocsDeleteTabCmd{}, []string{"doc1", "--tab", "Second"}, ctx, flags); err != nil {
		t.Fatalf("delete-tab: %v", err)
	}
	deleteReq := batchRequests[2][0].DeleteTab
	if deleteReq == nil || deleteReq.TabId != "t.second" {
		t.Fatalf("unexpected delete request: %#v", batchRequests[2][0])
	}

	if includeTabsCalls != 2 {
		t.Fatalf("expected 2 tab-aware GET calls, got %d", includeTabsCalls)
	}
}

func TestDocsRenameDeleteTab_NotFound(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := newDocsCmdContext(t)

	err := runKong(t, &DocsRenameTabCmd{}, []string{"doc1", "--tab", "Missing", "--title", "X"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `tab not found: "Missing"`) {
		t.Fatalf("unexpected rename error: %v", err)
	}

	err = runKong(t, &DocsDeleteTabCmd{}, []string{"doc1", "--tab", "Missing"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `tab not found: "Missing"`) {
		t.Fatalf("unexpected delete error: %v", err)
	}
}
