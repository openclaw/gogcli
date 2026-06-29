package docsedit

import (
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/docsformat"
)

func TestBuildWriteRequestsReplaceAndFormat(t *testing.T) {
	requests, err := BuildWriteRequests(WriteOptions{
		EndIndex:    12,
		InsertIndex: 1,
		Text:        "A😀",
		TabID:       "t.second",
		Format:      docsformat.Options{Bold: true},
	})
	if err != nil {
		t.Fatalf("BuildWriteRequests: %v", err)
	}

	if len(requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(requests))
	}

	deletion := requests[0].DeleteContentRange
	if deletion == nil || deletion.Range.StartIndex != 1 || deletion.Range.EndIndex != 11 ||
		deletion.Range.TabId != "t.second" {
		t.Fatalf("delete request = %#v", requests[0])
	}

	insertion := requests[1].InsertText
	if insertion == nil || insertion.Location.Index != 1 || insertion.Location.TabId != "t.second" ||
		insertion.Text != "A😀" {
		t.Fatalf("insert request = %#v", requests[1])
	}

	format := requests[2].UpdateTextStyle
	if format == nil || format.Range.StartIndex != 1 || format.Range.EndIndex != 4 ||
		format.Range.TabId != "t.second" {
		t.Fatalf("format request = %#v", requests[2])
	}
}

func TestBuildWriteRequestsAppendSkipsDelete(t *testing.T) {
	requests, err := BuildWriteRequests(WriteOptions{
		EndIndex:    12,
		InsertIndex: 11,
		Text:        "world",
		TabID:       "t.second",
		Append:      true,
	})
	if err != nil {
		t.Fatalf("BuildWriteRequests: %v", err)
	}

	if len(requests) != 1 || requests[0].InsertText == nil {
		t.Fatalf("requests = %#v", requests)
	}

	if requests[0].InsertText.Location.Index != 11 {
		t.Fatalf("insert index = %d, want 11", requests[0].InsertText.Location.Index)
	}
}

func TestBuildWriteRequestsBulletsAdjustParagraphRangeAfterTabs(t *testing.T) {
	requests, err := BuildWriteRequests(WriteOptions{
		EndIndex:    1,
		InsertIndex: 1,
		Text:        "\tFirst\n\t\tSecond\n",
		Format: docsformat.Options{
			Ordered:     true,
			SpacingMode: docsformat.SpacingModeCollapseLists,
		},
	})
	if err != nil {
		t.Fatalf("BuildWriteRequests: %v", err)
	}

	if len(requests) != 3 || requests[0].InsertText == nil || requests[1].CreateParagraphBullets == nil || requests[2].UpdateParagraphStyle == nil {
		t.Fatalf("requests = %#v", requests)
	}

	if got := requests[2].UpdateParagraphStyle.Range; got.StartIndex != 1 || got.EndIndex != 14 {
		t.Fatalf("post-bullet paragraph range = %#v, want 1..14", got)
	}

	if got := requests[2].UpdateParagraphStyle.ParagraphStyle.SpacingMode; got != docsformat.SpacingModeCollapseLists {
		t.Fatalf("spacing mode = %q, want %q", got, docsformat.SpacingModeCollapseLists)
	}
}

func TestBuildWriteRequestsEmptyBodySkipsDelete(t *testing.T) {
	requests, err := BuildWriteRequests(WriteOptions{
		EndIndex:    1,
		InsertIndex: 1,
		Text:        "hello",
	})
	if err != nil {
		t.Fatalf("BuildWriteRequests: %v", err)
	}

	if len(requests) != 1 || requests[0].InsertText == nil {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestBuildUpdateRequestsReplace(t *testing.T) {
	target := Range{Start: 7, End: 12}

	requests := BuildUpdateRequests("replacement", 7, "t.second", &target)
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}

	if got := requests[0].DeleteContentRange.Range; got.StartIndex != 7 || got.EndIndex != 12 ||
		got.TabId != "t.second" {
		t.Fatalf("delete range = %#v", got)
	}

	if got := requests[1].InsertText; got.Location.Index != 7 || got.Location.TabId != "t.second" ||
		got.Text != "replacement" {
		t.Fatalf("insert = %#v", got)
	}
}

func TestBuildInsertAndDeleteRequests(t *testing.T) {
	insert := BuildInsertRequest("hello", 5, "tab-1")
	if insert.InsertText == nil || insert.InsertText.Location.Index != 5 ||
		insert.InsertText.Location.TabId != "tab-1" {
		t.Fatalf("insert = %#v", insert)
	}

	deleteRequest := BuildDeleteRequest(Range{Start: 2, End: 7}, "tab-1")
	if got := deleteRequest.DeleteContentRange.Range; got.StartIndex != 2 || got.EndIndex != 7 ||
		got.TabId != "tab-1" {
		t.Fatalf("delete = %#v", got)
	}
}

func TestParseRange(t *testing.T) {
	got, ok, err := ParseRange(" 7 : 12 ")
	if err != nil || !ok || got != (Range{Start: 7, End: 12}) {
		t.Fatalf("ParseRange = (%#v, %t, %v)", got, ok, err)
	}

	got, ok, err = ParseRange(" ")
	if err != nil || ok || got != (Range{}) {
		t.Fatalf("empty ParseRange = (%#v, %t, %v)", got, ok, err)
	}

	for _, value := range []string{"7", "0:2", "2:2", "2:one"} {
		_, _, err = ParseRange(value)
		if err == nil || !strings.Contains(err.Error(), "replace-range") {
			t.Fatalf("ParseRange(%q) error = %v", value, err)
		}
	}
}
