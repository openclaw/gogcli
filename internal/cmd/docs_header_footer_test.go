package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestDocsHeaderCreatePopulatesReturnedSegment(t *testing.T) {
	t.Parallel()
	svc, calls, cleanup := newDocsCreateAndPopulateService(t, map[string]any{
		"createHeader": map[string]any{"headerId": "header-1"},
	})
	defer cleanup()

	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &DocsHeaderCreateCmd{}, []string{"doc1", "--text", "Header text"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("create header: %v", err)
	}
	if len(*calls) != 2 || (*calls)[0][0].CreateHeader == nil || (*calls)[0][0].CreateHeader.Type != "DEFAULT" {
		t.Fatalf("create requests = %#v", *calls)
	}
	insert := (*calls)[1][0].InsertText
	if insert == nil || insert.EndOfSegmentLocation == nil || insert.EndOfSegmentLocation.SegmentId != "header-1" || insert.Text != "Header text" {
		t.Fatalf("populate request = %#v", (*calls)[1])
	}
}

func newDocsCreateAndPopulateService(t *testing.T, createReply map[string]any) (*docs.Service, *[][]*docs.Request, func()) {
	t.Helper()

	var calls [][]*docs.Request
	svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			http.NotFound(w, r)
			return
		}
		var body docs.BatchUpdateDocumentRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		calls = append(calls, body.Requests)
		response := map[string]any{"documentId": "doc1"}
		if len(calls) == 1 {
			response["replies"] = []any{createReply}
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	return svc, &calls, cleanup
}

func TestDocsHeaderCreateTargetsFirstSectionOfSelectedTab(t *testing.T) {
	t.Parallel()
	doc := &docs.Document{DocumentId: "doc1", Tabs: []*docs.Tab{
		{
			TabProperties: &docs.TabProperties{TabId: "tab-1", Title: "First"},
			DocumentTab:   &docs.DocumentTab{Body: &docs.Body{}},
		},
		{
			TabProperties: &docs.TabProperties{TabId: "tab-2", Title: "Second"},
			DocumentTab: &docs.DocumentTab{Body: &docs.Body{Content: []*docs.StructuralElement{{
				StartIndex:   10,
				SectionBreak: &docs.SectionBreak{},
			}}}},
		},
	}}
	var wireBody []byte
	svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(doc)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			var err error
			wireBody, err = io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"documentId": "doc1",
				"replies":    []any{map[string]any{"createHeader": map[string]any{"headerId": "header-2"}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &DocsHeaderCreateCmd{}, []string{"doc1", "--tab", "Second"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("create header: %v", err)
	}
	encoded := string(wireBody)
	if !strings.Contains(encoded, `"sectionBreakLocation":{"index":0,"tabId":"tab-2"}`) {
		t.Fatalf("selected tab first-section location missing from wire request: %s", encoded)
	}
}

func TestDocsSegmentTextCommandsPropagateSegmentID(t *testing.T) {
	t.Parallel()
	segmentDoc := &docs.Document{
		DocumentId: "doc1",
		RevisionId: "rev1",
		Headers: map[string]docs.Header{
			"header-1": {
				HeaderId: "header-1",
				Content: []*docs.StructuralElement{{
					StartIndex: 0,
					EndIndex:   7,
					Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{
						StartIndex: 0, EndIndex: 7, TextRun: &docs.TextRun{Content: "Header\n"},
					}}},
				}},
			},
		},
	}
	var batches [][]*docs.Request
	svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(segmentDoc)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			var body docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			batches = append(batches, body.Requests)
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()
	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	flags := &RootFlags{Account: "a@b.com"}

	commands := []struct {
		cmd  any
		args []string
	}{
		{&DocsInsertCmd{}, []string{"doc1", "X", "--segment", "header-1", "--index", "0"}},
		{&DocsUpdateCmd{}, []string{"doc1", "--text", "Y", "--segment", "header-1", "--replace-range", "0:1"}},
		{&DocsDeleteCmd{}, []string{"doc1", "--segment", "header-1", "--start", "0", "--end", "1"}},
		{&DocsFormatCmd{}, []string{"doc1", "--segment", "header-1", "--match", "Header", "--bold"}},
	}
	for _, command := range commands {
		if err := runKong(t, command.cmd, command.args, ctx, flags); err != nil {
			t.Fatalf("run %T: %v", command.cmd, err)
		}
	}
	if len(batches) != len(commands) {
		t.Fatalf("batch count = %d, want %d", len(batches), len(commands))
	}
	for batchIndex, requests := range batches {
		found := false
		for _, request := range requests {
			if request.InsertText != nil && request.InsertText.Location != nil && request.InsertText.Location.SegmentId == "header-1" {
				found = true
			}
			for _, targetRange := range docsRequestRanges(request) {
				if targetRange.SegmentId == "header-1" {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("batch %d lost segment target: %#v", batchIndex, requests)
		}
	}
}

func TestDocsHeaderListAndDelete(t *testing.T) {
	t.Parallel()
	doc := &docs.Document{DocumentId: "doc1", Headers: map[string]docs.Header{
		"header-1": {HeaderId: "header-1", Content: []*docs.StructuralElement{{EndIndex: 4}}},
	}}
	var deleted *docs.DeleteHeaderRequest
	svc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(doc)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			var body docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			deleted = body.Requests[0].DeleteHeader
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()

	var output bytes.Buffer
	jsonCtx := withDocsTestService(newCmdRuntimeJSONOutputContext(t, &output, io.Discard), svc)
	if err := runKong(t, &DocsHeaderListCmd{}, []string{"doc1"}, jsonCtx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("list headers: %v", err)
	}
	var listed struct {
		Headers []docsSegmentListItem `json:"headers"`
	}
	if err := json.Unmarshal(output.Bytes(), &listed); err != nil {
		t.Fatalf("decode list output: %v", err)
	}
	if len(listed.Headers) != 1 || listed.Headers[0].ID != "header-1" {
		t.Fatalf("unexpected list output: %s", output.String())
	}

	ctx := withDocsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	if err := runKong(t, &DocsHeaderDeleteCmd{}, []string{"doc1", "header-1"}, ctx, &RootFlags{Account: "a@b.com", Force: true}); err != nil {
		t.Fatalf("delete header: %v", err)
	}
	if deleted == nil || deleted.HeaderId != "header-1" {
		t.Fatalf("delete request = %#v", deleted)
	}
}
