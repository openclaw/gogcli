package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
)

// fullDocResponse returns a richer Document payload than DocsInfoCmd would
// ever request (DocsInfoCmd restricts via Fields). This proves `raw` drops
// that restriction and exposes the full API tree.
func fullDocResponse(id string) map[string]any {
	return map[string]any{
		"documentId": id,
		"title":      "Full Doc",
		"revisionId": "rev1",
		"body": map[string]any{
			"content": []any{
				map[string]any{
					"startIndex": 1,
					"endIndex":   10,
					"paragraph": map[string]any{
						"elements": []any{
							map[string]any{
								"textRun": map[string]any{
									"content": "hello world\n",
								},
							},
						},
					},
				},
			},
		},
		"namedStyles": map[string]any{
			"styles": []any{map[string]any{"namedStyleType": "NORMAL_TEXT"}},
		},
	}
}

func tabbedDocResponse(id string) map[string]any {
	return map[string]any{
		"documentId":          id,
		"title":               "Tabbed Doc",
		"revisionId":          "rev-tabs",
		"suggestionsViewMode": "SUGGESTIONS_INLINE",
		"tabs": []any{
			map[string]any{
				"tabProperties": map[string]any{
					"tabId": "t.first",
					"title": "First",
					"index": 0,
				},
				"documentTab": map[string]any{
					"body": rawBodyWithText("first tab\n"),
				},
				"childTabs": []any{
					map[string]any{
						"tabProperties": map[string]any{
							"tabId": "t.nested",
							"title": "Nested Notes",
							"index": 0,
						},
						"documentTab": map[string]any{
							"body": rawBodyWithText("nested tab\n"),
							"inlineObjects": map[string]any{
								"kix.image": map[string]any{
									"objectId": "kix.image",
								},
							},
							"namedStyles": map[string]any{
								"styles": []any{map[string]any{"namedStyleType": "NORMAL_TEXT"}},
							},
						},
					},
				},
			},
		},
	}
}

func rawBodyWithText(text string) map[string]any {
	return map[string]any{
		"content": []any{
			map[string]any{
				"startIndex": 1,
				"endIndex":   1 + len(text),
				"paragraph": map[string]any{
					"elements": []any{
						map[string]any{
							"textRun": map[string]any{"content": text},
						},
					},
				},
			},
		},
	}
}

// newDocsRawTestServer builds a test Docs server; if status != 0 it returns
// that status instead of a successful response.
func newDocsRawTestServer(t *testing.T, status int, body map[string]any) *httptest.Server {
	t.Helper()
	return newDocsRawTestServerWithRequest(t, status, body, nil)
}

func newDocsRawTestServerWithRequest(
	t *testing.T,
	status int,
	body map[string]any,
	checkRequest func(*http.Request),
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/documents/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if checkRequest != nil {
			checkRequest(r)
		}
		if status != 0 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": status, "message": "mock error"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func installMockDocsService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := newDocsService
	t.Cleanup(func() { newDocsService = orig })

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
}

func rawTestContext(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func TestDocsRaw_HappyPath(t *testing.T) {
	srv := newDocsRawTestServerWithRequest(t, 0, fullDocResponse("doc1"), func(r *http.Request) {
		if _, ok := r.URL.Query()["includeTabsContent"]; ok {
			t.Fatalf("default raw request unexpectedly set includeTabsContent: %s", r.URL.RawQuery)
		}
	})
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		cmd := &DocsRawCmd{}
		if err := runKong(t, cmd, []string{"doc1"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, out)
	}
	// Bare struct: top-level keys must be Document fields, not a wrapper.
	if got["documentId"] != "doc1" {
		t.Fatalf("expected documentId=doc1, got: %v", got["documentId"])
	}
	// The whole point of raw: body.content must be present (info -j drops it).
	body, ok := got["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected body object in output, got: %v", got["body"])
	}
	if _, ok := body["content"]; !ok {
		t.Fatalf("expected body.content in raw output")
	}
}

func TestProjectRawDocumentTabCopiesLegacyFields(t *testing.T) {
	content := &docs.DocumentTab{
		Body:                          &docs.Body{},
		DocumentStyle:                 &docs.DocumentStyle{},
		Footers:                       map[string]docs.Footer{"footer": {}},
		Footnotes:                     map[string]docs.Footnote{"footnote": {}},
		Headers:                       map[string]docs.Header{"header": {}},
		InlineObjects:                 map[string]docs.InlineObject{"inline": {}},
		Lists:                         map[string]docs.List{"list": {}},
		NamedRanges:                   map[string]docs.NamedRanges{"range": {}},
		NamedStyles:                   &docs.NamedStyles{},
		PositionedObjects:             map[string]docs.PositionedObject{"positioned": {}},
		SuggestedDocumentStyleChanges: map[string]docs.SuggestedDocumentStyle{"doc-style": {}},
		SuggestedNamedStylesChanges:   map[string]docs.SuggestedNamedStyles{"named-style": {}},
	}
	doc := &docs.Document{
		DocumentId:          "doc1",
		RevisionId:          "rev1",
		SuggestionsViewMode: "SUGGESTIONS_INLINE",
		Title:               "Full Doc",
		Tabs:                []*docs.Tab{{}},
	}

	got, err := projectRawDocumentTab(doc, &docs.Tab{DocumentTab: content})
	if err != nil {
		t.Fatalf("projectRawDocumentTab: %v", err)
	}
	if got.DocumentId != doc.DocumentId || got.RevisionId != doc.RevisionId ||
		got.SuggestionsViewMode != doc.SuggestionsViewMode || got.Title != doc.Title {
		t.Fatalf("top-level metadata was not preserved: %#v", got)
	}
	if got.Body != content.Body || got.DocumentStyle != content.DocumentStyle || got.NamedStyles != content.NamedStyles {
		t.Fatal("selected tab pointer fields were not projected")
	}
	if len(got.Footers) != 1 || len(got.Footnotes) != 1 || len(got.Headers) != 1 ||
		len(got.InlineObjects) != 1 || len(got.Lists) != 1 || len(got.NamedRanges) != 1 ||
		len(got.PositionedObjects) != 1 || len(got.SuggestedDocumentStyleChanges) != 1 ||
		len(got.SuggestedNamedStylesChanges) != 1 {
		t.Fatalf("selected tab map fields were not projected: %#v", got)
	}
	if got.Tabs != nil {
		t.Fatalf("projected tab unexpectedly retained tabs: %#v", got.Tabs)
	}
}

func TestDocsRaw_TabProjectsSelectedNestedTab(t *testing.T) {
	for _, query := range []string{"t.nested", "nested notes"} {
		t.Run(query, func(t *testing.T) {
			srv := newDocsRawTestServerWithRequest(t, 0, tabbedDocResponse("doc1"), func(r *http.Request) {
				if got := r.URL.Query().Get("includeTabsContent"); got != "true" {
					t.Fatalf("includeTabsContent = %q, want true", got)
				}
			})
			defer srv.Close()
			installMockDocsService(t, srv)

			ctx := rawTestContext(t)
			flags := &RootFlags{Account: "a@b.com"}
			out := captureStdout(t, func() {
				cmd := &DocsRawCmd{}
				if err := runKong(t, cmd, []string{"doc1", "--tab", query}, ctx, flags); err != nil {
					t.Fatalf("run: %v", err)
				}
			})

			var got map[string]any
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("output is not valid JSON: %v\nraw: %s", err, out)
			}
			if got["documentId"] != "doc1" || got["title"] != "Tabbed Doc" || got["revisionId"] != "rev-tabs" {
				t.Fatalf("top-level document metadata was not preserved: %#v", got)
			}
			if _, ok := got["tabs"]; ok {
				t.Fatalf("selected-tab projection unexpectedly included tabs: %#v", got["tabs"])
			}
			if _, ok := got["inlineObjects"].(map[string]any)["kix.image"]; !ok {
				t.Fatalf("selected tab inline objects missing: %#v", got["inlineObjects"])
			}
			if text := rawDocumentText(t, got); text != "nested tab\n" {
				t.Fatalf("selected tab text = %q, want %q", text, "nested tab\n")
			}
		})
	}
}

func TestDocsRaw_TabNotFound(t *testing.T) {
	srv := newDocsRawTestServer(t, 0, tabbedDocResponse("doc1"))
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		cmd := &DocsRawCmd{}
		err := runKong(t, cmd, []string{"doc1", "--tab", "Missing"}, ctx, flags)
		if err == nil || !strings.Contains(err.Error(), "tab not found") {
			t.Fatalf("expected tab-not-found error, got %v", err)
		}
	})
}

func TestDocsRaw_AllTabsReturnsCanonicalDocument(t *testing.T) {
	srv := newDocsRawTestServerWithRequest(t, 0, tabbedDocResponse("doc1"), func(r *http.Request) {
		if got := r.URL.Query().Get("includeTabsContent"); got != "true" {
			t.Fatalf("includeTabsContent = %q, want true", got)
		}
	})
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		cmd := &DocsRawCmd{}
		if err := runKong(t, cmd, []string{"doc1", "--all-tabs"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, out)
	}
	tabs, ok := got["tabs"].([]any)
	if !ok || len(tabs) != 1 {
		t.Fatalf("canonical tabs output = %#v, want one top-level tab", got["tabs"])
	}
	first := tabs[0].(map[string]any)
	children, ok := first["childTabs"].([]any)
	if !ok || len(children) != 1 {
		t.Fatalf("nested tabs output = %#v, want one child tab", first["childTabs"])
	}
}

func TestDocsRaw_TabAndAllTabsConflict(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	err := (&DocsRawCmd{DocID: "doc1", Tab: "First", AllTabs: true}).Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("expected conflicting flags error, got %v", err)
	}
}

func rawDocumentText(t *testing.T, doc map[string]any) string {
	t.Helper()
	body, ok := doc["body"].(map[string]any)
	if !ok {
		t.Fatalf("body = %#v, want object", doc["body"])
	}
	content, ok := body["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("body.content = %#v, want elements", body["content"])
	}
	paragraph := content[0].(map[string]any)["paragraph"].(map[string]any)
	elements := paragraph["elements"].([]any)
	return elements[0].(map[string]any)["textRun"].(map[string]any)["content"].(string)
}

func TestDocsRaw_APIError(t *testing.T) {
	srv := newDocsRawTestServer(t, http.StatusInternalServerError, nil)
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}

	_ = captureStdout(t, func() {
		cmd := &DocsRawCmd{}
		err := runKong(t, cmd, []string{"doc1"}, ctx, flags)
		if err == nil {
			t.Fatalf("expected error on 500, got nil")
		}
	})
}

func TestDocsRaw_NotFound(t *testing.T) {
	srv := newDocsRawTestServer(t, http.StatusNotFound, nil)
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}

	_ = captureStdout(t, func() {
		cmd := &DocsRawCmd{}
		err := runKong(t, cmd, []string{"doc1"}, ctx, flags)
		if err == nil {
			t.Fatalf("expected error on 404")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected 'not found' in error, got: %v", err)
		}
	})
}

func TestDocsRaw_EmptyDocID(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	err := (&DocsRawCmd{}).Run(ctx, flags)
	if err == nil {
		t.Fatalf("expected error on empty docId")
	}
}
