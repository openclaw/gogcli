package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestDocsCreateCopyCat_JSON(t *testing.T) {
	t.Parallel()

	export := func(context.Context, *drive.Service, string, string) (*http.Response, error) {
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

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	flags := &RootFlags{Account: "a@b.com"}
	var stdout, stderr bytes.Buffer
	ctx := withDriveTestOperations(newCmdRuntimeJSONOutputContext(t, &stdout, &stderr), svc, nil, export)
	ctx = withDocsTestService(ctx, docSvc)

	cmd := &DocsCreateCmd{}
	if err := runKong(t, cmd, []string{"Doc"}, ctx, flags); err != nil {
		t.Fatalf("create: %v", err)
	}

	stdout.Reset()
	cmdCopy := &DocsCopyCmd{}
	if err := runKong(t, cmdCopy, []string{"doc1", "Copy"}, ctx, flags); err != nil {
		t.Fatalf("copy: %v", err)
	}

	stdout.Reset()
	cmdCat := &DocsCatCmd{}
	if err := runKong(t, cmdCat, []string{"doc1"}, ctx, flags); err != nil {
		t.Fatalf("cat: %v", err)
	}
	if !strings.Contains(stdout.String(), "doc text") {
		t.Fatalf("unexpected cat output: %q", stdout.String())
	}
}

func TestDocsCreate_Pageless(t *testing.T) {
	t.Parallel()

	var batchRequests [][]*docs.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		drivePath := strings.TrimPrefix(path, "/drive/v3")
		switch {
		case drivePath == "/files" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "doc1",
				"name":        "Doc",
				"mimeType":    "application/vnd.google-apps.document",
				"webViewLink": "http://example.com/doc1",
			})
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	driveSvc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withDocsTestService(newDocsJSONContextWithDrive(t, driveSvc), docSvc)

	cmd := &DocsCreateCmd{}
	if err := runKong(t, cmd, []string{"Doc", "--pageless"}, ctx, flags); err != nil {
		t.Fatalf("create pageless: %v", err)
	}

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 pageless batch request, got %d", len(batchRequests))
	}
	if got := batchRequests[0]; len(got) != 1 || got[0].UpdateDocumentStyle == nil {
		t.Fatalf("unexpected pageless create request: %#v", got)
	}
	if got := batchRequests[0][0].UpdateDocumentStyle; got.Fields != "documentFormat" || got.DocumentStyle.DocumentFormat.DocumentMode != "PAGELESS" {
		t.Fatalf("unexpected pageless create style request: %#v", got)
	}
}

func TestDocsCreate_DryRunDoesNotOpenService(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	ctx := withDriveTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, &stdout, &stderr),
		func(context.Context, string) (*drive.Service, error) {
			t.Fatal("Drive service should not be called during dry-run")
			return nil, errors.New("unexpected Drive service call")
		},
	)
	ctx = withDocsTestServiceFactory(ctx, func(context.Context, string) (*docs.Service, error) {
		t.Fatal("Docs service should not be called during dry-run")
		return nil, errors.New("unexpected Docs service call")
	})
	err := (&DocsCreateCmd{
		Title:    "Dry Run",
		Parent:   "folder1",
		Pageless: true,
	}).Run(ctx, &RootFlags{Account: "a@b.com", DryRun: true})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 0 {
		t.Fatalf("expected dry-run exit 0, got %v", err)
	}

	var payload struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			File     drive.File `json:"file"`
			Parent   string     `json:"parent"`
			Pageless bool       `json:"pageless"`
		} `json:"request"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode dry-run: %v\nout=%q", err, stdout.String())
	}
	if !payload.DryRun || payload.Op != "docs.create" || payload.Request.File.Name != "Dry Run" || payload.Request.Parent != "folder1" || !payload.Request.Pageless {
		t.Fatalf("unexpected dry-run output: %#v", payload)
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

func runDocsCatCommand(t *testing.T, svc *docs.Service, args []string, jsonMode bool) executeTestResult {
	t.Helper()

	var stdout, stderr bytes.Buffer
	var ctx context.Context
	if jsonMode {
		ctx = newCmdRuntimeJSONOutputContext(t, &stdout, &stderr)
	} else {
		ctx = newCmdRuntimeOutputContext(t, &stdout, &stderr)
	}
	ctx = withDocsTestService(ctx, svc)
	err := runKong(t, &DocsCatCmd{}, args, ctx, &RootFlags{Account: "a@b.com"})
	return executeTestResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
}

func TestDocsCat_DefaultNoTabs(t *testing.T) {
	t.Parallel()

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()

	result := runDocsCatCommand(t, docSvc, []string{"doc1"}, false)
	if result.err != nil {
		t.Fatalf("cat: %v", result.err)
	}
	out := result.stdout
	if !strings.Contains(out, "overview text") {
		t.Fatalf("expected default tab text, got: %q", out)
	}
	if strings.Contains(out, "=== Tab:") {
		t.Fatal("default mode should not show tab headers")
	}
}

func TestDocsCat_AllTabs(t *testing.T) {
	t.Parallel()

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()

	result := runDocsCatCommand(t, docSvc, []string{"doc1", "--all-tabs"}, false)
	if result.err != nil {
		t.Fatalf("cat --all-tabs: %v", result.err)
	}
	out := result.stdout
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
	t.Parallel()

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()

	execResult := runDocsCatCommand(t, docSvc, []string{"doc1", "--all-tabs"}, true)
	if execResult.err != nil {
		t.Fatalf("cat --all-tabs --json: %v", execResult.err)
	}
	out := execResult.stdout

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

func TestDocsCat_RejectsTabWithAllTabs(t *testing.T) {
	t.Parallel()

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, "unexpected Docs API request", http.StatusInternalServerError)
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

	result := runDocsCatCommand(t, docSvc, []string{"doc1", "--tab", "Overview", "--all-tabs"}, false)
	if result.err == nil || !strings.Contains(result.err.Error(), "--tab and --all-tabs cannot be used together") {
		t.Fatalf("expected tab/all-tabs usage error, got: %v", result.err)
	}

	rawResult := runDocsCatCommand(t, docSvc, []string{"doc1", "--raw", "--tab", "Overview", "--all-tabs"}, false)
	if rawResult.err == nil || !strings.Contains(rawResult.err.Error(), "--tab and --all-tabs cannot be used together") {
		t.Fatalf("expected raw tab/all-tabs usage error, got: %v", rawResult.err)
	}

	emptyTabResult := runDocsCatCommand(t, docSvc, []string{"doc1", "--tab", " "}, false)
	if emptyTabResult.err == nil || !strings.Contains(emptyTabResult.err.Error(), "--tab cannot be empty") {
		t.Fatalf("expected empty tab usage error, got: %v", emptyTabResult.err)
	}

	emptyRawTabResult := runDocsCatCommand(t, docSvc, []string{"doc1", "--raw", "--tab", " ", "--all-tabs"}, false)
	if emptyRawTabResult.err == nil || !strings.Contains(emptyRawTabResult.err.Error(), "--tab cannot be empty") {
		t.Fatalf("expected raw empty tab usage error, got: %v", emptyRawTabResult.err)
	}

	explicitEmptyTabResult := runDocsCatCommand(t, docSvc, []string{"doc1", "--tab="}, false)
	if explicitEmptyTabResult.err == nil || !strings.Contains(explicitEmptyTabResult.err.Error(), "--tab cannot be empty") {
		t.Fatalf("expected explicit empty tab usage error, got: %v", explicitEmptyTabResult.err)
	}

	explicitEmptyRawTabResult := runDocsCatCommand(t, docSvc, []string{"doc1", "--raw", "--tab=", "--all-tabs"}, false)
	if explicitEmptyRawTabResult.err == nil || !strings.Contains(explicitEmptyRawTabResult.err.Error(), "--tab cannot be empty") {
		t.Fatalf("expected raw explicit empty tab usage error, got: %v", explicitEmptyRawTabResult.err)
	}
	if requests != 0 {
		t.Fatalf("Docs API requests = %d, want 0", requests)
	}
}

func TestDocsCat_Raw(t *testing.T) {
	t.Parallel()

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()

	execResult := runDocsCatCommand(t, docSvc, []string{"doc1", "--raw"}, false)
	if execResult.err != nil {
		t.Fatalf("cat --raw: %v", execResult.err)
	}
	out := execResult.stdout

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("raw JSON parse: %v\nraw: %q", err, out)
	}
	// Raw output should contain the documentId field from the API response.
	if result["documentId"] != "doc1" {
		t.Fatalf("expected documentId=doc1, got: %v", result["documentId"])
	}
	// Should be pretty-printed (contain newlines + indentation).
	if !strings.Contains(out, "\n  ") {
		t.Fatal("expected pretty-printed JSON with indentation")
	}
}

func TestDocsCat_Raw_AllTabs(t *testing.T) {
	t.Parallel()

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()

	execResult := runDocsCatCommand(t, docSvc, []string{"doc1", "--raw", "--all-tabs"}, false)
	if execResult.err != nil {
		t.Fatalf("cat --raw --all-tabs: %v", execResult.err)
	}
	out := execResult.stdout

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("raw JSON parse: %v\nraw: %q", err, out)
	}
	// With --all-tabs, the raw response should include tabs content.
	if _, ok := result["tabs"]; !ok {
		t.Fatal("expected tabs field in raw --all-tabs output")
	}
}

func TestDocsCat_SingleTab(t *testing.T) {
	t.Parallel()

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()

	// By title.
	result := runDocsCatCommand(t, docSvc, []string{"doc1", "--tab", "Details"}, false)
	if result.err != nil {
		t.Fatalf("cat --tab Details: %v", result.err)
	}
	out := result.stdout
	if !strings.Contains(out, "details text") {
		t.Fatalf("expected details text, got: %q", out)
	}
	if strings.Contains(out, "overview text") {
		t.Fatal("should not contain other tab text")
	}

	// By ID.
	result = runDocsCatCommand(t, docSvc, []string{"doc1", "--tab", "t.child1"}, false)
	if result.err != nil {
		t.Fatalf("cat --tab t.child1: %v", result.err)
	}
	out = result.stdout
	if !strings.Contains(out, "child text") {
		t.Fatalf("expected child text, got: %q", out)
	}
}

func TestDocsCat_TabNotFound(t *testing.T) {
	t.Parallel()

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()

	result := runDocsCatCommand(t, docSvc, []string{"doc1", "--tab", "Nonexistent"}, false)
	if result.err == nil || !strings.Contains(result.err.Error(), "tab not found") {
		t.Fatalf("expected tab not found error, got: %v", result.err)
	}
	if !strings.Contains(result.err.Error(), "Overview") || !strings.Contains(result.err.Error(), "Details") {
		t.Fatalf("expected available tab names in error, got: %v", result.err)
	}
}

func TestDocsCat_SingleTab_JSON(t *testing.T) {
	t.Parallel()

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()

	execResult := runDocsCatCommand(t, docSvc, []string{"doc1", "--tab", "Overview"}, true)
	if execResult.err != nil {
		t.Fatalf("cat --tab Overview --json: %v", execResult.err)
	}
	out := execResult.stdout

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
	t.Parallel()

	docSvc, cleanup := newTabsTestServer(t)
	defer cleanup()

	result := runDocsCatCommand(t, docSvc, []string{"doc1", "--tab", "details"}, false)
	if result.err != nil {
		t.Fatalf("cat --tab details (lowercase): %v", result.err)
	}
	out := result.stdout
	if !strings.Contains(out, "details text") {
		t.Fatalf("case-insensitive match failed, got: %q", out)
	}
}

func TestDocsCat_BackwardCompatibility(t *testing.T) {
	t.Parallel()

	// Verify that docs cat without --tab or --all-tabs does NOT send
	// includeTabsContent parameter (backward compatible).
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
	result := runDocsCatCommand(t, docSvc, []string{"doc1"}, false)
	if result.err != nil {
		t.Fatalf("cat: %v", result.err)
	}

	if gotIncludeTabs {
		t.Fatal("default cat should NOT send includeTabsContent=true")
	}
}

func TestDocsCat_TabSendsIncludeTabsContent(t *testing.T) {
	t.Parallel()

	// Verify that --tab sends includeTabsContent=true.
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
	_ = runDocsCatCommand(t, docSvc, []string{"doc1", "--tab", "Overview"}, false)

	if !gotIncludeTabs {
		t.Fatal("--tab should send includeTabsContent=true")
	}
}

func TestDocsCreateCopyCat_Text(t *testing.T) {
	t.Parallel()

	export := func(context.Context, *drive.Service, string, string) (*http.Response, error) {
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

	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	flags := &RootFlags{Account: "a@b.com"}
	var stdout, stderr bytes.Buffer
	ctx := withDriveTestOperations(newCmdRuntimeOutputContext(t, &stdout, &stderr), svc, nil, export)
	ctx = withDocsTestService(ctx, docSvc)

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
	if !strings.Contains(stdout.String(), "doc text") || !strings.Contains(stdout.String(), "id\tdoc1") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}
