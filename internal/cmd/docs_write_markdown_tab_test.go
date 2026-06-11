package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

// TestDocsWrite_MarkdownReplaceWithTab covers the issue #595 workaround:
// --replace --markdown --tab must NOT go through Drive's whole-document
// markdown converter; instead it must wipe the tab's existing body via
// DeleteContentRange and re-render the markdown locally via Docs batchUpdate
// against the targeted tab.
func TestDocsWrite_MarkdownReplaceWithTab(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

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
				t.Fatalf("decode batch request: %v", err)
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
	defer cleanup()

	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		t.Fatal("markdown replace with --tab must not use the Drive converter")
		return nil, errors.New("unexpected Drive service call")
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	markdown := "# Title\n\n**bold**\n"
	if err := runKong(t, &DocsWriteCmd{}, []string{
		"doc1", "--text", markdown, "--replace", "--markdown", "--tab", "Second",
	}, ctx, flags); err != nil {
		t.Fatalf("markdown replace with tab: %v", err)
	}

	if includeTabsCalls != 1 {
		t.Fatalf("expected 1 tab-aware GET, got %d", includeTabsCalls)
	}
	if len(batchRequests) != 2 {
		t.Fatalf("expected 2 batch requests (delete + insert), got %d", len(batchRequests))
	}

	// First batch: DeleteContentRange covering [1, endIndex-1] on the tab.
	deleteReqs := batchRequests[0]
	if len(deleteReqs) != 1 || deleteReqs[0].DeleteContentRange == nil {
		t.Fatalf("expected first batch to be a single DeleteContentRange, got %#v", deleteReqs)
	}
	delRange := deleteReqs[0].DeleteContentRange.Range
	if delRange.TabId != "t.second" {
		t.Fatalf("delete range tab = %q, want t.second", delRange.TabId)
	}
	if delRange.StartIndex != 1 || delRange.EndIndex != 19 {
		// tabsDocWithEndIndex marks Second's endIndex=20 → delete to 19.
		t.Fatalf("delete range = [%d,%d), want [1,19)", delRange.StartIndex, delRange.EndIndex)
	}

	// Second batch: InsertText (tab-scoped, index 1) + formatting requests
	// all carrying the tab id.
	insertReqs := batchRequests[1]
	if len(insertReqs) < 1 || insertReqs[0].InsertText == nil {
		t.Fatalf("expected first request in second batch to be InsertText, got %#v", insertReqs)
	}
	loc := insertReqs[0].InsertText.Location
	if loc.TabId != "t.second" || loc.Index != 1 {
		t.Fatalf("insert location = %+v, want {TabId:t.second Index:1}", loc)
	}
	if got := insertReqs[0].InsertText.Text; got != "Title\nbold\n" {
		t.Fatalf("inserted text = %q, want %q", got, "Title\nbold\n")
	}
	for i, req := range insertReqs[1:] {
		var r *docs.Range
		switch {
		case req.UpdateTextStyle != nil:
			r = req.UpdateTextStyle.Range
		case req.UpdateParagraphStyle != nil:
			r = req.UpdateParagraphStyle.Range
		case req.CreateParagraphBullets != nil:
			r = req.CreateParagraphBullets.Range
		case req.DeleteParagraphBullets != nil:
			r = req.DeleteParagraphBullets.Range
		}
		if r == nil {
			continue
		}
		if r.TabId != "t.second" {
			t.Fatalf("formatting request %d range tab = %q, want t.second", i+1, r.TabId)
		}
	}
}

func TestDocsWrite_MarkdownReplaceTableBreaksUsesLocalRenderer(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

	getCalls := 0
	var batches []docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			getCalls++
			w.Header().Set("Content-Type", "application/json")
			if getCalls == 1 {
				_ = json.NewEncoder(w).Encode(&docs.Document{
					DocumentId: "doc1",
					RevisionId: "rev-1",
					Body: &docs.Body{Content: []*docs.StructuralElement{{
						StartIndex: 1,
						EndIndex:   2,
						Paragraph:  &docs.Paragraph{},
					}}},
				})
			} else {
				_ = json.NewEncoder(w).Encode(docsTableOpsTestDocument(docsTableOpsTestElement(1, "", 2, 2)))
			}
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batch request: %v", err)
			}
			batches = append(batches, req)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		t.Fatal("table-cell breaks must use the local Docs renderer")
		return nil, errors.New("unexpected Drive service call")
	}

	markdown := "| Value | Literal |\n| --- | --- |\n| Alice<br>Bob | `<br>` |"
	if err := runKong(t, &DocsWriteCmd{}, []string{
		"doc1", "--text", markdown, "--replace", "--markdown",
	}, newDocsJSONContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("markdown replace with table breaks: %v", err)
	}

	if getCalls != 2 {
		t.Fatalf("GET calls = %d, want 2", getCalls)
	}
	if len(batches) != 3 {
		t.Fatalf("batch calls = %d, want placeholder, table, and cells", len(batches))
	}
	var inserted []string
	var sawCodeStyle bool
	for _, req := range batches[2].Requests {
		if req.InsertText != nil {
			inserted = append(inserted, req.InsertText.Text)
		}
		if req.UpdateTextStyle != nil && req.UpdateTextStyle.TextStyle != nil &&
			req.UpdateTextStyle.TextStyle.WeightedFontFamily != nil {
			sawCodeStyle = true
		}
	}
	if !slices.Contains(inserted, "Alice\nBob") {
		t.Fatalf("cell inserts = %#v, missing converted line break", inserted)
	}
	if !slices.Contains(inserted, "<br>") || !sawCodeStyle {
		t.Fatalf("cell inserts = %#v, protected code literal/style missing", inserted)
	}
}

func TestDocsWrite_MarkdownReplaceWithTabRewritesExplicitHeadingAnchorLinks(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

	var batchRequests [][]*docs.Request
	var includeTabsCalls int
	var getCalls int

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			if strings.Contains(r.URL.RawQuery, "includeTabsContent=true") {
				includeTabsCalls++
			}
			getCalls++
			w.Header().Set("Content-Type", "application/json")
			if getCalls == 1 {
				_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
				return
			}
			_ = json.NewEncoder(w).Encode(&docs.Document{
				DocumentId: "doc1",
				Tabs: []*docs.Tab{{
					TabProperties: &docs.TabProperties{TabId: "t.second", Title: "Second"},
					DocumentTab: &docs.DocumentTab{Body: &docs.Body{Content: []*docs.StructuralElement{
						{
							StartIndex: 1,
							EndIndex:   7,
							Paragraph: &docs.Paragraph{
								ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1", HeadingId: "h.files"},
								Elements: []*docs.ParagraphElement{{
									StartIndex: 1,
									EndIndex:   7,
									TextRun:    &docs.TextRun{Content: "Files\n"},
								}},
							},
						},
						{
							StartIndex: 7,
							EndIndex:   12,
							Paragraph: &docs.Paragraph{
								Elements: []*docs.ParagraphElement{{
									StartIndex: 7,
									EndIndex:   11,
									TextRun: &docs.TextRun{
										Content:   "Jump",
										TextStyle: &docs.TextStyle{Link: &docs.Link{Url: "#attachments"}},
									},
								}},
							},
						},
					}}},
				}},
			})
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batch request: %v", err)
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
	defer cleanup()

	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		t.Fatal("markdown replace with --tab must not use the Drive converter")
		return nil, errors.New("unexpected Drive service call")
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	markdown := "# Files {#attachments}\n\n[Jump](#attachments)\n"
	if err := runKong(t, &DocsWriteCmd{}, []string{
		"doc1", "--text", markdown, "--replace", "--markdown", "--tab", "Second",
	}, ctx, flags); err != nil {
		t.Fatalf("markdown replace with tab: %v", err)
	}

	if includeTabsCalls != 2 {
		t.Fatalf("expected 2 tab-aware GETs, got %d", includeTabsCalls)
	}
	if len(batchRequests) != 3 {
		t.Fatalf("expected delete + insert + link rewrite batches, got %d", len(batchRequests))
	}
	insertReqs := batchRequests[1]
	if len(insertReqs) == 0 || insertReqs[0].InsertText == nil {
		t.Fatalf("expected insert batch, got %#v", insertReqs)
	}
	if got := insertReqs[0].InsertText.Text; got != "Files\nJump\n" {
		t.Fatalf("inserted text = %q, want explicit anchor stripped", got)
	}
	rewriteReqs := batchRequests[2]
	if len(rewriteReqs) != 1 || rewriteReqs[0].UpdateTextStyle == nil {
		t.Fatalf("expected one link rewrite request, got %#v", rewriteReqs)
	}
	styleReq := rewriteReqs[0].UpdateTextStyle
	if styleReq.Range.TabId != "t.second" || styleReq.Range.StartIndex != 7 || styleReq.Range.EndIndex != 11 {
		t.Fatalf("unexpected rewrite range: %#v", styleReq.Range)
	}
	if styleReq.TextStyle == nil || styleReq.TextStyle.Link == nil ||
		styleReq.TextStyle.Link.Heading == nil ||
		styleReq.TextStyle.Link.Heading.Id != "h.files" ||
		styleReq.TextStyle.Link.Heading.TabId != "t.second" {
		t.Fatalf("unexpected heading link target: %#v", styleReq.TextStyle)
	}
}

func TestDocsWrite_MarkdownReplaceWithTab_NestedLists(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batch request: %v", err)
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
	defer cleanup()

	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		t.Fatal("markdown replace with --tab must not use the Drive converter")
		return nil, errors.New("unexpected Drive service call")
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	markdown := "- Parent\n  - Child\n    - Grandchild\n"
	if err := runKong(t, &DocsWriteCmd{}, []string{
		"doc1", "--text=" + markdown, "--replace", "--markdown", "--tab", "Second",
	}, ctx, flags); err != nil {
		t.Fatalf("markdown replace with nested tab list: %v", err)
	}
	if len(batchRequests) != 2 {
		t.Fatalf("expected 2 batch requests (delete + insert), got %d", len(batchRequests))
	}

	insertReqs := batchRequests[1]
	if len(insertReqs) != 2 {
		t.Fatalf("expected insert plus 1 list-block bullet request, got %#v", insertReqs)
	}
	if got := insertReqs[0].InsertText; got == nil || got.Text != "Parent\n\tChild\n\t\tGrandchild\n" {
		t.Fatalf("unexpected inserted text: %#v", got)
	}
	got := insertReqs[1].CreateParagraphBullets
	if got == nil {
		t.Fatalf("request 1 missing CreateParagraphBullets: %#v", insertReqs[1])
	}
	if got.Range.TabId != "t.second" || got.Range.StartIndex != 1 || got.Range.EndIndex != 28 {
		t.Fatalf("bullet range = %#v, want tab t.second [1,28)", got.Range)
	}
}

// TestDocsWrite_MarkdownReplaceWithTab_EmptyTab verifies that when the
// targeted tab is already empty (endIndex == 1) the DeleteContentRange step
// is skipped — the Docs API rejects a delete range where end <= start.
func TestDocsWrite_MarkdownReplaceWithTab_EmptyTab(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

	var batchRequests [][]*docs.Request

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/documents/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"tabs": []any{
					map[string]any{
						"tabProperties": map[string]any{"tabId": "t.blank", "title": "Blank", "index": 0},
						"documentTab":   map[string]any{"body": map[string]any{"content": []any{}}},
					},
				},
			})
			return
		case r.Method == http.MethodPost && strings.Contains(path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batch request: %v", err)
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
	defer cleanup()

	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		t.Fatal("markdown replace with --tab must not use the Drive converter")
		return nil, errors.New("unexpected Drive service call")
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	if err := runKong(t, &DocsWriteCmd{}, []string{
		"doc1", "--text", "hello\n", "--replace", "--markdown", "--tab", "Blank",
	}, ctx, flags); err != nil {
		t.Fatalf("markdown replace empty tab: %v", err)
	}

	if len(batchRequests) != 1 {
		t.Fatalf("expected 1 batch request (insert only, no delete), got %d", len(batchRequests))
	}
	if batchRequests[0][0].InsertText == nil {
		t.Fatalf("expected first request to be InsertText, got %#v", batchRequests[0][0])
	}
	if loc := batchRequests[0][0].InsertText.Location; loc.TabId != "t.blank" || loc.Index != 1 {
		t.Fatalf("insert location = %+v, want {TabId:t.blank Index:1}", loc)
	}
}

// TestDocsWrite_MarkdownReplaceWithTab_TabNotFound asserts that an unknown
// --tab value still produces the standard tab-not-found error and never
// issues any batchUpdate.
func TestDocsWrite_MarkdownReplaceWithTab_TabNotFound(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

	var batchUpdates int

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate") {
			batchUpdates++
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tabsDocWithEndIndex())
	}))
	defer cleanup()

	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		t.Fatal("tab-not-found path must not invoke Drive")
		return nil, errors.New("unexpected Drive service call")
	}

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newDocsJSONContext(t)

	err := runKong(t, &DocsWriteCmd{}, []string{
		"doc1", "--text", "x", "--replace", "--markdown", "--tab", "Missing",
	}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `tab not found: "Missing"`) {
		t.Fatalf("expected tab-not-found error, got %v", err)
	}
	if batchUpdates != 0 {
		t.Fatalf("expected no batchUpdate calls, got %d", batchUpdates)
	}
}

func TestInsertImagesIntoDocs_TabReadback(t *testing.T) {
	placeholder := markdownImage{index: 0, token: "test"}.placeholder()
	images := []markdownImage{{
		index:       0,
		token:       "test",
		originalRef: "https://example.com/image.png",
	}}

	var sawIncludeTabs bool
	var batchRequests []*docs.Request
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			sawIncludeTabs = strings.Contains(r.URL.RawQuery, "includeTabsContent=true")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"tabs": []any{
					map[string]any{
						"tabProperties": map[string]any{"tabId": "t.second", "title": "Second", "index": 0},
						"documentTab": map[string]any{
							"body": map[string]any{
								"content": []any{
									map[string]any{
										"startIndex": 1,
										"endIndex":   1 + len(placeholder) + 1,
										"paragraph": map[string]any{
											"elements": []any{
												map[string]any{
													"startIndex": 1,
													"endIndex":   1 + len(placeholder) + 1,
													"textRun": map[string]any{
														"content": placeholder + "\n",
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
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			var req docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batch request: %v", err)
			}
			batchRequests = append(batchRequests, req.Requests...)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	if err := insertImagesIntoDocs(context.Background(), docSvc, "doc1", images, "t.second"); err != nil {
		t.Fatalf("insertImagesIntoDocs: %v", err)
	}
	if !sawIncludeTabs {
		t.Fatalf("expected image placeholder readback to request includeTabsContent=true")
	}
	if len(batchRequests) != 2 {
		t.Fatalf("expected delete + image insert requests, got %#v", batchRequests)
	}
	del := batchRequests[0].DeleteContentRange
	if del == nil || del.Range == nil || del.Range.TabId != "t.second" {
		t.Fatalf("delete request missing target tab: %#v", batchRequests[0])
	}
	img := batchRequests[1].InsertInlineImage
	if img == nil || img.Location == nil || img.Location.TabId != "t.second" {
		t.Fatalf("image insert request missing target tab: %#v", batchRequests[1])
	}
}
