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

	tableCellContent := suggestionContent(10, 14, "cell", "table", nil)
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
				Table: &docs.Table{TableRows: []*docs.TableRow{{
					TableCells: []*docs.TableCell{{Content: tableCellContent}},
				}}},
			},
		}},
		Headers: map[string]docs.Header{
			"z": {Content: suggestionContent(3, 5, "zz", "header-z", nil)},
			"a": {Content: suggestionContent(1, 3, "aa", "header-a", nil)},
		},
	}

	got := enumerateDocsSuggestions(doc)
	want := []docsSuggestionListItem{
		{SuggestionID: "insert", Kind: "insertion", Segment: "body", StartIndex: 1, EndIndex: 6, Text: "hello"},
		{SuggestionID: "nested", Kind: "insertion", Segment: "body", StartIndex: 1, EndIndex: 4, Text: "hel"},
		{SuggestionID: "delete", Kind: "deletion", Segment: "body", StartIndex: 6, EndIndex: 7, Text: "!"},
		{SuggestionID: "table", Kind: "insertion", Segment: "body", StartIndex: 10, EndIndex: 14, Text: "cell"},
		{SuggestionID: "header-a", Kind: "insertion", Segment: "header:a", StartIndex: 1, EndIndex: 3, Text: "aa"},
		{SuggestionID: "header-z", Kind: "insertion", Segment: "header:z", StartIndex: 3, EndIndex: 5, Text: "zz"},
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

func suggestionContent(start, end int64, text, insertionID string, deletionIDs []string) []*docs.StructuralElement {
	run := &docs.ParagraphElement{
		StartIndex: start,
		EndIndex:   end,
		TextRun: &docs.TextRun{
			Content:               text,
			SuggestedInsertionIds: []string{insertionID},
			SuggestedDeletionIds:  deletionIDs,
		},
	}
	return []*docs.StructuralElement{{Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{run}}}}
}
