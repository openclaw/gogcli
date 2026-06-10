package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
)

func TestDocsTablesListNestedTabJSON(t *testing.T) {
	response := map[string]any{
		"documentId": "doc1",
		"revisionId": "rev1",
		"tabs": []any{
			map[string]any{
				"tabProperties": map[string]any{"tabId": "t.main", "title": "Main"},
				"documentTab":   map[string]any{"body": rawBodyWithText("main\n")},
				"childTabs": []any{
					map[string]any{
						"tabProperties": map[string]any{"tabId": "t.data", "title": "Data"},
						"documentTab": map[string]any{
							"body": map[string]any{
								"content": []any{
									map[string]any{
										"startIndex": 1,
										"endIndex":   20,
										"table": map[string]any{
											"rows":    3,
											"columns": 4,
											"tableRows": []any{
												map[string]any{
													"tableCells": []any{
														rawTableCell("Name"),
														rawTableCell("Status"),
													},
												},
												map[string]any{
													"tableCells": []any{
														rawTableCell("One"),
														rawTableCell("Ready"),
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
		},
	}
	srv := newDocsRawTestServerWithRequest(t, 0, response, func(r *http.Request) {
		if got := r.URL.Query().Get("includeTabsContent"); got != "true" {
			t.Fatalf("includeTabsContent = %q, want true", got)
		}
	})
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := outfmt.WithMode(rawTestContext(t), outfmt.Mode{JSON: true})
	out := captureStdout(t, func() {
		cmd := &DocsTablesListCmd{}
		if err := runKong(t, cmd, []string{"doc1", "--tab", "Data"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var got struct {
		DocumentID string              `json:"documentId"`
		TabID      string              `json:"tabId"`
		Tables     []docsTableListItem `json:"tables"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if got.DocumentID != "doc1" || got.TabID != "t.data" || len(got.Tables) != 1 {
		t.Fatalf("unexpected result: %#v", got)
	}
	table := got.Tables[0]
	if table.Index != 1 || table.StartIndex != 1 || table.Rows != 3 || table.Columns != 4 {
		t.Fatalf("unexpected table: %#v", table)
	}
	if strings.Join(table.Header, "|") != "Name|Status" {
		t.Fatalf("header = %#v", table.Header)
	}
}

func TestEnumerateDocsImagesOrderAndMetadata(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{
				StartIndex: 3,
				Paragraph: &docs.Paragraph{
					PositionedObjectIds: []string{"z-last"},
					Elements: []*docs.ParagraphElement{
						{StartIndex: 4, InlineObjectElement: &docs.InlineObjectElement{InlineObjectId: "inline"}},
					},
				},
			},
		}},
		InlineObjects: map[string]docs.InlineObject{
			"inline": {
				InlineObjectProperties: &docs.InlineObjectProperties{
					EmbeddedObject: &docs.EmbeddedObject{
						Title:           "Diagram",
						Description:     "Flow",
						ImageProperties: &docs.ImageProperties{},
						Size: &docs.Size{
							Width:  &docs.Dimension{Magnitude: 720, Unit: "PT"},
							Height: &docs.Dimension{Magnitude: 166, Unit: "PT"},
						},
					},
				},
			},
		},
		PositionedObjects: map[string]docs.PositionedObject{
			"a-first": {
				PositionedObjectProperties: &docs.PositionedObjectProperties{
					EmbeddedObject: &docs.EmbeddedObject{
						Description:     "Floating",
						ImageProperties: &docs.ImageProperties{},
					},
				},
			},
			"z-last": {
				PositionedObjectProperties: &docs.PositionedObjectProperties{
					EmbeddedObject: &docs.EmbeddedObject{
						Description:     "Anchored",
						ImageProperties: &docs.ImageProperties{},
					},
				},
			},
		},
	}

	items := enumerateDocsImages(doc)
	if len(items) != 3 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].ObjectID != "z-last" || items[0].StartIndex != 3 || !items[0].Positioned {
		t.Fatalf("anchored positioned image = %#v", items[0])
	}
	if items[1].ObjectID != "inline" || items[1].StartIndex != 4 ||
		items[1].Alt != "Diagram Flow" || items[1].Width != 720 || items[1].Height != 166 ||
		items[1].SizeUnit != "PT" {
		t.Fatalf("inline image = %#v", items[1])
	}
	if items[2].ObjectID != "a-first" {
		t.Fatalf("unanchored positioned image = %#v", items[2])
	}
}

func TestDocsHeadingsAndParagraphsFilters(t *testing.T) {
	response := map[string]any{
		"documentId": "doc1",
		"body": map[string]any{"content": []any{
			rawStyledParagraph(1, 10, "HEADING_1", "Title\n"),
			rawStyledParagraph(10, 20, "HEADING_2", "Section\n"),
			rawStyledParagraph(20, 30, "NORMAL_TEXT", "Body\n"),
		}},
	}
	srv := newDocsRawTestServer(t, 0, response)
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := outfmt.WithMode(rawTestContext(t), outfmt.Mode{JSON: true})
	headingsOut := captureStdout(t, func() {
		if err := runKong(t, &DocsHeadingsListCmd{}, []string{"doc1", "--level", "2"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("headings run: %v", err)
		}
	})
	if !strings.Contains(headingsOut, `"index": 2`) ||
		!strings.Contains(headingsOut, `"text": "Section"`) ||
		strings.Contains(headingsOut, `"text": "Title"`) {
		t.Fatalf("headings output: %s", headingsOut)
	}

	paragraphsOut := captureStdout(t, func() {
		if err := runKong(t, &DocsParagraphsListCmd{}, []string{"doc1", "--style", "normal_text"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("paragraphs run: %v", err)
		}
	})
	if !strings.Contains(paragraphsOut, `"index": 3`) ||
		!strings.Contains(paragraphsOut, `"text": "Body"`) ||
		strings.Contains(paragraphsOut, `"text": "Section"`) {
		t.Fatalf("paragraphs output: %s", paragraphsOut)
	}
}

func TestDocsParagraphsPlainHasNoHeader(t *testing.T) {
	response := map[string]any{
		"documentId": "doc1",
		"body": map[string]any{"content": []any{
			rawStyledParagraph(1, 18, "NORMAL_TEXT", "Hello\tWorld\nNext\n"),
		}},
	}
	srv := newDocsRawTestServer(t, 0, response)
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := outfmt.WithMode(rawTestContext(t), outfmt.Mode{Plain: true})
	out := captureStdout(t, func() {
		if err := runKong(t, &DocsParagraphsListCmd{}, []string{"doc1"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	if strings.Contains(out, "START") || out != `1	1	18	NORMAL_TEXT	Hello\tWorld\nNext
` {
		t.Fatalf("plain output = %q", out)
	}
}

func TestEnumerateDocsParagraphsRecursesIntoTablesAndTOC(t *testing.T) {
	doc := &docs.Document{Body: &docs.Body{Content: []*docs.StructuralElement{
		{
			StartIndex: 1,
			Table: &docs.Table{TableRows: []*docs.TableRow{
				{TableCells: []*docs.TableCell{
					{Content: []*docs.StructuralElement{
						{
							StartIndex: 3,
							EndIndex:   10,
							Paragraph: &docs.Paragraph{
								ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_3"},
								Elements:       []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: "Nested\n"}}},
							},
						},
					}},
				}},
			}},
		},
		{
			TableOfContents: &docs.TableOfContents{Content: []*docs.StructuralElement{
				{
					StartIndex: 11,
					EndIndex:   17,
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: "TOC\n"}}},
					},
				},
			}},
		},
	}}}

	got := enumerateDocsParagraphs(doc)
	if len(got) != 2 || got[0].Style != "HEADING_3" || got[0].Text != "Nested" ||
		got[1].Style != "NORMAL_TEXT" || got[1].Text != "TOC" {
		t.Fatalf("paragraphs = %#v", got)
	}
}

func rawTableCell(text string) map[string]any {
	return map[string]any{
		"content": []any{rawStyledParagraph(1, int64(len(text)+2), "NORMAL_TEXT", text+"\n")},
	}
}

func rawStyledParagraph(start, end int64, style, text string) map[string]any {
	return map[string]any{
		"startIndex": start,
		"endIndex":   end,
		"paragraph": map[string]any{
			"paragraphStyle": map[string]any{"namedStyleType": style},
			"elements": []any{
				map[string]any{
					"startIndex": start,
					"endIndex":   end,
					"textRun":    map[string]any{"content": text},
				},
			},
		},
	}
}
