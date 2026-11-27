package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
)

const docsSuggestionTestTab = `{"documentId":"doc1","revisionId":"r1","suggestionsViewMode":"SUGGESTIONS_INLINE","tabs":[{"tabProperties":{"tabId":"t.target","title":"Review"},"documentTab":{"body":{"content":[{"startIndex":1,"endIndex":12,"paragraph":{"elements":[{"startIndex":1,"endIndex":12,"textRun":{"content":"😀 example\n"}}]}}]}}}]}`

func TestDocsExistingSuggestionPaths(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		cmd  func() any
		args []string
		ops  []string
	}{
		{"insert", func() any { return &DocsInsertCmd{} }, []string{"doc1", "new", "--at=example"}, []string{"insertText"}},
		{"delete", func() any { return &DocsDeleteCmd{} }, []string{"doc1", "--start=4", "--end=11", "--revision=r1"}, []string{"deleteContentRange"}},
		{"replace", func() any { return &DocsUpdateCmd{} }, []string{"doc1", "--at=example", "--text=sample"}, []string{"deleteContentRange", "insertText"}},
		{"first match deletion", func() any { return &DocsFindReplaceCmd{} }, []string{"doc1", "example", "--first"}, []string{"deleteContentRange"}},
		{"replace all", func() any { return &DocsFindReplaceCmd{} }, []string{"doc1", "example", "sample"}, []string{"replaceAllText"}},
		{"edit", func() any { return &DocsEditCmd{} }, []string{"doc1", "example", "sample"}, []string{"replaceAllText"}},
		{"format", func() any { return &DocsFormatCmd{} }, []string{"doc1", "--match=example", "--bold", "--alignment=center"}, []string{"updateTextStyle", "updateParagraphStyle"}},
		{"page break", func() any { return &DocsInsertPageBreakCmd{} }, []string{"doc1", "--index=4", "--revision=r1"}, []string{"insertPageBreak"}},
		{"section columns at emoji", func() any { return &DocsSectionColumnsCmd{} }, []string{"doc1", "--at=😀", "--count=2"}, []string{"updateSectionStyle"}},
		{"section break", func() any { return &DocsSectionBreakCmd{} }, []string{"doc1", "--at=example", "--type=continuous"}, []string{"insertSectionBreak"}},
		{"table", func() any { return &DocsInsertTableCmd{} }, []string{"doc1", "--index=4", "--revision=r1", "--rows=2", "--cols=2"}, []string{"insertTable"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var reads, writes atomic.Int32
			svc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					reads.Add(1)
					if r.URL.Query().Get("suggestionsViewMode") != docsSuggestionsInline || r.URL.Query().Get("includeTabsContent") != "true" {
						t.Errorf("wrong snapshot query: %s", r.URL.RawQuery)
					}
					fmt.Fprint(w, docsSuggestionTestTab)
					return
				}
				writes.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/v1/documents/doc1:batchUpdate" {
					t.Errorf("unexpected write: %s %s", r.Method, r.URL.Path)
				}
				assertDocsExistingSuggestionWire(t, r.Body, tc.ops)
				response := &docs.BatchUpdateDocumentResponse{
					DocumentId: "doc1", CommentUpdateState: "ALL_SAVED",
					SuggestionResponses: []*docs.SuggestionResponse{{CreatedSuggestionIds: []string{"suggest.id"}}},
				}
				for range tc.ops[1:] {
					response.SuggestionResponses = append(response.SuggestionResponses, &docs.SuggestionResponse{UpdatedSummarySuggestionIds: []string{"suggest.id"}})
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Error(err)
				}
			})
			defer cleanup()
			var out bytes.Buffer
			ctx := withRawTestHTTP(t, withDocsTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc), svc.BasePath)
			ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
			args := append(append([]string{}, tc.args...), "--suggest", "--tab=Review")
			if err := runKong(t, tc.cmd(), args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
				t.Fatal(err)
			}
			if reads.Load() != 1 || writes.Load() != 1 || !strings.Contains(out.String(), "suggest.id") {
				t.Fatalf("reads=%d writes=%d output=%q", reads.Load(), writes.Load(), out.String())
			}
		})
	}
}

func assertDocsExistingSuggestionWire(t *testing.T, reader io.Reader, ops []string) {
	t.Helper()
	var req struct {
		WriteControl docs.WriteControl            `json:"writeControl"`
		Requests     []map[string]json.RawMessage `json:"requests"`
	}
	if err := json.NewDecoder(reader).Decode(&req); err != nil {
		t.Error(err)
		return
	}
	if req.WriteControl.WriteMode != "SUGGEST" || req.WriteControl.RequiredRevisionId != "r1" || req.WriteControl.TargetRevisionId != "" || len(req.Requests) != len(ops) {
		t.Errorf("unexpected request: %+v", req)
		return
	}
	for i, op := range ops {
		var body struct {
			Location     *docs.Location
			Range        *docs.Range
			TabsCriteria *docs.TabsCriteria
		}
		if err := json.Unmarshal(req.Requests[i][op], &body); len(req.Requests[i]) != 1 || err != nil {
			t.Errorf("unexpected operation: %v, %v", req.Requests[i], err)
			continue
		}
		switch {
		case body.Location != nil && body.Location.TabId == "t.target" && body.Location.Index == 4:
		case body.Range != nil && body.Range.TabId == "t.target" && body.Range.StartIndex == 4 && body.Range.EndIndex == 11:
		case op == "updateSectionStyle" && body.Range != nil && body.Range.TabId == "t.target" && body.Range.StartIndex == 1 && body.Range.EndIndex == 3:
		case body.TabsCriteria != nil && len(body.TabsCriteria.TabIds) == 1 && body.TabsCriteria.TabIds[0] == "t.target":
		default:
			t.Errorf("unexpected targeting for %s: %s", op, req.Requests[i][op])
		}
	}
}

func TestDocsSuggestionRejectsUnpinnedOrIncompatibleOptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		cmd  any
		args []string
	}{
		{&DocsInsertCmd{}, []string{"doc1", "text", "--suggest", "--index=1"}},
		{&DocsInsertCmd{}, []string{"doc1", "text", "--suggest", "--markdown"}},
		{&DocsDeleteCmd{}, []string{"doc1", "--suggest", "--at=text", "--batch=b1"}},
		{&DocsUpdateCmd{}, []string{"doc1", "--suggest", "--text=text", "--pageless"}},
		{&DocsUpdateCmd{}, []string{"doc1", "--text=text", "--revision=r1"}},
		{&DocsFormatCmd{}, []string{"doc1", "--suggest", "--bold", "--link=#heading"}},
		{&DocsInsertTableCmd{}, []string{"doc1", "--suggest", "--rows=1", "--cols=1", "--values-json=[[\"text\"]]"}},
		{&DocsFindReplaceCmd{}, []string{"doc1", "find", "replace", "--suggest", "--format=markdown"}},
		{&DocsClearCmd{}, []string{"doc1", "--suggest"}},
	} {
		ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
		if err := runKong(t, tc.cmd, tc.args, ctx, &RootFlags{DryRun: true}); err == nil || ExitCode(err) == 0 {
			t.Errorf("accepted unsafe options: %T %v: %v", tc.cmd, tc.args, err)
		}
	}
}

func TestDocsSuggestionRejectsStaleOrNonInlineSnapshot(t *testing.T) {
	t.Parallel()
	for _, payload := range []string{
		strings.Replace(docsSuggestionTestTab, `"r1"`, `"changed"`, 1),
		strings.Replace(docsSuggestionTestTab, docsSuggestionsInline, "PREVIEW_WITHOUT_SUGGESTIONS", 1),
		strings.Replace(docsSuggestionTestTab, `"r1"`, `""`, 1),
	} {
		svc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Error("unexpected write after unsafe snapshot")
			}
			fmt.Fprint(w, payload)
		})
		ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
		err := runKong(t, &DocsDeleteCmd{}, []string{"doc1", "--start=4", "--end=11", "--revision=r1", "--suggest", "--tab=Review"}, ctx, &RootFlags{Account: "a@b.com"})
		cleanup()
		if err == nil || !strings.Contains(err.Error(), "revision") {
			t.Fatalf("want revision error, got %v", err)
		}
	}
}

func TestDocsSuggestionRejectsInvalidUTF16Offsets(t *testing.T) {
	t.Parallel()
	svc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("unexpected write with an invalid offset")
		}
		fmt.Fprint(w, docsSuggestionTestTab)
	})
	defer cleanup()
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	for _, tc := range []struct{ index, want string }{{"2", "surrogate pair"}, {"12", "outside the editable range"}} {
		err := runKong(t, &DocsInsertCmd{}, []string{"doc1", "text", "--index=" + tc.index, "--revision=r1", "--suggest", "--tab=Review"}, ctx, &RootFlags{Account: "a@b.com"})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("offset %s: got %v, want %s", tc.index, err, tc.want)
		}
	}
}

func TestDocsSuggestionUTF16TraversalIncludesTablesAndTOC(t *testing.T) {
	t.Parallel()
	paragraph := &docs.StructuralElement{StartIndex: 5, EndIndex: 8, Paragraph: &docs.Paragraph{
		Elements: []*docs.ParagraphElement{{StartIndex: 5, EndIndex: 8, TextRun: &docs.TextRun{Content: "😀\n"}}},
	}}
	for _, element := range []*docs.StructuralElement{
		{StartIndex: 2, EndIndex: 8, TableOfContents: &docs.TableOfContents{Content: []*docs.StructuralElement{paragraph}}},
		{StartIndex: 2, EndIndex: 8, Table: &docs.Table{TableRows: []*docs.TableRow{{TableCells: []*docs.TableCell{{Content: []*docs.StructuralElement{paragraph}}}}}}},
	} {
		if !splitsDocsSurrogate([]*docs.StructuralElement{element}, 6) || splitsDocsSurrogate([]*docs.StructuralElement{element}, 7) {
			t.Fatalf("nested structure failed to recognize UTF-16 boundaries: %#v", element)
		}
	}
}
