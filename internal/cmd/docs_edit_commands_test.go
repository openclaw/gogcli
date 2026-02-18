package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

func newDocsServiceForTest(t *testing.T, h http.HandlerFunc) (*docs.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(h)
	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		srv.Close()
		t.Fatalf("NewDocsService: %v", err)
	}
	return docSvc, srv.Close
}

func newDocsCmdContext(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func TestDocsWriteReplace_EmptyBody_NoPanic(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsWriteCmd{}
	if err := runKong(t, cmd, []string{"doc1", "hello", "--replace"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs write --replace: %v", err)
	}

	if len(got.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(got.Requests))
	}
	if got.Requests[0].DeleteContentRange != nil {
		t.Fatal("unexpected delete request for empty doc body")
	}
	if got.Requests[0].InsertText == nil || got.Requests[0].InsertText.Text != "hello" {
		t.Fatalf("unexpected insert request: %#v", got.Requests[0])
	}
}

func TestDocsCatAllTabs_PropagatesStdoutError(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tabsDocResponse("doc1"))
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_ = r.Close()
	_ = w.Close()
	origStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsCatCmd{}
	err = runKong(t, cmd, []string{"doc1", "--all-tabs"}, newDocsCmdContext(t), flags)
	if err == nil {
		t.Fatal("expected stdout write error")
	}
}

func TestDocsInsert_SendsExpectedRequest(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsInsertCmd{}
	if err := runKong(t, cmd, []string{"doc1", "hello", "--index", "5"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs insert: %v", err)
	}

	if len(got.Requests) != 1 || got.Requests[0].InsertText == nil {
		t.Fatalf("unexpected request payload: %#v", got.Requests)
	}
	if got.Requests[0].InsertText.Text != "hello" || got.Requests[0].InsertText.Location == nil || got.Requests[0].InsertText.Location.Index != 5 {
		t.Fatalf("unexpected insert payload: %#v", got.Requests[0].InsertText)
	}
}

func TestDocsDelete_SendsExpectedRequest(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsDeleteCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--start", "2", "--end", "7"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs delete: %v", err)
	}

	if len(got.Requests) != 1 || got.Requests[0].DeleteContentRange == nil || got.Requests[0].DeleteContentRange.Range == nil {
		t.Fatalf("unexpected request payload: %#v", got.Requests)
	}
	rng := got.Requests[0].DeleteContentRange.Range
	if rng.StartIndex != 2 || rng.EndIndex != 7 {
		t.Fatalf("unexpected delete range: %#v", rng)
	}
}

func TestDocsFindReplace_SendsExpectedRequest(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies":    []any{map[string]any{"replaceAllText": map[string]any{"occurrencesChanged": 3}}},
			})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	if err := runKong(t, cmd, []string{"doc1", "foo", "bar", "--match-case"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs find-replace: %v", err)
	}

	if len(got.Requests) != 1 || got.Requests[0].ReplaceAllText == nil || got.Requests[0].ReplaceAllText.ContainsText == nil {
		t.Fatalf("unexpected request payload: %#v", got.Requests)
	}
	r := got.Requests[0].ReplaceAllText
	if r.ContainsText.Text != "foo" || !r.ContainsText.MatchCase || r.ReplaceText != "bar" {
		t.Fatalf("unexpected replace payload: %#v", r)
	}
}

// --- Tab-ID tests ---

// tabsDocWithEndIndex returns a multi-tab doc response where each tab has
// content with proper endIndex values (needed for --replace calculations).
func tabsDocWithEndIndex() map[string]any {
	return map[string]any{
		"documentId": "doc1",
		"title":      "Multi-Tab Doc",
		"tabs": []any{
			map[string]any{
				"tabProperties": map[string]any{
					"tabId": "t.first",
					"title": "First",
					"index": 0,
				},
				"documentTab": map[string]any{
					"body": map[string]any{
						"content": []any{
							map[string]any{
								"endIndex": 10,
								"paragraph": map[string]any{
									"elements": []any{
										map[string]any{
											"textRun": map[string]any{"content": "first tab"},
										},
									},
								},
							},
						},
					},
				},
			},
			map[string]any{
				"tabProperties": map[string]any{
					"tabId": "t.second",
					"title": "Second",
					"index": 1,
				},
				"documentTab": map[string]any{
					"body": map[string]any{
						"content": []any{
							map[string]any{
								"endIndex": 20,
								"paragraph": map[string]any{
									"elements": []any{
										map[string]any{
											"textRun": map[string]any{"content": "second tab content"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestDocsInsert_WithTabID(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsInsertCmd{}
	if err := runKong(t, cmd, []string{"doc1", "hello", "--index", "5", "--tab-id", "t.abc"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs insert --tab-id: %v", err)
	}

	if len(got.Requests) != 1 || got.Requests[0].InsertText == nil {
		t.Fatalf("unexpected request payload: %#v", got.Requests)
	}
	loc := got.Requests[0].InsertText.Location
	if loc == nil || loc.Index != 5 || loc.TabId != "t.abc" {
		t.Fatalf("expected TabId=t.abc at Index=5, got: %#v", loc)
	}
}

func TestDocsDelete_WithTabID(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsDeleteCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--start", "2", "--end", "7", "--tab-id", "t.xyz"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs delete --tab-id: %v", err)
	}

	if len(got.Requests) != 1 || got.Requests[0].DeleteContentRange == nil {
		t.Fatalf("unexpected request payload: %#v", got.Requests)
	}
	rng := got.Requests[0].DeleteContentRange.Range
	if rng.StartIndex != 2 || rng.EndIndex != 7 || rng.TabId != "t.xyz" {
		t.Fatalf("expected TabId=t.xyz range 2-7, got: %#v", rng)
	}
}

func TestDocsFindReplace_WithTabID(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies":    []any{map[string]any{"replaceAllText": map[string]any{"occurrencesChanged": 1}}},
			})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	if err := runKong(t, cmd, []string{"doc1", "foo", "bar", "--tab-id", "t.abc"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs find-replace --tab-id: %v", err)
	}

	if len(got.Requests) != 1 || got.Requests[0].ReplaceAllText == nil {
		t.Fatalf("unexpected request payload: %#v", got.Requests)
	}
	r := got.Requests[0].ReplaceAllText
	if r.TabsCriteria == nil || len(r.TabsCriteria.TabIds) != 1 || r.TabsCriteria.TabIds[0] != "t.abc" {
		t.Fatalf("expected TabsCriteria with t.abc, got: %#v", r.TabsCriteria)
	}
}

func TestDocsFindReplace_WithoutTabID_NoTabsCriteria(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies":    []any{map[string]any{"replaceAllText": map[string]any{"occurrencesChanged": 0}}},
			})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsFindReplaceCmd{}
	if err := runKong(t, cmd, []string{"doc1", "foo", "bar"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs find-replace: %v", err)
	}

	if len(got.Requests) != 1 || got.Requests[0].ReplaceAllText == nil {
		t.Fatalf("unexpected request payload: %#v", got.Requests)
	}
	if got.Requests[0].ReplaceAllText.TabsCriteria != nil {
		t.Fatalf("expected no TabsCriteria without --tab-id, got: %#v", got.Requests[0].ReplaceAllText.TabsCriteria)
	}
}

func TestDocsWriteAppend_WithTabID(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsWriteCmd{}
	if err := runKong(t, cmd, []string{"doc1", "hello", "--tab-id", "t.abc"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs write --tab-id: %v", err)
	}

	if len(got.Requests) != 1 || got.Requests[0].InsertText == nil {
		t.Fatalf("unexpected request payload: %#v", got.Requests)
	}
	eol := got.Requests[0].InsertText.EndOfSegmentLocation
	if eol == nil || eol.TabId != "t.abc" {
		t.Fatalf("expected EndOfSegmentLocation.TabId=t.abc, got: %#v", eol)
	}
}

func TestDocsWriteReplace_WithTabID(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsWriteCmd{}
	if err := runKong(t, cmd, []string{"doc1", "new content", "--replace", "--tab-id", "t.second"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs write --replace --tab-id: %v", err)
	}

	// Should have delete + insert requests, both with TabId set.
	if len(got.Requests) != 2 {
		t.Fatalf("expected 2 requests (delete+insert), got %d", len(got.Requests))
	}

	del := got.Requests[0].DeleteContentRange
	if del == nil || del.Range == nil || del.Range.TabId != "t.second" {
		t.Fatalf("expected delete with TabId=t.second, got: %#v", del)
	}
	if del.Range.EndIndex != 19 {
		t.Fatalf("expected endIndex=19 (20-1), got: %d", del.Range.EndIndex)
	}

	ins := got.Requests[1].InsertText
	if ins == nil || ins.EndOfSegmentLocation == nil || ins.EndOfSegmentLocation.TabId != "t.second" {
		t.Fatalf("expected insert with TabId=t.second, got: %#v", ins)
	}
}

func TestDocsWriteReplace_WithTabID_TabNotFound(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsWriteCmd{}
	err := runKong(t, cmd, []string{"doc1", "content", "--replace", "--tab-id", "t.nonexistent"}, newDocsCmdContext(t), flags)
	if err == nil {
		t.Fatal("expected error for nonexistent tab")
	}
	if !strings.Contains(err.Error(), "tab not found") {
		t.Fatalf("expected 'tab not found' error, got: %v", err)
	}
}

func TestSetTabIdOnRequests(t *testing.T) {
	reqs := []*docs.Request{
		{InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: 1},
		}},
		{InsertText: &docs.InsertTextRequest{
			EndOfSegmentLocation: &docs.EndOfSegmentLocation{},
		}},
		{DeleteContentRange: &docs.DeleteContentRangeRequest{
			Range: &docs.Range{StartIndex: 1, EndIndex: 5},
		}},
		{UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
			Range: &docs.Range{StartIndex: 1, EndIndex: 3},
		}},
		{UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{StartIndex: 2, EndIndex: 4},
		}},
		nil,
	}

	setTabIdOnRequests(reqs, "t.test")

	if reqs[0].InsertText.Location.TabId != "t.test" {
		t.Fatalf("InsertText.Location.TabId not set")
	}
	if reqs[1].InsertText.EndOfSegmentLocation.TabId != "t.test" {
		t.Fatalf("InsertText.EndOfSegmentLocation.TabId not set")
	}
	if reqs[2].DeleteContentRange.Range.TabId != "t.test" {
		t.Fatalf("DeleteContentRange.Range.TabId not set")
	}
	if reqs[3].UpdateParagraphStyle.Range.TabId != "t.test" {
		t.Fatalf("UpdateParagraphStyle.Range.TabId not set")
	}
	if reqs[4].UpdateTextStyle.Range.TabId != "t.test" {
		t.Fatalf("UpdateTextStyle.Range.TabId not set")
	}
}

func TestSetTabIdOnRequests_EmptyTabID(t *testing.T) {
	reqs := []*docs.Request{
		{InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: 1},
		}},
	}

	setTabIdOnRequests(reqs, "")

	if reqs[0].InsertText.Location.TabId != "" {
		t.Fatalf("expected empty TabId when called with empty string")
	}
}

func TestDocsUpdateReplace_WithTabID(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsUpdateCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--content", "new text", "--tab-id", "t.second"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs update --tab-id: %v", err)
	}

	// Should have delete + insert requests, both with TabId.
	if len(got.Requests) != 2 {
		t.Fatalf("expected 2 requests (delete+insert), got %d", len(got.Requests))
	}

	del := got.Requests[0].DeleteContentRange
	if del == nil || del.Range == nil || del.Range.TabId != "t.second" {
		t.Fatalf("expected delete with TabId=t.second, got: %#v", del)
	}

	ins := got.Requests[1].InsertText
	if ins == nil || ins.Location == nil || ins.Location.TabId != "t.second" {
		t.Fatalf("expected insert with TabId=t.second, got: %#v", ins)
	}
}

func TestDocsUpdateAppend_WithTabID(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsUpdateCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--content", "appended", "--append", "--tab-id", "t.second"}, newDocsCmdContext(t), flags); err != nil {
		t.Fatalf("docs update --append --tab-id: %v", err)
	}

	// Append: only insert, no delete.
	if len(got.Requests) != 1 {
		t.Fatalf("expected 1 request (insert), got %d", len(got.Requests))
	}

	ins := got.Requests[0].InsertText
	if ins == nil || ins.Location == nil || ins.Location.TabId != "t.second" {
		t.Fatalf("expected insert with TabId=t.second, got: %#v", ins)
	}
	// Append index should be endIndex-1 = 19.
	if ins.Location.Index != 19 {
		t.Fatalf("expected append at index 19, got %d", ins.Location.Index)
	}
}

func TestDocsUpdate_WithTabID_TabNotFound(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	cmd := &DocsUpdateCmd{}
	err := runKong(t, cmd, []string{"doc1", "--content", "text", "--tab-id", "t.nope"}, newDocsCmdContext(t), flags)
	if err == nil {
		t.Fatal("expected error for nonexistent tab")
	}
	if !strings.Contains(err.Error(), "tab not found") {
		t.Fatalf("expected 'tab not found' error, got: %v", err)
	}
}
