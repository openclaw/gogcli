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

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDocsCat_Tab(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/documents/doc1") && r.Method == http.MethodGet {
			if r.URL.Query().Get("includeTabsContent") != "true" {
				t.Fatalf("expected includeTabsContent=true, got %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"tabs": []any{
					map[string]any{
						"tabProperties": map[string]any{"tabId": "tab1", "title": "Tab 1"},
						"documentTab": map[string]any{
							"body": map[string]any{
								"content": []any{
									map[string]any{
										"paragraph": map[string]any{
											"elements": []any{
												map[string]any{"textRun": map[string]any{"content": "Tab text"}},
											},
										},
									},
								},
							},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	out := captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		if err := runKong(t, cmd, []string{"doc1", "--tab", "tab1"}, ctx, flags); err != nil {
			t.Fatalf("cat: %v", err)
		}
	})
	if out != "Tab text" {
		t.Fatalf("unexpected cat output: %q", out)
	}
}

func TestDocsInfo_Tab(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/documents/doc1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"title":      "Doc",
				"revisionId": "r1",
				"tabs": []any{
					map[string]any{
						"tabProperties": map[string]any{
							"tabId":        "tab1",
							"title":        "First",
							"index":        0,
							"nestingLevel": 0,
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	var outBuf strings.Builder
	u, uiErr := ui.New(ui.Options{Stdout: &outBuf, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsInfoCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--tab", "tab1"}, ctx, flags); err != nil {
		t.Fatalf("info: %v", err)
	}
	out := outBuf.String()
	if !strings.Contains(out, "tab\ttab1") {
		t.Fatalf("expected tab output, got %q", out)
	}
	if !strings.Contains(out, "link\thttps://docs.google.com/document/d/doc1/edit?tab=tab1") {
		t.Fatalf("expected tab link, got %q", out)
	}
}

func TestDocsTabsList_JSON(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/documents/doc1") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"title":      "Doc",
				"tabs": []any{
					map[string]any{
						"tabProperties": map[string]any{
							"tabId":        "t1",
							"title":        "One",
							"index":        0,
							"nestingLevel": 0,
						},
						"childTabs": []any{
							map[string]any{
								"tabProperties": map[string]any{
									"tabId":        "t1a",
									"title":        "Child",
									"index":        0,
									"nestingLevel": 1,
									"parentTabId":  "t1",
								},
							},
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		cmd := &DocsTabsListCmd{}
		if err := runKong(t, cmd, []string{"doc1"}, ctx, flags); err != nil {
			t.Fatalf("tabs list: %v", err)
		}
	})

	var parsed struct {
		TabCount int `json:"tabCount"`
		Tabs     []struct {
			ID string `json:"id"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.TabCount != 2 || len(parsed.Tabs) != 2 {
		t.Fatalf("unexpected tab count: %#v", parsed)
	}
	if parsed.Tabs[0].ID != "t1" || parsed.Tabs[1].ID != "t1a" {
		t.Fatalf("unexpected tabs: %#v", parsed.Tabs)
	}
}

func TestDocsUpdate_RequestsJSON(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1/documents/doc1:batchUpdate") && r.Method == http.MethodPost {
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].InsertText == nil {
				t.Fatalf("expected insertText request, got %#v", req.Requests)
			}
			if req.WriteControl == nil || req.WriteControl.RequiredRevisionId != "r1" {
				t.Fatalf("missing required revision: %#v", req.WriteControl)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies":    []any{map[string]any{}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"doc1",
		"--requests-json", `[{"insertText":{"text":"Hello","location":{"index":1}}}]`,
		"--required-revision", "r1",
	}, ctx, flags); err != nil {
		t.Fatalf("update: %v", err)
	}
}

func TestDocsTabsAdd(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1/documents/doc1:batchUpdate") && r.Method == http.MethodPost {
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].AddDocumentTab == nil {
				t.Fatalf("expected addDocumentTab request, got %#v", req.Requests)
			}
			props := req.Requests[0].AddDocumentTab.TabProperties
			if props == nil || props.Title != "New Tab" || props.ParentTabId != "parent1" || props.Index != 0 {
				t.Fatalf("unexpected tab properties: %#v", props)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies": []any{
					map[string]any{"addDocumentTab": map[string]any{"tabProperties": map[string]any{"tabId": "tab123", "title": "New Tab"}}},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsTabsAddCmd{}
	if err := runKong(t, cmd, []string{"doc1", "New Tab", "--parent", "parent1", "--index", "0"}, ctx, flags); err != nil {
		t.Fatalf("tabs add: %v", err)
	}
}

func TestDocsTabsUpdate(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1/documents/doc1:batchUpdate") && r.Method == http.MethodPost {
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].UpdateDocumentTabProperties == nil {
				t.Fatalf("expected updateDocumentTabProperties request, got %#v", req.Requests)
			}
			update := req.Requests[0].UpdateDocumentTabProperties
			if update.Fields != "title,index" {
				t.Fatalf("unexpected fields: %q", update.Fields)
			}
			if update.TabProperties == nil || update.TabProperties.TabId != "tab1" || update.TabProperties.Title != "Renamed" || update.TabProperties.Index != 0 {
				t.Fatalf("unexpected tab properties: %#v", update.TabProperties)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies":    []any{map[string]any{}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsTabsUpdateCmd{}
	if err := runKong(t, cmd, []string{"doc1", "tab1", "--title", "Renamed", "--index", "0"}, ctx, flags); err != nil {
		t.Fatalf("tabs update: %v", err)
	}
}

func TestDocsTabsDelete(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1/documents/doc1:batchUpdate") && r.Method == http.MethodPost {
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].DeleteTab == nil || req.Requests[0].DeleteTab.TabId != "tab1" {
				t.Fatalf("unexpected delete request: %#v", req.Requests)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies":    []any{map[string]any{}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsTabsDeleteCmd{}
	if err := runKong(t, cmd, []string{"doc1", "tab1"}, ctx, flags); err != nil {
		t.Fatalf("tabs delete: %v", err)
	}
}

func TestDocsInsert_Index(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1/documents/doc1:batchUpdate") && r.Method == http.MethodPost {
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].InsertText == nil {
				t.Fatalf("expected insertText request, got %#v", req.Requests)
			}
			insert := req.Requests[0].InsertText
			if insert.Text != "Hello" {
				t.Fatalf("unexpected text: %q", insert.Text)
			}
			if insert.Location == nil || insert.Location.Index != 0 || insert.Location.TabId != "tab1" {
				t.Fatalf("unexpected location: %#v", insert.Location)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies":    []any{map[string]any{}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsInsertCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--text", "Hello", "--index", "0", "--tab", "tab1"}, ctx, flags); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestDocsReplace_Tab(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1/documents/doc1:batchUpdate") && r.Method == http.MethodPost {
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].ReplaceAllText == nil {
				t.Fatalf("expected replaceAllText request, got %#v", req.Requests)
			}
			replace := req.Requests[0].ReplaceAllText
			if replace.ContainsText == nil || replace.ContainsText.Text != "old" || replace.ReplaceText != "new" {
				t.Fatalf("unexpected replace request: %#v", replace)
			}
			if replace.TabsCriteria == nil || len(replace.TabsCriteria.TabIds) != 1 || replace.TabsCriteria.TabIds[0] != "tab1" {
				t.Fatalf("unexpected tabs criteria: %#v", replace.TabsCriteria)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies": []any{
					map[string]any{"replaceAllText": map[string]any{"occurrencesChanged": 2}},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsReplaceCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--match", "old", "--replace", "new", "--tab", "tab1"}, ctx, flags); err != nil {
		t.Fatalf("replace: %v", err)
	}
}
