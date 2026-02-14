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
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDocsCreateCopyCat_JSON(t *testing.T) {
	origNew := newDriveService
	origDocs := newDocsService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		newDocsService = origDocs
		driveExportDownload = origExport
	})

	driveExportDownload = func(context.Context, *drive.Service, string, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("doc text")),
		}, nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		drivePath := strings.TrimPrefix(path, "/drive/v3")
		switch {
		case strings.HasPrefix(path, "/v1/documents/") && r.Method == http.MethodGet:
			id := strings.TrimPrefix(path, "/v1/documents/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": id,
				"title":      "Doc",
				"body": map[string]any{
					"content": []any{
						map[string]any{
							"paragraph": map[string]any{
								"elements": []any{
									map[string]any{
										"textRun": map[string]any{
											"content": "doc text",
										},
									},
								},
							},
						},
					},
				},
			})
			return
		case strings.HasPrefix(drivePath, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "doc1",
				"mimeType": "application/vnd.google-apps.document",
			})
			return
		case drivePath == "/files" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "doc1",
				"name":        "Doc",
				"mimeType":    "application/vnd.google-apps.document",
				"webViewLink": "http://example.com/doc1",
			})
			return
		case strings.Contains(drivePath, "/files/doc1/copy") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "doc2",
				"name":        "Copy",
				"mimeType":    "application/vnd.google-apps.document",
				"webViewLink": "http://example.com/doc2",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

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
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	_ = captureStdout(t, func() {
		cmd := &DocsCreateCmd{}
		if err := runKong(t, cmd, []string{"Doc"}, ctx, flags); err != nil {
			t.Fatalf("create: %v", err)
		}
	})

	_ = captureStdout(t, func() {
		cmd := &DocsCopyCmd{}
		if err := runKong(t, cmd, []string{"doc1", "Copy"}, ctx, flags); err != nil {
			t.Fatalf("copy: %v", err)
		}
	})

	out := captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		if err := runKong(t, cmd, []string{"doc1"}, ctx, flags); err != nil {
			t.Fatalf("cat: %v", err)
		}
	})
	if !strings.Contains(out, "doc text") {
		t.Fatalf("unexpected cat output: %q", out)
	}
}

// tabsDocResponse returns a JSON response for a document with multiple tabs
// (using includeTabsContent=true). The body/content fields are empty because
// the Docs API populates doc.Tabs instead when that flag is set.
func tabsDocResponse(id string) map[string]any {
	return map[string]any{
		"documentId": id,
		"title":      "Multi-Tab Doc",
		"tabs": []any{
			map[string]any{
				"tabProperties": map[string]any{
					"tabId": "t.0",
					"title": "Overview",
					"index": 0,
				},
				"documentTab": map[string]any{
					"body": map[string]any{
						"content": []any{
							map[string]any{
								"paragraph": map[string]any{
									"elements": []any{
										map[string]any{
											"textRun": map[string]any{"content": "overview text"},
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
					"tabId": "t.abc",
					"title": "Details",
					"index": 1,
				},
				"documentTab": map[string]any{
					"body": map[string]any{
						"content": []any{
							map[string]any{
								"paragraph": map[string]any{
									"elements": []any{
										map[string]any{
											"textRun": map[string]any{"content": "details text"},
										},
									},
								},
							},
						},
					},
				},
				"childTabs": []any{
					map[string]any{
						"tabProperties": map[string]any{
							"tabId":        "t.child1",
							"title":        "Sub-Detail",
							"index":        0,
							"nestingLevel": 1,
							"parentTabId":  "t.abc",
						},
						"documentTab": map[string]any{
							"body": map[string]any{
								"content": []any{
									map[string]any{
										"paragraph": map[string]any{
											"elements": []any{
												map[string]any{
													"textRun": map[string]any{"content": "child text"},
												},
											},
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

func newTabsTestServer(t *testing.T) (*docs.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/v1/documents/") && r.Method == http.MethodGet {
			id := strings.TrimPrefix(path, "/v1/documents/")
			w.Header().Set("Content-Type", "application/json")
			// Check if includeTabsContent is requested.
			if r.URL.Query().Get("includeTabsContent") == "true" {
				_ = json.NewEncoder(w).Encode(tabsDocResponse(id))
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"documentId": id,
					"title":      "Multi-Tab Doc",
					"body": map[string]any{
						"content": []any{
							map[string]any{
								"paragraph": map[string]any{
									"elements": []any{
										map[string]any{
											"textRun": map[string]any{"content": "overview text"},
										},
									},
								},
							},
						},
					},
				})
			}
			return
		}
		http.NotFound(w, r)
	}))

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}

	return docSvc, srv.Close
}

func TestDocsCat_DefaultNoTabs(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	out := captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		if err := runKong(t, cmd, []string{"doc1"}, ctx, flags); err != nil {
			t.Fatalf("cat: %v", err)
		}
	})
	if !strings.Contains(out, "overview text") {
		t.Fatalf("expected default tab text, got: %q", out)
	}
	if strings.Contains(out, "=== Tab:") {
		t.Fatal("default mode should not show tab headers")
	}
}

func TestDocsCat_AllTabs(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	out := captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		if err := runKong(t, cmd, []string{"doc1", "--all-tabs"}, ctx, flags); err != nil {
			t.Fatalf("cat --all-tabs: %v", err)
		}
	})
	if !strings.Contains(out, "=== Tab: Overview ===") {
		t.Fatalf("missing Overview tab header in: %q", out)
	}
	if !strings.Contains(out, "=== Tab: Details ===") {
		t.Fatalf("missing Details tab header in: %q", out)
	}
	if !strings.Contains(out, "=== Tab: Sub-Detail ===") {
		t.Fatalf("missing Sub-Detail (child) tab header in: %q", out)
	}
	if !strings.Contains(out, "overview text") || !strings.Contains(out, "details text") || !strings.Contains(out, "child text") {
		t.Fatalf("missing tab content in: %q", out)
	}
}

func TestDocsCat_AllTabs_JSON(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		if err := runKong(t, cmd, []string{"doc1", "--all-tabs"}, ctx, flags); err != nil {
			t.Fatalf("cat --all-tabs --json: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("JSON parse: %v\nraw: %q", err, out)
	}
	tabs, ok := result["tabs"].([]any)
	if !ok || len(tabs) != 3 {
		t.Fatalf("expected 3 tabs in JSON, got: %v", result)
	}
	first := tabs[0].(map[string]any)
	if first["title"] != "Overview" || first["id"] != "t.0" {
		t.Fatalf("unexpected first tab: %v", first)
	}
}

func TestDocsCat_SingleTab(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	// By title.
	out := captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		if err := runKong(t, cmd, []string{"doc1", "--tab", "Details"}, ctx, flags); err != nil {
			t.Fatalf("cat --tab Details: %v", err)
		}
	})
	if !strings.Contains(out, "details text") {
		t.Fatalf("expected details text, got: %q", out)
	}
	if strings.Contains(out, "overview text") {
		t.Fatal("should not contain other tab text")
	}

	// By ID.
	out = captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		if err := runKong(t, cmd, []string{"doc1", "--tab", "t.child1"}, ctx, flags); err != nil {
			t.Fatalf("cat --tab t.child1: %v", err)
		}
	})
	if !strings.Contains(out, "child text") {
		t.Fatalf("expected child text, got: %q", out)
	}
}

func TestDocsCat_TabNotFound(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DocsCatCmd{}
	err := runKong(t, cmd, []string{"doc1", "--tab", "Nonexistent"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "tab not found") {
		t.Fatalf("expected tab not found error, got: %v", err)
	}
}

func TestDocsListTabs(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	// Text output.
	out := captureStdout(t, func() {
		u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		ctx := ui.WithUI(context.Background(), u)
		cmd := &DocsListTabsCmd{}
		if err := runKong(t, cmd, []string{"doc1"}, ctx, flags); err != nil {
			t.Fatalf("list-tabs: %v", err)
		}
	})
	if !strings.Contains(out, "t.0") || !strings.Contains(out, "Overview") {
		t.Fatalf("missing tab info in: %q", out)
	}
	if !strings.Contains(out, "t.abc") || !strings.Contains(out, "Details") {
		t.Fatalf("missing tab info in: %q", out)
	}
	if !strings.Contains(out, "t.child1") || !strings.Contains(out, "Sub-Detail") {
		t.Fatalf("missing child tab info in: %q", out)
	}
}

func TestDocsListTabs_JSON(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		cmd := &DocsListTabsCmd{}
		if err := runKong(t, cmd, []string{"doc1"}, ctx, flags); err != nil {
			t.Fatalf("list-tabs --json: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("JSON parse: %v\nraw: %q", err, out)
	}
	tabs, ok := result["tabs"].([]any)
	if !ok || len(tabs) != 3 {
		t.Fatalf("expected 3 tabs, got: %v", result)
	}
	child := tabs[2].(map[string]any)
	if child["id"] != "t.child1" || child["parentTabId"] != "t.abc" {
		t.Fatalf("unexpected child tab: %v", child)
	}
}

func TestDocsCat_SingleTab_JSON(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		if err := runKong(t, cmd, []string{"doc1", "--tab", "Overview"}, ctx, flags); err != nil {
			t.Fatalf("cat --tab Overview --json: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("JSON parse: %v\nraw: %q", err, out)
	}
	tab, ok := result["tab"].(map[string]any)
	if !ok {
		t.Fatalf("expected tab object, got: %v", result)
	}
	if tab["title"] != "Overview" || tab["text"] != "overview text" {
		t.Fatalf("unexpected tab: %v", tab)
	}
}

func TestDocsCat_CaseInsensitiveTabTitle(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	out := captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		if err := runKong(t, cmd, []string{"doc1", "--tab", "details"}, ctx, flags); err != nil {
			t.Fatalf("cat --tab details (lowercase): %v", err)
		}
	})
	if !strings.Contains(out, "details text") {
		t.Fatalf("case-insensitive match failed, got: %q", out)
	}
}

func TestDocsCat_BackwardCompatibility(t *testing.T) {
	// Verify that docs cat without --tab or --all-tabs does NOT send
	// includeTabsContent parameter (backward compatible).
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var gotIncludeTabs bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("includeTabsContent") == "true" {
			gotIncludeTabs = true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"documentId": "doc1",
			"title":      "Doc",
			"body": map[string]any{
				"content": []any{
					map[string]any{
						"paragraph": map[string]any{
							"elements": []any{
								map[string]any{
									"textRun": map[string]any{"content": "hello"},
								},
							},
						},
					},
				},
			},
		})
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
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	_ = captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		if err := runKong(t, cmd, []string{"doc1"}, ctx, flags); err != nil {
			t.Fatalf("cat: %v", err)
		}
	})

	if gotIncludeTabs {
		t.Fatal("default cat should NOT send includeTabsContent=true")
	}
}

func TestDocsCat_TabSendsIncludeTabsContent(t *testing.T) {
	// Verify that --tab sends includeTabsContent=true.
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var gotIncludeTabs bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("includeTabsContent") == "true" {
			gotIncludeTabs = true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tabsDocResponse("doc1"))
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
	u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	ctx := ui.WithUI(context.Background(), u)

	_ = captureStdout(t, func() {
		cmd := &DocsCatCmd{}
		_ = runKong(t, cmd, []string{"doc1", "--tab", "Overview"}, ctx, flags)
	})

	if !gotIncludeTabs {
		t.Fatal("--tab should send includeTabsContent=true")
	}
}

func TestDocsCreateCopyCat_Text(t *testing.T) {
	origNew := newDriveService
	origDocs := newDocsService
	origExport := driveExportDownload
	t.Cleanup(func() {
		newDriveService = origNew
		newDocsService = origDocs
		driveExportDownload = origExport
	})

	driveExportDownload = func(context.Context, *drive.Service, string, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("doc text")),
		}, nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		drivePath := strings.TrimPrefix(path, "/drive/v3")
		switch {
		case strings.HasPrefix(path, "/v1/documents/") && r.Method == http.MethodGet:
			id := strings.TrimPrefix(path, "/v1/documents/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": id,
				"title":      "Doc",
				"body": map[string]any{
					"content": []any{
						map[string]any{
							"paragraph": map[string]any{
								"elements": []any{
									map[string]any{
										"textRun": map[string]any{
											"content": "doc text",
										},
									},
								},
							},
						},
					},
				},
			})
			return
		case strings.HasPrefix(drivePath, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "doc1",
				"mimeType": "application/vnd.google-apps.document",
			})
			return
		case drivePath == "/files" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "doc1",
				"name":        "Doc",
				"mimeType":    "application/vnd.google-apps.document",
				"webViewLink": "http://example.com/doc1",
			})
			return
		case strings.Contains(drivePath, "/files/doc1/copy") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "doc2",
				"name":        "Copy",
				"mimeType":    "application/vnd.google-apps.document",
				"webViewLink": "http://example.com/doc2",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

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

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		createCmd := &DocsCreateCmd{}
		if err := runKong(t, createCmd, []string{"Doc"}, ctx, flags); err != nil {
			t.Fatalf("create: %v", err)
		}

		copyCmd := &DocsCopyCmd{}
		if err := runKong(t, copyCmd, []string{"doc1", "Copy"}, ctx, flags); err != nil {
			t.Fatalf("copy: %v", err)
		}

		catCmd := &DocsCatCmd{}
		if err := runKong(t, catCmd, []string{"doc1"}, ctx, flags); err != nil {
			t.Fatalf("cat: %v", err)
		}
	})
	if !strings.Contains(out, "doc text") || !strings.Contains(out, "id\tdoc1") {
		t.Fatalf("unexpected output: %q", out)
	}
}
