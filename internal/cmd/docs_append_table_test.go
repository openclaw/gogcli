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
)

// Regression for #592: `gog docs write --markdown --append` silently dropped
// any markdown table in the appended content, failing with
// "insert native table: table not found near index N".
//
// The bug: getTableCellIndices used a fixed ±2 window when locating the
// freshly-inserted table in the post-write document. In practice the Docs API
// reports the new table's StartIndex with a drift larger than 2 code units
// from the requested Location.Index — depending on the surrounding paragraph
// and the auto-newline behavior documented for InsertTableRequest, the
// post-insert StartIndex can be several units past tableStartIndex. When the
// match window misses the table the inserter returns
// `table not found near index N` and the table is silently dropped. The `docs
// create --file --markdown` path is unaffected because it uses Drive's
// native markdown import end-to-end and never calls InsertNativeTable.
//
// The fix replaces the tight ±2 window with a nearest-table search that
// accepts any Table element at or after (tableStartIndex - small_tolerance)
// and picks the closest StartIndex — robust to the API's actual drift while
// still ruling out unrelated tables earlier in the document.

// fakeDocsTableServer is a minimal Docs-API mock. On every InsertTable
// batchUpdate it materialises a synthetic Table structural element so the
// subsequent Documents.Get call returns a body the inserter can walk. The
// table's reported StartIndex is `Location.Index + tableDrift`, letting tests
// model the real-world drift that broke the ±2 window.
type fakeDocsTableServer struct {
	t          *testing.T
	body       string // current body text (no table)
	tableDrift int64  // drift between Location.Index and reported StartIndex
	hasTable   bool
	tableIdx   int64
	tableRows  int64
	tableCols  int64
	batchCalls [][]*docs.Request
}

func (f *fakeDocsTableServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/documents/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(f.docPayload())
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				f.t.Fatalf("read batchUpdate body: %v", err)
			}
			var req docs.BatchUpdateDocumentRequest
			if err := json.Unmarshal(body, &req); err != nil {
				f.t.Fatalf("decode batchUpdate body: %v", err)
			}
			f.batchCalls = append(f.batchCalls, req.Requests)

			for _, rq := range req.Requests {
				if rq.InsertTable != nil {
					f.hasTable = true
					f.tableIdx = rq.InsertTable.Location.Index + f.tableDrift
					f.tableRows = rq.InsertTable.Rows
					f.tableCols = rq.InsertTable.Columns
				}
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		default:
			http.NotFound(w, r)
		}
	}
}

func (f *fakeDocsTableServer) docPayload() map[string]any {
	content := []any{
		map[string]any{
			"startIndex":   0,
			"endIndex":     1,
			"sectionBreak": map[string]any{"sectionStyle": map[string]any{}},
		},
		map[string]any{
			"startIndex": 1,
			"endIndex":   int64(1 + len(f.body)),
			"paragraph": map[string]any{
				"elements": []any{
					map[string]any{
						"startIndex": 1,
						"endIndex":   int64(1 + len(f.body)),
						"textRun":    map[string]any{"content": f.body},
					},
				},
			},
		},
	}
	if f.hasTable {
		idx := f.tableIdx
		rows := make([]any, 0, f.tableRows)
		for r := int64(0); r < f.tableRows; r++ {
			cells := make([]any, 0, f.tableCols)
			for c := int64(0); c < f.tableCols; c++ {
				idx++ // cell-start marker
				cellStart := idx
				cells = append(cells, map[string]any{
					"startIndex": cellStart,
					"endIndex":   cellStart + 1,
					"content": []any{map[string]any{
						"startIndex": cellStart,
						"endIndex":   cellStart + 1,
						"paragraph": map[string]any{
							"elements": []any{
								map[string]any{
									"startIndex": cellStart,
									"endIndex":   cellStart + 1,
									"textRun":    map[string]any{"content": "\n"},
								},
							},
						},
					}},
				})
				idx++ // end of cell paragraph
			}
			rows = append(rows, map[string]any{"tableCells": cells})
		}
		tableEnd := idx + 1
		content = append(content, map[string]any{
			"startIndex": f.tableIdx,
			"endIndex":   tableEnd,
			"table": map[string]any{
				"rows":      f.tableRows,
				"columns":   f.tableCols,
				"tableRows": rows,
			},
		})
	}
	return map[string]any{
		"documentId": "doc1",
		"body":       map[string]any{"content": content},
	}
}

func newFakeDocsTableSvc(t *testing.T, body string, drift int64) (*docs.Service, *fakeDocsTableServer) {
	t.Helper()
	f := &fakeDocsTableServer{t: t, body: body, tableDrift: drift}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	svc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	return svc, f
}

// TestInsertDocsMarkdownAt_AppendsTable_IssueRepro replays the exact repro
// from #592 — a table-only markdown file appended to a doc via the same code
// path `gog docs write --markdown --append` exercises. The fake server reports
// the inserted table with a drift of 5 from the requested Location.Index, well
// outside the original ±2 search window; without the fix this fails with
// "insert native table: table not found near index 9".
func TestInsertDocsMarkdownAt_AppendsTable_IssueRepro(t *testing.T) {
	svc, fake := newFakeDocsTableSvc(t, "Existing\n", 5)

	markdown := strings.Join([]string{
		"| API call | Type | Vuln class |",
		"|---|---|---|",
		"| HttpServletRequest.getParameter | SOURCE | XSS |",
		"| HttpServletResponse.sendRedirect | SINK | open redirect |",
		"",
	}, "\n")

	// Initial body is "Existing\n" (9 chars), so the document endIndex is 10
	// and docsAppendIndex(10) = 9.
	const insertIdx int64 = 9

	if _, _, err := insertDocsMarkdownAt(context.Background(), svc, "doc1", insertIdx, markdown, ""); err != nil {
		t.Fatalf("insertDocsMarkdownAt: %v", err)
	}

	if !fake.hasTable {
		t.Fatalf("expected InsertTable request, got %d batch calls: %#v", len(fake.batchCalls), fake.batchCalls)
	}
	if fake.tableRows != 3 || fake.tableCols != 3 {
		t.Fatalf("expected 3x3 table, got rows=%d cols=%d", fake.tableRows, fake.tableCols)
	}

	if len(fake.batchCalls) < 2 {
		t.Fatalf("expected at least 2 batchUpdate calls (text + table), got %d", len(fake.batchCalls))
	}

	first := fake.batchCalls[0]
	var sawInsertText bool
	for _, rq := range first {
		if rq.InsertText != nil {
			sawInsertText = true
			if rq.InsertText.Location.Index != insertIdx {
				t.Fatalf("first InsertText at %d, want %d", rq.InsertText.Location.Index, insertIdx)
			}
		}
	}
	if !sawInsertText {
		t.Fatalf("first batch should carry InsertText, got %#v", first)
	}
}

// TestInsertDocsMarkdownAt_AppendsTableWithLeadingParagraph covers the mixed
// case from the issue: prose followed by a trailing table. Prior to the fix
// the table was silently dropped while the prose appended successfully.
func TestInsertDocsMarkdownAt_AppendsTableWithLeadingParagraph(t *testing.T) {
	svc, fake := newFakeDocsTableSvc(t, "Existing\n", 4)

	markdown := strings.Join([]string{
		"Some intro paragraph that should append.",
		"",
		"| col1 | col2 |",
		"|---|---|",
		"| a | b |",
		"",
	}, "\n")

	const insertIdx int64 = 9

	if _, _, err := insertDocsMarkdownAt(context.Background(), svc, "doc1", insertIdx, markdown, ""); err != nil {
		t.Fatalf("insertDocsMarkdownAt: %v", err)
	}
	if !fake.hasTable {
		t.Fatalf("expected InsertTable request for trailing table; batches=%#v", fake.batchCalls)
	}
	if fake.tableRows != 2 || fake.tableCols != 2 {
		t.Fatalf("expected 2x2 table, got %dx%d", fake.tableRows, fake.tableCols)
	}
}

// TestPickTableNear_PrefersClosestForwardMatch documents the search semantics
// the fix relies on: among multiple Table elements in the body, pick the one
// whose StartIndex is closest to tableStartIndex but not more than the small
// backward tolerance behind it. This is what lets us tolerate the Docs API's
// observed +N drift on append without misidentifying unrelated tables that
// already lived in the document.
func TestPickTableNear_PrefersClosestForwardMatch(t *testing.T) {
	mkTable := func(start, end int64) *docs.StructuralElement {
		return &docs.StructuralElement{
			StartIndex: start,
			EndIndex:   end,
			Table:      &docs.Table{Rows: 1, Columns: 1, TableRows: []*docs.TableRow{}},
		}
	}

	tests := []struct {
		name       string
		content    []*docs.StructuralElement
		target     int64
		wantStart  int64
		wantNilHit bool
	}{
		{
			name:      "exact match",
			content:   []*docs.StructuralElement{mkTable(100, 120)},
			target:    100,
			wantStart: 100,
		},
		{
			name:      "drift_+1",
			content:   []*docs.StructuralElement{mkTable(101, 121)},
			target:    100,
			wantStart: 101,
		},
		{
			name:      "drift_+5",
			content:   []*docs.StructuralElement{mkTable(105, 125)},
			target:    100,
			wantStart: 105,
		},
		{
			name:      "drift_+25_still_picked_when_only_candidate",
			content:   []*docs.StructuralElement{mkTable(125, 145)},
			target:    100,
			wantStart: 125,
		},
		{
			name: "earlier_table_outside_backward_tolerance_is_ignored",
			content: []*docs.StructuralElement{
				mkTable(50, 70),   // existing table earlier in doc
				mkTable(101, 121), // the freshly-inserted table
			},
			target:    100,
			wantStart: 101,
		},
		{
			name: "tie_breaker_picks_forward_drift",
			content: []*docs.StructuralElement{
				mkTable(99, 110),
				mkTable(101, 120),
			},
			target:    100,
			wantStart: 99, // 99 and 101 are equidistant; iteration order picks first encountered
		},
		{
			name:       "no_tables",
			content:    []*docs.StructuralElement{},
			target:     100,
			wantNilHit: true,
		},
		{
			name: "only_far_backward_table_is_rejected",
			content: []*docs.StructuralElement{
				mkTable(10, 20),
			},
			target:     100,
			wantNilHit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickTableNear(tt.content, tt.target, 1, 1)
			if tt.wantNilHit {
				if got != nil {
					t.Fatalf("expected nil, got element at %d", got.StartIndex)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected element at %d, got nil", tt.wantStart)
			}
			if got.StartIndex != tt.wantStart {
				t.Fatalf("matched StartIndex = %d, want %d", got.StartIndex, tt.wantStart)
			}
		})
	}
}

func TestPickTableNear_IgnoresWrongDimensions(t *testing.T) {
	mkTable := func(start, end, rows, cols int64) *docs.StructuralElement {
		return &docs.StructuralElement{
			StartIndex: start,
			EndIndex:   end,
			Table:      &docs.Table{Rows: rows, Columns: cols, TableRows: []*docs.TableRow{}},
		}
	}

	got := pickTableNear([]*docs.StructuralElement{
		mkTable(101, 121, 1, 1),
		mkTable(105, 145, 2, 3),
	}, 100, 2, 3)
	if got == nil {
		t.Fatal("expected matching table")
	}
	if got.StartIndex != 105 {
		t.Fatalf("matched StartIndex = %d, want 105", got.StartIndex)
	}
}

// TestInsertDocsMarkdownAt_TableErrorIsActionable guards the wrapped error
// message so the original symptom of #592 stays diagnostically searchable in
// logs when the Docs API genuinely does not produce a table.
func TestInsertDocsMarkdownAt_TableErrorIsActionable(t *testing.T) {
	var batchCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(docBodyWithText("Existing\n"))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			batchCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}

	markdown := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	_, _, err = insertDocsMarkdownAt(context.Background(), svc, "doc1", 9, markdown, "")
	if err == nil {
		t.Fatal("expected error when server has no table; got nil")
	}
	if !strings.Contains(err.Error(), "insert native table") {
		t.Fatalf("error should be wrapped with 'insert native table'; got %v", err)
	}
	if batchCalls < 2 {
		t.Fatalf("expected >=2 batchUpdate calls before failure, got %d", batchCalls)
	}
}
