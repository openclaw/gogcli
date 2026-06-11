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
		strings.Contains(headingsOut, `"isEmpty"`) ||
		strings.Contains(headingsOut, `"runs"`) ||
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

func TestDocsParagraphsJSONIncludesRunsAndEmptiness(t *testing.T) {
	response := map[string]any{
		"documentId": "doc1",
		"body": map[string]any{"content": []any{
			map[string]any{
				"startIndex": 1,
				"endIndex":   15,
				"paragraph": map[string]any{
					"paragraphStyle": map[string]any{"namedStyleType": "NORMAL_TEXT"},
					"elements": []any{
						map[string]any{
							"startIndex": 1,
							"endIndex":   7,
							"textRun": map[string]any{
								"content":   "Alpha ",
								"textStyle": map[string]any{"bold": true},
							},
						},
						map[string]any{
							"startIndex": 7,
							"endIndex":   11,
							"textRun": map[string]any{
								"content": "link",
								"textStyle": map[string]any{
									"italic":        true,
									"underline":     true,
									"strikethrough": true,
									"link":          map[string]any{"url": "https://example.com"},
								},
							},
						},
						map[string]any{
							"startIndex": 11,
							"endIndex":   14,
							"textRun":    map[string]any{"content": "🐢\n"},
						},
					},
				},
			},
			map[string]any{
				"startIndex": 15,
				"endIndex":   16,
				"paragraph": map[string]any{
					"elements": []any{
						map[string]any{
							"startIndex": 15,
							"endIndex":   16,
							"textRun":    map[string]any{"content": "\n"},
						},
					},
				},
			},
			map[string]any{
				"startIndex": 16,
				"endIndex":   17,
				"paragraph": map[string]any{
					"elements": []any{
						map[string]any{
							"startIndex":          16,
							"endIndex":            17,
							"inlineObjectElement": map[string]any{"inlineObjectId": "image1"},
						},
					},
				},
			},
		}},
	}
	srv := newDocsRawTestServer(t, 0, response)
	defer srv.Close()
	installMockDocsService(t, srv)

	ctx := outfmt.WithMode(rawTestContext(t), outfmt.Mode{JSON: true})
	out := captureStdout(t, func() {
		if err := runKong(t, &DocsParagraphsListCmd{}, []string{"doc1"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var got struct {
		Paragraphs []docsParagraphInspectItem `json:"paragraphs"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if len(got.Paragraphs) != 3 {
		t.Fatalf("paragraphs = %#v", got.Paragraphs)
	}

	first := got.Paragraphs[0]
	if first.IsEmpty || first.Text != "Alpha link🐢" || len(first.Runs) != 3 {
		t.Fatalf("first paragraph = %#v", first)
	}
	if !first.Runs[0].Bold || first.Runs[0].Italic || first.Runs[0].Link != nil {
		t.Fatalf("bold run = %#v", first.Runs[0])
	}
	linkRun := first.Runs[1]
	if linkRun.Bold || !linkRun.Italic || !linkRun.Underline || !linkRun.Strikethrough ||
		linkRun.Link == nil || linkRun.Link.URL != "https://example.com" {
		t.Fatalf("link run = %#v", linkRun)
	}
	if first.Runs[2].StartIndex != 11 || first.Runs[2].EndIndex != 14 || first.Runs[2].Text != "🐢\n" {
		t.Fatalf("UTF-16 run = %#v", first.Runs[2])
	}
	if !got.Paragraphs[1].IsEmpty || len(got.Paragraphs[1].Runs) != 1 {
		t.Fatalf("blank paragraph = %#v", got.Paragraphs[1])
	}
	if got.Paragraphs[2].IsEmpty || len(got.Paragraphs[2].Runs) != 0 {
		t.Fatalf("inline-object paragraph = %#v", got.Paragraphs[2])
	}
}

func TestDocsParagraphRunLinkFrom(t *testing.T) {
	tests := []struct {
		name string
		link *docs.Link
		want docsParagraphRunLink
	}{
		{
			name: "legacy bookmark",
			link: &docs.Link{BookmarkId: "bookmark1"},
			want: docsParagraphRunLink{BookmarkID: "bookmark1"},
		},
		{
			name: "tab-aware bookmark",
			link: &docs.Link{Bookmark: &docs.BookmarkLink{Id: "bookmark2", TabId: "tab1"}},
			want: docsParagraphRunLink{BookmarkID: "bookmark2", TabID: "tab1"},
		},
		{
			name: "tab-aware heading",
			link: &docs.Link{Heading: &docs.HeadingLink{Id: "heading1", TabId: "tab2"}},
			want: docsParagraphRunLink{HeadingID: "heading1", TabID: "tab2"},
		},
		{
			name: "tab",
			link: &docs.Link{TabId: "tab3"},
			want: docsParagraphRunLink{TabID: "tab3"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := docsParagraphRunLinkFrom(test.link)
			if got == nil || *got != test.want {
				t.Fatalf("link = %#v, want %#v", got, test.want)
			}
		})
	}
	if got := docsParagraphRunLinkFrom(&docs.Link{}); got != nil {
		t.Fatalf("empty link = %#v, want nil", got)
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
