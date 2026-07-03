package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestEnumerateDocsSuggestions(t *testing.T) {
	t.Parallel()

	tableCellContent := suggestionContent(10, 14, "cell", "table-run")
	tocContent := suggestionContent(20, 23, "toc", "toc-run")
	doc := &docs.Document{
		Body: &docs.Body{Content: []*docs.StructuralElement{
			{
				Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{
					{StartIndex: 1, EndIndex: 4, TextRun: &docs.TextRun{
						Content: "hel", SuggestedInsertionIds: []string{"nested", "insert", "insert"},
					}},
					{StartIndex: 4, EndIndex: 6, TextRun: &docs.TextRun{
						Content: "lo", SuggestedInsertionIds: []string{"insert"},
					}},
					{StartIndex: 6, EndIndex: 7, TextRun: &docs.TextRun{
						Content: "!", SuggestedDeletionIds: []string{"delete"},
					}},
					{StartIndex: 7, EndIndex: 8, TextRun: &docs.TextRun{
						Content: "x", SuggestedTextStyleChanges: map[string]docs.SuggestedTextStyle{"style": {}},
					}},
				}},
			},
			{
				Table: &docs.Table{SuggestedInsertionIds: []string{"table-structure"}, TableRows: []*docs.TableRow{{
					SuggestedDeletionIds: []string{"row-structure"},
					TableCells: []*docs.TableCell{{
						Content:               tableCellContent,
						SuggestedInsertionIds: []string{"cell-structure", "table-structure"},
					}},
				}}},
			},
			{
				TableOfContents: &docs.TableOfContents{
					Content:              tocContent,
					SuggestedDeletionIds: []string{"toc-structure"},
				},
			},
		}},
		Headers: map[string]docs.Header{
			"z": {Content: suggestionContent(3, 5, "zz", "header-z")},
			"a": {Content: suggestionContent(1, 3, "aa", "header-a")},
		},
	}

	got := enumerateDocsSuggestions(doc)
	want := []docsSuggestionListItem{
		{SuggestionID: "insert", Kind: "insertion", Segment: "body", StartIndex: 1, EndIndex: 6, Text: "hello"},
		{SuggestionID: "nested", Kind: "insertion", Segment: "body", StartIndex: 1, EndIndex: 4, Text: "hel"},
		{SuggestionID: "delete", Kind: "deletion", Segment: "body", StartIndex: 6, EndIndex: 7, Text: "!"},
		{SuggestionID: "cell-structure", Kind: "insertion", Segment: "body", StartIndex: 10, EndIndex: 14, Text: "cell"},
		{SuggestionID: "table-run", Kind: "insertion", Segment: "body", StartIndex: 10, EndIndex: 14, Text: "cell"},
		{SuggestionID: "table-structure", Kind: "insertion", Segment: "body", StartIndex: 10, EndIndex: 14, Text: "cell"},
		{SuggestionID: "row-structure", Kind: "deletion", Segment: "body", StartIndex: 10, EndIndex: 14, Text: "cell"},
		{SuggestionID: "toc-run", Kind: "insertion", Segment: "body", StartIndex: 20, EndIndex: 23, Text: "toc"},
		{SuggestionID: "toc-structure", Kind: "deletion", Segment: "body", StartIndex: 20, EndIndex: 23, Text: "toc"},
		{SuggestionID: "header-a", Kind: "insertion", Segment: "header:a", StartIndex: 1, EndIndex: 3, Text: "aa"},
		{SuggestionID: "header-z", Kind: "insertion", Segment: "header:z", StartIndex: 3, EndIndex: 5, Text: "zz"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("suggestions mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestEnumerateDocsSuggestions_NonTextParagraphElements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		element *docs.ParagraphElement
	}{
		{
			name: "auto text",
			element: &docs.ParagraphElement{AutoText: &docs.AutoText{
				SuggestedInsertionIds: []string{"insert"}, SuggestedDeletionIds: []string{"delete"},
			}},
		},
		{
			name: "column break",
			element: &docs.ParagraphElement{ColumnBreak: &docs.ColumnBreak{
				SuggestedInsertionIds: []string{"insert"}, SuggestedDeletionIds: []string{"delete"},
			}},
		},
		{
			name: "date",
			element: &docs.ParagraphElement{DateElement: &docs.DateElement{
				SuggestedInsertionIds: []string{"insert"}, SuggestedDeletionIds: []string{"delete"},
			}},
		},
		{
			name: "equation",
			element: &docs.ParagraphElement{Equation: &docs.Equation{
				SuggestedInsertionIds: []string{"insert"}, SuggestedDeletionIds: []string{"delete"},
			}},
		},
		{
			name: "footnote reference",
			element: &docs.ParagraphElement{FootnoteReference: &docs.FootnoteReference{
				SuggestedInsertionIds: []string{"insert"}, SuggestedDeletionIds: []string{"delete"},
			}},
		},
		{
			name: "horizontal rule",
			element: &docs.ParagraphElement{HorizontalRule: &docs.HorizontalRule{
				SuggestedInsertionIds: []string{"insert"}, SuggestedDeletionIds: []string{"delete"},
			}},
		},
		{
			name: "inline object",
			element: &docs.ParagraphElement{InlineObjectElement: &docs.InlineObjectElement{
				SuggestedInsertionIds: []string{"insert"}, SuggestedDeletionIds: []string{"delete"},
			}},
		},
		{
			name: "page break",
			element: &docs.ParagraphElement{PageBreak: &docs.PageBreak{
				SuggestedInsertionIds: []string{"insert"}, SuggestedDeletionIds: []string{"delete"},
			}},
		},
		{
			name: "person chip",
			element: &docs.ParagraphElement{Person: &docs.Person{
				SuggestedInsertionIds: []string{"insert"}, SuggestedDeletionIds: []string{"delete"},
			}},
		},
		{
			name: "rich link chip",
			element: &docs.ParagraphElement{RichLink: &docs.RichLink{
				SuggestedInsertionIds: []string{"insert"}, SuggestedDeletionIds: []string{"delete"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.element.StartIndex = 4
			tt.element.EndIndex = 5
			doc := &docs.Document{Body: &docs.Body{Content: []*docs.StructuralElement{{
				Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{tt.element}},
			}}}}

			got := enumerateDocsSuggestions(doc)
			want := []docsSuggestionListItem{
				{SuggestionID: "insert", Kind: "insertion", Segment: "body", StartIndex: 4, EndIndex: 5},
				{SuggestionID: "delete", Kind: "deletion", Segment: "body", StartIndex: 4, EndIndex: 5},
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("suggestions mismatch\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}

func TestEnumerateDocsSuggestions_SectionBreak(t *testing.T) {
	t.Parallel()

	doc := &docs.Document{Body: &docs.Body{Content: []*docs.StructuralElement{{
		StartIndex: 8,
		EndIndex:   9,
		SectionBreak: &docs.SectionBreak{
			SuggestedInsertionIds: []string{"insert"},
			SuggestedDeletionIds:  []string{"delete"},
		},
	}}}}

	got := enumerateDocsSuggestions(doc)
	want := []docsSuggestionListItem{
		{SuggestionID: "insert", Kind: "insertion", Segment: "body", StartIndex: 8, EndIndex: 9},
		{SuggestionID: "delete", Kind: "deletion", Segment: "body", StartIndex: 8, EndIndex: 9},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("suggestions mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestEnumerateDocsSuggestions_PositionedObjects(t *testing.T) {
	t.Parallel()

	doc := &docs.Document{
		Body: &docs.Body{Content: []*docs.StructuralElement{{
			StartIndex: 10,
			EndIndex:   20,
			Paragraph: &docs.Paragraph{
				PositionedObjectIds: []string{"deleted"},
				SuggestedPositionedObjectIds: map[string]docs.ObjectReferences{
					"insert-suggestion": {ObjectIds: []string{"inserted"}},
				},
			},
		}}},
		PositionedObjects: map[string]docs.PositionedObject{
			"inserted": {SuggestedInsertionId: "insert-suggestion"},
			"deleted":  {SuggestedDeletionIds: []string{"delete-suggestion"}},
		},
	}

	got := enumerateDocsSuggestions(doc)
	want := []docsSuggestionListItem{
		{SuggestionID: "delete-suggestion", Kind: "deletion", Segment: "body", StartIndex: 10, EndIndex: 10},
		{SuggestionID: "insert-suggestion", Kind: "insertion", Segment: "body", StartIndex: 10, EndIndex: 10},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("suggestions mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestEnumerateDocsSuggestions_InlineObjectMap(t *testing.T) {
	t.Parallel()

	doc := &docs.Document{
		Body: &docs.Body{Content: []*docs.StructuralElement{{
			Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{
				StartIndex:          4,
				EndIndex:            5,
				InlineObjectElement: &docs.InlineObjectElement{InlineObjectId: "image"},
			}}},
		}}},
		InlineObjects: map[string]docs.InlineObject{
			"image": {
				SuggestedInsertionId: "insert",
				SuggestedDeletionIds: []string{"delete"},
			},
		},
	}

	got := enumerateDocsSuggestions(doc)
	want := []docsSuggestionListItem{
		{SuggestionID: "insert", Kind: "insertion", Segment: "body", StartIndex: 4, EndIndex: 5},
		{SuggestionID: "delete", Kind: "deletion", Segment: "body", StartIndex: 4, EndIndex: 5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("suggestions mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestEnumerateDocsSuggestions_ListMap(t *testing.T) {
	t.Parallel()

	doc := &docs.Document{
		Body: &docs.Body{Content: []*docs.StructuralElement{{
			StartIndex: 3,
			EndIndex:   7,
			Paragraph:  &docs.Paragraph{Bullet: &docs.Bullet{ListId: "list"}},
		}}},
		Lists: map[string]docs.List{
			"list": {
				SuggestedInsertionId: "insert",
				SuggestedDeletionIds: []string{"delete"},
			},
		},
	}

	got := enumerateDocsSuggestions(doc)
	want := []docsSuggestionListItem{
		{SuggestionID: "insert", Kind: "insertion", Segment: "body", StartIndex: 3, EndIndex: 3},
		{SuggestionID: "delete", Kind: "deletion", Segment: "body", StartIndex: 3, EndIndex: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("suggestions mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestDocsSuggestionsList_JSON(t *testing.T) {
	t.Parallel()

	srv := newDocsRawTestServerWithRequest(t, 0, map[string]any{
		"documentId": "doc1",
		"body": map[string]any{"content": []any{map[string]any{
			"paragraph": map[string]any{"elements": []any{map[string]any{
				"startIndex": 1,
				"endIndex":   6,
				"textRun": map[string]any{
					"content":               "draft",
					"suggestedInsertionIds": []any{"suggestion-1"},
				},
			}}},
		}}},
	}, func(r *http.Request) {
		if got := r.URL.Query().Get("suggestionsViewMode"); got != "SUGGESTIONS_INLINE" {
			t.Fatalf("suggestionsViewMode=%q", got)
		}
		if _, ok := r.URL.Query()["includeTabsContent"]; ok {
			t.Fatalf("default request unexpectedly set includeTabsContent: %s", r.URL.RawQuery)
		}
	})
	defer srv.Close()

	output := &bytes.Buffer{}
	ctx := newCmdRuntimeJSONOutputContext(t, output, io.Discard)
	ctx = withDocsTestService(ctx, newMockDocsService(t, srv))
	cmd := &DocsSuggestionsListCmd{}
	if err := runKong(t, cmd, []string{"doc1"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	var got struct {
		DocumentID  string                   `json:"documentId"`
		TabID       string                   `json:"tabId"`
		Suggestions []docsSuggestionListItem `json:"suggestions"`
	}
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	if got.DocumentID != "doc1" || got.TabID != "" || len(got.Suggestions) != 1 {
		t.Fatalf("unexpected output: %#v", got)
	}
	if got.Suggestions[0].SuggestionID != "suggestion-1" || got.Suggestions[0].Text != "draft" {
		t.Fatalf("unexpected suggestion: %#v", got.Suggestions[0])
	}
}

func TestDocsSuggestionsList_Tab(t *testing.T) {
	t.Parallel()

	srv := newDocsRawTestServerWithRequest(t, 0, map[string]any{
		"documentId": "doc1",
		"tabs": []any{map[string]any{
			"tabProperties": map[string]any{"tabId": "tab-1", "title": "Review"},
			"documentTab": map[string]any{
				"body": map[string]any{"content": []any{map[string]any{
					"paragraph": map[string]any{"elements": []any{map[string]any{
						"startIndex": 2,
						"endIndex":   5,
						"textRun": map[string]any{
							"content":              "old",
							"suggestedDeletionIds": []any{"suggestion-2"},
						},
					}}},
				}}},
			},
		}},
	}, func(r *http.Request) {
		if got := r.URL.Query().Get("suggestionsViewMode"); got != "SUGGESTIONS_INLINE" {
			t.Fatalf("suggestionsViewMode=%q", got)
		}
		if got := r.URL.Query().Get("includeTabsContent"); got != "true" {
			t.Fatalf("includeTabsContent=%q", got)
		}
	})
	defer srv.Close()

	output := &bytes.Buffer{}
	ctx := newCmdRuntimeJSONOutputContext(t, output, io.Discard)
	ctx = withDocsTestService(ctx, newMockDocsService(t, srv))
	cmd := &DocsSuggestionsListCmd{}
	if err := runKong(t, cmd, []string{"doc1", "--tab", "Review"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"tabId": "tab-1"`)) ||
		!bytes.Contains(output.Bytes(), []byte(`"kind": "deletion"`)) {
		t.Fatalf("unexpected output: %s", output.String())
	}
}

func TestDocsSuggestionsList_Text(t *testing.T) {
	t.Parallel()

	srv := newDocsRawTestServer(t, 0, map[string]any{
		"documentId": "doc1",
		"body": map[string]any{"content": []any{map[string]any{
			"paragraph": map[string]any{"elements": []any{map[string]any{
				"startIndex": 1,
				"endIndex":   3,
				"textRun": map[string]any{
					"content":               "a\tb\n",
					"suggestedInsertionIds": []any{"suggestion-1"},
				},
			}}},
		}}},
	})
	defer srv.Close()

	output := &bytes.Buffer{}
	ctx := newCmdRuntimeOutputContext(t, output, io.Discard)
	ctx = withDocsTestService(ctx, newMockDocsService(t, srv))
	if err := runKong(t, &DocsSuggestionsListCmd{}, []string{"doc1"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "SUGGESTION ID  KIND       SEGMENT  START  END  TEXT\nsuggestion-1   insertion  body     1      3    a\\tb\\n\n"
	if output.String() != want {
		t.Fatalf("output mismatch\n got: %q\nwant: %q", output.String(), want)
	}
}

func TestDocsSuggestionsList_EmptyDocID(t *testing.T) {
	t.Parallel()

	err := (&DocsSuggestionsListCmd{}).Run(context.Background(), &RootFlags{})
	if err == nil {
		t.Fatal("expected empty docId error")
	}
}

func suggestionContent(start, end int64, text, insertionID string) []*docs.StructuralElement {
	run := &docs.ParagraphElement{
		StartIndex: start,
		EndIndex:   end,
		TextRun: &docs.TextRun{
			Content:               text,
			SuggestedInsertionIds: []string{insertionID},
		},
	}
	return []*docs.StructuralElement{{Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{run}}}}
}
