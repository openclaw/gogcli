package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestSelectDocsSegmentTargetFindsSegmentAcrossTabs(t *testing.T) {
	t.Parallel()
	doc := &docs.Document{DocumentId: "doc1", RevisionId: "rev1", Tabs: []*docs.Tab{
		{
			TabProperties: &docs.TabProperties{TabId: "tab-1", Title: "First"},
			DocumentTab:   &docs.DocumentTab{Body: &docs.Body{}},
		},
		{
			TabProperties: &docs.TabProperties{TabId: "tab-2", Title: "Second"},
			DocumentTab: &docs.DocumentTab{
				Body: &docs.Body{},
				Headers: map[string]docs.Header{
					"header-2": {HeaderId: "header-2", Content: []*docs.StructuralElement{{StartIndex: 0, EndIndex: 7}}},
				},
			},
		},
	}}

	target, err := selectDocsSegmentTarget(doc, "", "header-2")
	if err != nil {
		t.Fatalf("select segment: %v", err)
	}
	if target.tabID != "tab-2" || target.segmentID != "header-2" || target.segmentKind != docsSegmentKindHeader {
		t.Fatalf("unexpected target: %#v", target)
	}
	if got := docsDocumentEndIndex(target.target); got != 7 {
		t.Fatalf("segment end = %d, want 7", got)
	}
}

func TestApplyDocsRequestTargetPropagatesSegment(t *testing.T) {
	t.Parallel()
	requests := []*docs.Request{
		{InsertText: &docs.InsertTextRequest{Location: &docs.Location{Index: 2}, Text: "x"}},
		{DeleteContentRange: &docs.DeleteContentRangeRequest{Range: &docs.Range{StartIndex: 1, EndIndex: 2}}},
		{UpdateTextStyle: &docs.UpdateTextStyleRequest{Range: &docs.Range{StartIndex: 1, EndIndex: 2}}},
		{UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{Range: &docs.Range{StartIndex: 1, EndIndex: 2}}},
	}
	applyDocsRequestTarget(requests, docsRequestTarget{TabID: "tab-2", SegmentID: "header-2", SegmentKind: docsSegmentKindHeader})

	if got := requests[0].InsertText.Location; got.TabId != "tab-2" || got.SegmentId != "header-2" {
		t.Fatalf("insert location = %#v", got)
	}
	for index, request := range requests[1:] {
		ranges := docsRequestRanges(request)
		if len(ranges) != 1 || ranges[0].TabId != "tab-2" || ranges[0].SegmentId != "header-2" {
			t.Fatalf("request %d ranges = %#v", index+1, ranges)
		}
	}
	wire, err := json.Marshal(requests)
	if err != nil {
		t.Fatalf("marshal requests: %v", err)
	}
	encoded := string(wire)
	if !strings.Contains(encoded, `"segmentId":"header-2"`) {
		t.Fatalf("wire requests lost segment ID: %s", encoded)
	}
}

func TestApplyDocsRequestTargetForcesZeroIndexesOntoWire(t *testing.T) {
	t.Parallel()
	requests := []*docs.Request{
		{InsertText: &docs.InsertTextRequest{Location: &docs.Location{Index: 0}, Text: "x"}},
		{DeleteContentRange: &docs.DeleteContentRangeRequest{Range: &docs.Range{StartIndex: 0, EndIndex: 1}}},
	}
	applyDocsRequestTarget(requests, docsRequestTarget{SegmentID: "header-1", SegmentKind: docsSegmentKindHeader})
	wire, err := json.Marshal(requests)
	if err != nil {
		t.Fatalf("marshal requests: %v", err)
	}
	encoded := string(wire)
	if !strings.Contains(encoded, `"index":0`) || !strings.Contains(encoded, `"startIndex":0`) {
		t.Fatalf("zero indexes omitted from wire request: %s", encoded)
	}
}
