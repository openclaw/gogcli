package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestRewriteDocsCellUpdateContentArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "bullet list",
			args: []string{"docs", "cell-update", "doc1", "--content", "- one\n- two", "--row", "1", "--col", "1"},
			want: []string{"docs", "cell-update", "doc1", "--content=- one\n- two", "--row", "1", "--col", "1"},
		},
		{
			name: "aliases and global flag",
			args: []string{"--account", "a@b.com", "doc", "update-cell", "doc1", "--content", "- one"},
			want: []string{"--account", "a@b.com", "doc", "update-cell", "doc1", "--content=- one"},
		},
		{
			name: "existing equals form",
			args: []string{"docs", "cell-update", "doc1", "--content=- one"},
			want: []string{"docs", "cell-update", "doc1", "--content=- one"},
		},
		{
			name: "missing content value",
			args: []string{"docs", "cell-update", "doc1", "--content", "--append"},
			want: []string{"docs", "cell-update", "doc1", "--content", "--append"},
		},
		{
			name: "other command",
			args: []string{"docs", "write", "doc1", "--content", "- one"},
			want: []string{"docs", "write", "doc1", "--content", "- one"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rewriteDocsCellUpdateContentArgs(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("rewriteDocsCellUpdateContentArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestExecuteDocsCellUpdateLeadingDashContentDryRun(t *testing.T) {
	out := captureStdout(t, func() {
		err := Execute([]string{
			"--json", "--dry-run", "--no-input",
			"docs", "cell-update", "doc1",
			"--row", "1", "--col", "1",
			"--content", "- one\n- two",
		})
		if err != nil && ExitCode(err) != 0 {
			t.Fatalf("Execute: %v", err)
		}
	})
	if !strings.Contains(out, `"op": "docs.cell-update"`) {
		t.Fatalf("unexpected dry-run output: %s", out)
	}
}

func TestDocsCellUpdate_ReplacesTargetCellOnly(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(cellUpdateTestDoc())
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

	cmd := &DocsCellUpdateCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--table-index", "1", "--row", "1", "--col", "2", "--content", "New", "--format", "plain"}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("docs cell-update: %v", err)
	}
	if got.WriteControl == nil || got.WriteControl.RequiredRevisionId != "rev1" {
		t.Fatalf("missing write control: %#v", got.WriteControl)
	}
	if len(got.Requests) != 2 {
		t.Fatalf("expected delete+insert, got %d requests", len(got.Requests))
	}
	del := got.Requests[0].DeleteContentRange
	if del == nil || del.Range.StartIndex != 10 || del.Range.EndIndex != 15 {
		t.Fatalf("unexpected delete range: %#v", del)
	}
	ins := got.Requests[1].InsertText
	if ins == nil || ins.Location.Index != 10 || ins.Text != "New" {
		t.Fatalf("unexpected insert: %#v", ins)
	}
}

func TestDocsCellUpdate_ReplacesWithNativeMarkdownList(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(cellUpdateTestDoc())
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

	cmd := &DocsCellUpdateCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--row", "1", "--col", "2", "--content=- Alpha\n- Beta"}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("docs cell-update list: %v", err)
	}
	if len(got.Requests) != 3 {
		t.Fatalf("expected delete+insert+bullets, got %d requests", len(got.Requests))
	}
	ins := got.Requests[1].InsertText
	if ins == nil || ins.Location.Index != 10 || ins.Text != "Alpha\nBeta" {
		t.Fatalf("unexpected list insert: %#v", ins)
	}
	bullets := got.Requests[2].CreateParagraphBullets
	if bullets == nil || bullets.Range.StartIndex != 10 || bullets.Range.EndIndex != 20 || bullets.BulletPreset != bulletPresetDisc {
		t.Fatalf("unexpected bullet request: %#v", bullets)
	}
}

func TestDocsCellUpdate_ReplacesWithNestedNativeMarkdownList(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(cellUpdateTestDoc())
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

	cmd := &DocsCellUpdateCmd{}
	content := "- a\n  - a1\n  - a2\n- b"
	if err := runKong(t, cmd, []string{"doc1", "--row", "1", "--col", "2", "--content=" + content}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("docs cell-update nested list: %v", err)
	}
	if len(got.Requests) != 3 {
		t.Fatalf("expected delete+insert+bullets, got %d requests", len(got.Requests))
	}
	ins := got.Requests[1].InsertText
	if ins == nil || ins.Location.Index != 10 || ins.Text != "a\n\ta1\n\ta2\nb" {
		t.Fatalf("unexpected nested-list insert: %#v", ins)
	}
	bullets := got.Requests[2].CreateParagraphBullets
	if bullets == nil || bullets.Range.StartIndex != 10 || bullets.Range.EndIndex != 21 || bullets.BulletPreset != bulletPresetDisc {
		t.Fatalf("unexpected nested bullet request: %#v", bullets)
	}
}

func TestDocsCellUpdate_ReplacesPlainCellWithInlineCodeStyle(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(cellUpdateTestDoc())
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

	cmd := &DocsCellUpdateCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--row", "1", "--col", "2", "--content", "`doThing()`"}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("docs cell-update inline code: %v", err)
	}
	if len(got.Requests) != 3 {
		t.Fatalf("expected delete+insert+code style, got %d requests", len(got.Requests))
	}
	ins := got.Requests[1].InsertText
	if ins == nil || ins.Location.Index != 10 || ins.Text != "doThing()" {
		t.Fatalf("unexpected inline-code insert: %#v", ins)
	}
	style := got.Requests[2].UpdateTextStyle
	if style == nil || style.Range == nil || style.TextStyle == nil || style.TextStyle.WeightedFontFamily == nil {
		t.Fatalf("missing inline-code style request: %#v", got.Requests[2])
	}
	if style.Range.StartIndex != 10 || style.Range.EndIndex != 19 {
		t.Fatalf("inline-code style range = %#v, want [10,19)", style.Range)
	}
	font := style.TextStyle.WeightedFontFamily
	if font.FontFamily != "Courier New" || font.Weight != 400 || style.Fields != "weightedFontFamily" {
		t.Fatalf("inline-code style = %#v fields=%q", font, style.Fields)
	}
}

func TestDocsCellUpdate_AppendMarkdownWithTab(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	var includeTabs bool
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			includeTabs = r.URL.Query().Get("includeTabsContent") == "true"
			_ = json.NewEncoder(w).Encode(cellUpdateTabsTestDoc())
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

	cmd := &DocsCellUpdateCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--tab", "Second", "--row", "1", "--col", "1", "--content", " **bold**", "--append"}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("docs cell-update append: %v", err)
	}
	if !includeTabs {
		t.Fatalf("expected tab-aware GET")
	}
	if len(got.Requests) < 2 || got.Requests[0].DeleteContentRange != nil {
		t.Fatalf("append should not delete, requests=%#v", got.Requests)
	}
	ins := got.Requests[0].InsertText
	if ins == nil || ins.Location.Index != 8 || ins.Location.TabId != "t.second" || ins.Text != " bold" {
		t.Fatalf("unexpected append insert: %#v", ins)
	}
	style := got.Requests[1].UpdateTextStyle
	if style == nil || style.Range.TabId != "t.second" || !style.TextStyle.Bold {
		t.Fatalf("missing bold style on tab: %#v", style)
	}
}

func TestDocsCellUpdate_AppendBlockMarkdownStartsNewParagraph(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(cellUpdateTabsTestDoc())
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

	cmd := &DocsCellUpdateCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--tab", "Second", "--row", "1", "--col", "1", "--content", "# Ready", "--append"}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("docs cell-update append heading: %v", err)
	}
	if len(got.Requests) < 2 || got.Requests[0].DeleteContentRange != nil {
		t.Fatalf("append should not delete, requests=%#v", got.Requests)
	}
	ins := got.Requests[0].InsertText
	if ins == nil || ins.Location.Index != 8 || ins.Text != "\nReady" {
		t.Fatalf("unexpected append insert: %#v", ins)
	}
	para := got.Requests[1].UpdateParagraphStyle
	if para == nil || para.Range.StartIndex != 9 || para.Range.EndIndex != 14 {
		t.Fatalf("heading style should start after inserted boundary: %#v", para)
	}
}

func TestDocsCellUpdate_RejectsMarkdownWithoutEditableText(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var posted bool
	docSvc, cleanup := newDocsServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(cellUpdateTestDoc())
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			posted = true
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	cmd := &DocsCellUpdateCmd{}
	err := runKong(t, cmd, []string{"doc1", "--row", "1", "--col", "2", "--content", "```\n```"}, newDocsCmdContext(t), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "no editable cell text") {
		t.Fatalf("expected no editable text error, got %v", err)
	}
	if posted {
		t.Fatal("unexpected batch update for ineffective markdown")
	}
}

func TestBuildMarkdownCellContent_PreservesHorizontalRuleSemantics(t *testing.T) {
	_, text, inserted, err := buildMarkdownCellContent("---", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("-", 40)
	if text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
	if inserted != utf16Len(want) {
		t.Fatalf("inserted = %d, want %d", inserted, utf16Len(want))
	}
}

func cellUpdateTestDoc() *docs.Document {
	return &docs.Document{
		DocumentId: "doc1",
		RevisionId: "rev1",
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{
				StartIndex: 1,
				EndIndex:   20,
				Table: &docs.Table{TableRows: []*docs.TableRow{
					{TableCells: []*docs.TableCell{
						cellUpdateTestCell(5, "Keep\n"),
						cellUpdateTestCell(10, "Old B\n"),
					}},
				}},
			},
		}},
	}
}

func cellUpdateTabsTestDoc() *docs.Document {
	tabDoc := cellUpdateTestDoc()
	tabDoc.Body.Content[0].Table.TableRows[0].TableCells[0] = cellUpdateTestCell(5, "Old\n")
	return &docs.Document{
		DocumentId: "doc1",
		RevisionId: "rev1",
		Tabs: []*docs.Tab{
			{
				TabProperties: &docs.TabProperties{TabId: "t.second", Title: "Second"},
				DocumentTab:   &docs.DocumentTab{Body: tabDoc.Body},
			},
		},
	}
}

func cellUpdateTestCell(start int64, text string) *docs.TableCell {
	end := start + int64(len(text))
	return &docs.TableCell{Content: []*docs.StructuralElement{
		{
			StartIndex: start,
			EndIndex:   end,
			Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{
				{
					StartIndex: start,
					EndIndex:   end,
					TextRun:    &docs.TextRun{Content: text},
				},
			}},
		},
	}}
}
