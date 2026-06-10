package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type docsNamedRangeRecorder struct {
	batches     []docs.BatchUpdateDocumentRequest
	rawBatches  [][]byte
	includeTabs []string
}

func setupDocsNamedRangeTestService(t *testing.T, doc *docs.Document, rec *docsNamedRangeRecorder) {
	t.Helper()

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			rec.includeTabs = append(rec.includeTabs, r.URL.Query().Get("includeTabsContent"))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(doc)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":batchUpdate"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read batch body: %v", err)
			}
			var req docs.BatchUpdateDocumentRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("decode batch: %v", err)
			}
			rec.rawBatches = append(rec.rawBatches, body)
			rec.batches = append(rec.batches, req)
			response := map[string]any{"documentId": "doc1"}
			if len(req.Requests) == 1 && req.Requests[0].CreateNamedRange != nil {
				response["replies"] = []any{map[string]any{
					"createNamedRange": map[string]any{"namedRangeId": "nr-new"},
				}}
			}
			if len(req.Requests) == 1 && req.Requests[0].ReplaceNamedRangeContent != nil {
				updateDocsNamedRangeForTest(doc, req.Requests[0].ReplaceNamedRangeContent)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(cleanup)
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
}

func TestDocsNamedRangesListTabJSONAndPlain(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	doc := docsNamedRangeTabbedDocument()
	rec := &docsNamedRangeRecorder{}
	setupDocsNamedRangeTestService(t, doc, rec)

	var runErr error
	out := captureStdout(t, func() {
		runErr = runKong(t, &DocsNamedRangesListCmd{}, []string{"doc1", "--tab", "Work"}, newDocsJSONContext(t), &RootFlags{Account: "a@b.com"})
	})
	if runErr != nil {
		t.Fatalf("list: %v", runErr)
	}
	if len(rec.includeTabs) != 1 || rec.includeTabs[0] != "true" {
		t.Fatalf("includeTabsContent = %#v, want true", rec.includeTabs)
	}
	var payload struct {
		DocumentID  string               `json:"documentId"`
		TabID       string               `json:"tabId"`
		NamedRanges []docsNamedRangeItem `json:"namedRanges"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if payload.DocumentID != "doc1" || payload.TabID != "t.work" || len(payload.NamedRanges) != 2 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.NamedRanges[0].Name != "alpha" || payload.NamedRanges[1].Name != "stable" {
		t.Fatalf("order = %#v", payload.NamedRanges)
	}
	if got := payload.NamedRanges[0].Ranges[0].SegmentID; got != "header-1" {
		t.Fatalf("alpha segmentId = %q, want header-1", got)
	}
	if got := payload.NamedRanges[1].Ranges[0]; got.StartIndex != 7 || got.EndIndex != 13 || got.TabID != "t.work" {
		t.Fatalf("stable range = %#v", got)
	}

	var plainErr error
	plain := captureStdout(t, func() {
		ctx := outfmt.WithMode(newDocsCmdContext(t), outfmt.Mode{Plain: true})
		plainErr = runKong(t, &DocsNamedRangesListCmd{}, []string{"doc1", "--tab", "Work", "--name", "stable"}, ctx, &RootFlags{Account: "a@b.com"})
	})
	if plainErr != nil {
		t.Fatalf("plain list: %v", plainErr)
	}
	if got, want := plain, "stable\tnr-stable\t7\t13\tt.work\t\n"; got != want {
		t.Fatalf("plain output = %q, want %q", got, want)
	}
}

func TestDocsNamedRangesCreateAtUsesUTF16TabAndRevision(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	doc := docsNamedRangeTabbedDocument()
	rec := &docsNamedRangeRecorder{}
	setupDocsNamedRangeTestService(t, doc, rec)

	var runErr error
	out := captureStdout(t, func() {
		runErr = runKong(t, &DocsNamedRangesCreateCmd{}, []string{
			"doc1", "--name", "new-anchor", "--at", "😀 anchor", "--tab", "Work",
		}, newDocsJSONContext(t), &RootFlags{Account: "a@b.com"})
	})
	if runErr != nil {
		t.Fatalf("create: %v", runErr)
	}
	if len(rec.batches) != 1 {
		t.Fatalf("batches = %d, want 1", len(rec.batches))
	}
	batch := rec.batches[0]
	if batch.WriteControl == nil || batch.WriteControl.RequiredRevisionId != "rev1" {
		t.Fatalf("write control = %#v", batch.WriteControl)
	}
	req := batch.Requests[0].CreateNamedRange
	if req == nil || req.Name != "new-anchor" {
		t.Fatalf("create request = %#v", req)
	}
	if got := req.Range; got.StartIndex != 14 || got.EndIndex != 23 || got.TabId != "t.work" {
		t.Fatalf("range = %#v, want 14..23 on t.work", got)
	}
	var payload struct {
		NamedRange docsNamedRangeItem `json:"namedRange"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if payload.NamedRange.NamedRangeID != "nr-new" {
		t.Fatalf("output = %s", out)
	}
}

func TestDocsNamedRangesCreateIndexAndRejectDuplicate(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	doc := docsFindRangeDoc(docsFindRangeParagraph(1, "plain body\n"))
	doc.DocumentId = "doc1"
	doc.RevisionId = "rev-index"
	doc.NamedRanges = map[string]docs.NamedRanges{
		"stable": {
			Name: "stable",
			NamedRanges: []*docs.NamedRange{{
				Name:         "stable",
				NamedRangeId: "nr-existing",
				Ranges:       []*docs.Range{{StartIndex: 1, EndIndex: 6}},
			}},
		},
	}
	rec := &docsNamedRangeRecorder{}
	setupDocsNamedRangeTestService(t, doc, rec)

	if err := runKong(t, &DocsNamedRangesCreateCmd{}, []string{
		"doc1", "--name", "index-anchor", "--start", "2", "--end", "5",
	}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("index create: %v", err)
	}
	got := rec.batches[0].Requests[0].CreateNamedRange.Range
	if got.StartIndex != 2 || got.EndIndex != 5 || got.TabId != "" {
		t.Fatalf("range = %#v", got)
	}

	err := runKong(t, &DocsNamedRangesCreateCmd{}, []string{
		"doc1", "--name", "stable", "--start", "2", "--end", "5",
	}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate error = %v", err)
	}
	if len(rec.batches) != 1 {
		t.Fatalf("duplicate sent mutation; batches = %d", len(rec.batches))
	}
}

func TestDocsNamedRangesDeleteAndReplaceByExactID(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	doc := docsFindRangeDoc(docsFindRangeParagraph(1, "stable\n"))
	doc.DocumentId = "doc1"
	doc.RevisionId = "rev-mutate"
	doc.NamedRanges = map[string]docs.NamedRanges{
		"stable": {
			Name: "stable",
			NamedRanges: []*docs.NamedRange{{
				Name:         "stable",
				NamedRangeId: "nr-stable",
				Ranges:       []*docs.Range{{StartIndex: 1, EndIndex: 7}},
			}},
		},
	}
	rec := &docsNamedRangeRecorder{}
	setupDocsNamedRangeTestService(t, doc, rec)

	if err := runKong(t, &DocsNamedRangesDeleteCmd{}, []string{"doc1", "stable"}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	deleteBatch := rec.batches[0]
	if deleteBatch.WriteControl == nil || deleteBatch.WriteControl.RequiredRevisionId != "rev-mutate" {
		t.Fatalf("delete write control = %#v", deleteBatch.WriteControl)
	}
	if got := deleteBatch.Requests[0].DeleteNamedRange; got == nil || got.NamedRangeId != "nr-stable" || got.Name != "" {
		t.Fatalf("delete request = %#v", got)
	}

	var replaceErr error
	replaceOut := captureStdout(t, func() {
		replaceErr = runKong(t, &DocsNamedRangesReplaceCmd{}, []string{"doc1", "nr-stable", "--text", "replacement"}, newDocsJSONContext(t), &RootFlags{Account: "a@b.com"})
	})
	if replaceErr != nil {
		t.Fatalf("replace: %v", replaceErr)
	}
	replaceBatch := rec.batches[1]
	if got := replaceBatch.Requests[0].ReplaceNamedRangeContent; got == nil || got.NamedRangeId != "nr-stable" {
		t.Fatalf("replace request = %#v", got)
	}
	var replacePayload struct {
		NamedRange docsNamedRangeItem `json:"namedRange"`
	}
	if err := json.Unmarshal([]byte(replaceOut), &replacePayload); err != nil {
		t.Fatalf("replace json: %v\n%s", err, replaceOut)
	}
	if got := replacePayload.NamedRange.Ranges[0].EndIndex; got != 12 {
		t.Fatalf("post-replace endIndex = %d, want 12", got)
	}

	if err := runKong(t, &DocsNamedRangesReplaceCmd{}, []string{"doc1", "nr-stable", "--text="}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("replace empty: %v", err)
	}
	if !bytes.Contains(rec.rawBatches[2], []byte(`"text":""`)) {
		t.Fatalf("empty text omitted from wire body: %s", rec.rawBatches[2])
	}
}

func TestDocsNamedRangeTSVPreservesUnicodeAndLiteralCharacters(t *testing.T) {
	input := "Résumé \"quoted\" C:\\path\tline\nnext"
	got := docsNamedRangeTSV(input)
	want := `Résumé "quoted" C:\path\tline\nnext`
	if got != want {
		t.Fatalf("TSV value = %q, want %q", got, want)
	}
}

func TestWriteDocsNamedRangeTextResultIsStableTSV(t *testing.T) {
	ctx, out := newDocsCmdOutputContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{Plain: true})
	writeDocsNamedRangeTextResult(ui.FromContext(ctx), "doc1", docsNamedRangeItem{
		Name:         "Résumé\tline\nnext",
		NamedRangeID: "nr1",
		Ranges: []docsNamedRangeSpan{{
			StartIndex: 2,
			EndIndex:   5,
			TabID:      "tab\t1",
			SegmentID:  "header\n1",
		}},
	})
	want := "" +
		"documentId\tdoc1\n" +
		"name\tRésumé\\tline\\nnext\n" +
		"namedRangeId\tnr1\n" +
		"rangeCount\t1\n" +
		"range1StartIndex\t2\n" +
		"range1EndIndex\t5\n" +
		"range1TabId\ttab\\t1\n" +
		"range1SegmentId\theader\\n1\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestResolveDocsNamedRangeRejectsAmbiguousName(t *testing.T) {
	items := []docsNamedRangeItem{
		{Name: "same", NamedRangeID: "nr2"},
		{Name: "same", NamedRangeID: "nr1"},
	}
	if got, found, err := resolveDocsNamedRange("nr2", items); err != nil || !found || got.NamedRangeID != "nr2" {
		t.Fatalf("id resolve = %#v, %t, %v", got, found, err)
	}
	_, found, err := resolveDocsNamedRange("same", items)
	if err == nil || found || !strings.Contains(err.Error(), "nr1, nr2") {
		t.Fatalf("ambiguous resolve = found %t, err %v", found, err)
	}
}

func docsNamedRangeTabbedDocument() *docs.Document {
	return &docs.Document{
		DocumentId: "doc1",
		RevisionId: "rev1",
		Tabs: []*docs.Tab{{
			TabProperties: &docs.TabProperties{TabId: "t.work", Title: "Work"},
			DocumentTab: &docs.DocumentTab{
				Body: docsFindRangeDoc(docsFindRangeParagraph(1, "Hello stable 😀 anchor\n")).Body,
				NamedRanges: map[string]docs.NamedRanges{
					"stable": {
						Name: "stable",
						NamedRanges: []*docs.NamedRange{{
							Name:         "stable",
							NamedRangeId: "nr-stable",
							Ranges:       []*docs.Range{{StartIndex: 7, EndIndex: 13, TabId: "t.work"}},
						}},
					},
					"alpha": {
						Name: "alpha",
						NamedRanges: []*docs.NamedRange{{
							Name:         "alpha",
							NamedRangeId: "nr-alpha",
							Ranges:       []*docs.Range{{StartIndex: 1, EndIndex: 6, TabId: "t.work", SegmentId: "header-1"}},
						}},
					},
				},
			},
		}},
	}
}

func updateDocsNamedRangeForTest(doc *docs.Document, req *docs.ReplaceNamedRangeContentRequest) {
	if doc == nil || req == nil {
		return
	}
	updateGroups := func(groups map[string]docs.NamedRanges) {
		for name, group := range groups {
			for _, namedRange := range group.NamedRanges {
				if namedRange == nil || namedRange.NamedRangeId != req.NamedRangeId || len(namedRange.Ranges) == 0 {
					continue
				}
				namedRange.Ranges[0].EndIndex = namedRange.Ranges[0].StartIndex + utf16Len(req.Text)
				namedRange.Ranges = namedRange.Ranges[:1]
				group.NamedRanges = []*docs.NamedRange{namedRange}
				groups[name] = group
			}
		}
	}
	updateGroups(doc.NamedRanges)
	for _, tab := range flattenTabs(doc.Tabs) {
		if tab != nil && tab.DocumentTab != nil {
			updateGroups(tab.DocumentTab.NamedRanges)
		}
	}
}
