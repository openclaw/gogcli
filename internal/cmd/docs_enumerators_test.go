package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
)

func TestDocsEnumerateTables(t *testing.T) {
	scope := &docsEnumeratorScope{content: []*docs.StructuralElement{
		{
			StartIndex: 10,
			EndIndex:   40,
			Table: &docs.Table{TableRows: []*docs.TableRow{
				docsEnumTableRow("Author", "Sebastian Roth"),
				docsEnumTableRow("Reviewer", "Status"),
			}},
		},
	}}

	items := enumerateDocsTables(scope)
	if len(items) != 1 {
		t.Fatalf("expected 1 table, got %d", len(items))
	}
	got := items[0]
	if got.Index != 1 || got.StartIndex != 10 || got.EndIndex != 40 {
		t.Fatalf("unexpected table indices: %#v", got)
	}
	if got.Rows != 2 || got.Cols != 2 {
		t.Fatalf("unexpected dimensions: %#v", got)
	}
	if strings.Join(got.HeaderRow, "|") != "Author|Sebastian Roth" {
		t.Fatalf("unexpected header row: %#v", got.HeaderRow)
	}
}

func TestDocsEnumerateImages(t *testing.T) {
	scope := &docsEnumeratorScope{
		inlineObjects: map[string]docs.InlineObject{
			"img.inline": {InlineObjectProperties: &docs.InlineObjectProperties{
				EmbeddedObject: &docs.EmbeddedObject{
					Description: "Architecture diagram",
					Size: &docs.Size{
						Width:  &docs.Dimension{Magnitude: 1008, Unit: "PT"},
						Height: &docs.Dimension{Magnitude: 500, Unit: "PT"},
					},
				},
			}},
		},
		positionedObjects: map[string]docs.PositionedObject{
			"img.positioned": {PositionedObjectProperties: &docs.PositionedObjectProperties{
				EmbeddedObject: &docs.EmbeddedObject{Title: "Floating logo"},
			}},
		},
		content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{
				{StartIndex: 42, InlineObjectElement: &docs.InlineObjectElement{InlineObjectId: "img.inline"}},
			}}},
		},
	}

	items := enumerateDocsImages(scope)
	if len(items) != 2 {
		t.Fatalf("expected 2 images, got %d", len(items))
	}
	if got := items[0]; got.Index != 1 || got.ObjectID != "img.inline" || got.StartIndex != 42 || got.Alt != "Architecture diagram" {
		t.Fatalf("unexpected inline image: %#v", got)
	}
	if items[0].WidthPt != 1008 || items[0].HeightPt != 500 {
		t.Fatalf("unexpected image size: %#v", items[0])
	}
	if got := items[1]; got.Index != 2 || got.ObjectID != "img.positioned" || !got.Positioned || got.Alt != "Floating logo" {
		t.Fatalf("unexpected positioned image: %#v", got)
	}
}

func TestDocsEnumerateImages_UsesTabObjectMaps(t *testing.T) {
	scope := &docsEnumeratorScope{
		tabID:    "t.second",
		tabTitle: "Second",
		inlineObjects: map[string]docs.InlineObject{
			"tab.inline": {InlineObjectProperties: &docs.InlineObjectProperties{
				EmbeddedObject: &docs.EmbeddedObject{
					Title: "Tab image",
					Size:  &docs.Size{Width: &docs.Dimension{Magnitude: 320, Unit: "PT"}},
				},
			}},
		},
		positionedObjects: map[string]docs.PositionedObject{
			"tab.positioned": {PositionedObjectProperties: &docs.PositionedObjectProperties{
				EmbeddedObject: &docs.EmbeddedObject{Description: "Tab positioned image"},
			}},
		},
		content: []*docs.StructuralElement{
			{Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{
				{StartIndex: 9, InlineObjectElement: &docs.InlineObjectElement{InlineObjectId: "tab.inline"}},
			}}},
		},
	}

	items := enumerateDocsImages(scope)
	if len(items) != 2 {
		t.Fatalf("expected 2 tab images, got %d", len(items))
	}
	if got := items[0]; got.ObjectID != "tab.inline" || got.Alt != "Tab image" || got.WidthPt != 320 || got.TabID != "t.second" {
		t.Fatalf("unexpected tab inline image: %#v", got)
	}
	if got := items[1]; got.ObjectID != "tab.positioned" || got.Alt != "Tab positioned image" || !got.Positioned || got.TabTitle != "Second" {
		t.Fatalf("unexpected tab positioned image: %#v", got)
	}
}

func TestDocsEnumerateHeadingsAndParagraphs(t *testing.T) {
	scope := &docsEnumeratorScope{content: []*docs.StructuralElement{
		docsEnumParagraphElement(5, 18, "HEADING_1", "Intro\n"),
		docsEnumParagraphElement(18, 35, docsNamedStyleNormalText, "Body text\n"),
		docsEnumParagraphElement(35, 50, "HEADING_2", "Details\n"),
	}}

	headings := enumerateDocsHeadings(scope, 2)
	if len(headings) != 1 {
		t.Fatalf("expected 1 h2, got %d", len(headings))
	}
	if got := headings[0]; got.Index != 1 || got.Level != 2 || got.StartIndex != 35 || got.Text != "Details" {
		t.Fatalf("unexpected heading: %#v", got)
	}

	paragraphs := enumerateDocsParagraphs(scope, docsNamedStyleNormalText)
	if len(paragraphs) != 1 {
		t.Fatalf("expected 1 normal paragraph, got %d", len(paragraphs))
	}
	if got := paragraphs[0]; got.Index != 1 || got.Style != docsNamedStyleNormalText || got.Text != "Body text" {
		t.Fatalf("unexpected paragraph: %#v", got)
	}
}

func TestDocsHeadingsList_TabJSON(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	var includeTabs bool
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			http.NotFound(w, r)
			return
		}
		includeTabs = strings.Contains(r.URL.RawQuery, "includeTabsContent=true")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"documentId": "doc1",
			"tabs": []any{
				map[string]any{
					"tabProperties": map[string]any{"tabId": "t.first", "title": "First", "index": 0},
					"documentTab": map[string]any{
						"body": map[string]any{"content": []any{
							docsEnumParagraphMap(1, 10, "HEADING_1", "First heading\n"),
						}},
					},
				},
			},
		})
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	ctx := outfmt.WithMode(newCmdOutputContext(t, &bytes.Buffer{}, &bytes.Buffer{}), outfmt.Mode{JSON: true})
	out := captureStdout(t, func() {
		err := runKong(t, &DocsHeadingsListCmd{}, []string{"doc1", "--tab", "First"}, ctx, &RootFlags{Account: "a@b.com"})
		if err != nil {
			t.Fatalf("headings list: %v", err)
		}
	})
	if !includeTabs {
		t.Fatalf("expected includeTabsContent=true")
	}

	var got []docsHeadingListItem
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse json: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].TabID != "t.first" || got[0].TabTitle != "First" || got[0].Text != "First heading" {
		t.Fatalf("unexpected JSON: %#v", got)
	}
}

func TestDocsParagraphsList_PlainTSV(t *testing.T) {
	origDocs := newDocsService
	t.Cleanup(func() { newDocsService = origDocs })

	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"documentId": "doc1",
			"body": map[string]any{"content": []any{
				docsEnumParagraphMap(1, 15, docsNamedStyleNormalText, "line\tone\n"),
			}},
		})
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }

	var stdout bytes.Buffer
	ctx := outfmt.WithMode(newCmdOutputContext(t, &stdout, &bytes.Buffer{}), outfmt.Mode{Plain: true})
	err := runKong(t, &DocsParagraphsListCmd{}, []string{"doc1"}, ctx, &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("paragraphs list: %v", err)
	}

	want := "paragraph\t1\tNORMAL_TEXT\t1\t15\tfalse\t0\tline one\n"
	if stdout.String() != want {
		t.Fatalf("plain output = %q, want %q", stdout.String(), want)
	}
}

func docsEnumParagraphElement(start, end int64, style, text string) *docs.StructuralElement {
	return &docs.StructuralElement{
		StartIndex: start,
		EndIndex:   end,
		Paragraph: &docs.Paragraph{
			ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: style},
			Elements:       []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: text}}},
		},
	}
}

func docsEnumParagraphMap(start, end int64, style, text string) map[string]any {
	return map[string]any{
		"startIndex": start,
		"endIndex":   end,
		"paragraph": map[string]any{
			"paragraphStyle": map[string]any{"namedStyleType": style},
			"elements": []any{
				map[string]any{"textRun": map[string]any{"content": text}},
			},
		},
	}
}

func docsEnumTableRow(values ...string) *docs.TableRow {
	cells := make([]*docs.TableCell, 0, len(values))
	for _, value := range values {
		cells = append(cells, &docs.TableCell{Content: []*docs.StructuralElement{
			docsEnumParagraphElement(0, 0, docsNamedStyleNormalText, value+"\n"),
		}})
	}
	return &docs.TableRow{TableCells: cells}
}
