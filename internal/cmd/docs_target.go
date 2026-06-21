package cmd

import (
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docsedit"
)

const (
	docsSegmentKindBody     = "body"
	docsSegmentKindHeader   = "header"
	docsSegmentKindFooter   = "footer"
	docsSegmentKindFootnote = "footnote"
)

type docsRequestTarget struct {
	TabID       string
	SegmentID   string
	SegmentKind string
}

func docsTargetFromPlacement(resolved docsedit.ResolvedPlacement) docsRequestTarget {
	kind := resolved.SegmentKind
	if kind == "" {
		kind = docsSegmentKindBody
	}
	return docsRequestTarget{TabID: resolved.TabID, SegmentID: resolved.SegmentID, SegmentKind: kind}
}

func applyDocsRequestTarget(requests []*docs.Request, target docsRequestTarget) {
	for _, req := range requests {
		if req == nil {
			continue
		}
		if req.InsertText != nil {
			if req.InsertText.Location != nil {
				req.InsertText.Location.TabId = target.TabID
				req.InsertText.Location.SegmentId = target.SegmentID
				forceZeroLocationIndex(req.InsertText.Location)
			}
			if req.InsertText.EndOfSegmentLocation != nil {
				req.InsertText.EndOfSegmentLocation.TabId = target.TabID
				req.InsertText.EndOfSegmentLocation.SegmentId = target.SegmentID
			}
		}
		for _, location := range docsRequestLocations(req) {
			location.TabId = target.TabID
			location.SegmentId = target.SegmentID
			forceZeroLocationIndex(location)
		}
		for _, targetRange := range docsRequestRanges(req) {
			targetRange.TabId = target.TabID
			targetRange.SegmentId = target.SegmentID
			forceZeroRangeStart(targetRange)
		}
	}
}

func forceZeroLocationIndex(location *docs.Location) {
	if location != nil && location.Index == 0 && !docsContainsString(location.ForceSendFields, "Index") {
		location.ForceSendFields = append(location.ForceSendFields, "Index")
	}
}

func forceZeroRangeStart(targetRange *docs.Range) {
	if targetRange != nil && targetRange.StartIndex == 0 && !docsContainsString(targetRange.ForceSendFields, "StartIndex") {
		targetRange.ForceSendFields = append(targetRange.ForceSendFields, "StartIndex")
	}
}

func docsContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func docsRequestLocations(req *docs.Request) []*docs.Location {
	var locations []*docs.Location
	if req.InsertPageBreak != nil && req.InsertPageBreak.Location != nil {
		locations = append(locations, req.InsertPageBreak.Location)
	}
	if req.InsertSectionBreak != nil && req.InsertSectionBreak.Location != nil {
		locations = append(locations, req.InsertSectionBreak.Location)
	}
	if req.CreateFootnote != nil && req.CreateFootnote.Location != nil {
		locations = append(locations, req.CreateFootnote.Location)
	}
	return locations
}

func docsRequestRanges(req *docs.Request) []*docs.Range {
	var ranges []*docs.Range
	if req.DeleteContentRange != nil && req.DeleteContentRange.Range != nil {
		ranges = append(ranges, req.DeleteContentRange.Range)
	}
	if req.UpdateTextStyle != nil && req.UpdateTextStyle.Range != nil {
		ranges = append(ranges, req.UpdateTextStyle.Range)
	}
	if req.UpdateParagraphStyle != nil && req.UpdateParagraphStyle.Range != nil {
		ranges = append(ranges, req.UpdateParagraphStyle.Range)
	}
	if req.CreateParagraphBullets != nil && req.CreateParagraphBullets.Range != nil {
		ranges = append(ranges, req.CreateParagraphBullets.Range)
	}
	if req.DeleteParagraphBullets != nil && req.DeleteParagraphBullets.Range != nil {
		ranges = append(ranges, req.DeleteParagraphBullets.Range)
	}
	if req.UpdateSectionStyle != nil && req.UpdateSectionStyle.Range != nil {
		ranges = append(ranges, req.UpdateSectionStyle.Range)
	}
	return ranges
}

func selectDocsSegmentTarget(doc *docs.Document, tabQuery, segmentID string) (*docsLoadedTarget, error) {
	tabQuery = strings.TrimSpace(tabQuery)
	segmentID = strings.TrimSpace(segmentID)
	if segmentID == "" {
		return nil, usage("empty --segment")
	}

	if tabQuery != "" {
		tab, err := findTab(flattenTabs(doc.Tabs), tabQuery)
		if err != nil {
			return nil, err
		}
		return docsSegmentInTab(doc, tab, segmentID)
	}

	var matches []*docsLoadedTarget
	for _, tab := range flattenTabs(doc.Tabs) {
		match, err := docsSegmentInTab(doc, tab, segmentID)
		if err == nil {
			matches = append(matches, match)
		}
	}
	if match := docsLegacySegment(doc, segmentID); match != nil {
		matches = append(matches, match)
	}
	if len(matches) == 0 {
		return nil, usagef("segment not found: %s", segmentID)
	}
	if len(matches) > 1 {
		return nil, usagef("segment %s exists in multiple tabs; pass --tab", segmentID)
	}
	return matches[0], nil
}

func docsSegmentInTab(doc *docs.Document, tab *docs.Tab, segmentID string) (*docsLoadedTarget, error) {
	if tab == nil || tab.DocumentTab == nil {
		return nil, usagef("segment not found: %s", segmentID)
	}
	tabID := ""
	if tab.TabProperties != nil {
		tabID = strings.TrimSpace(tab.TabProperties.TabId)
	}
	content, kind, ok := docsSegmentContent(tab.DocumentTab.Headers, tab.DocumentTab.Footers, tab.DocumentTab.Footnotes, segmentID)
	if !ok {
		return nil, usagef("segment not found in tab %s: %s", tabID, segmentID)
	}
	return newDocsSegmentTarget(doc, tabID, segmentID, kind, content), nil
}

func docsLegacySegment(doc *docs.Document, segmentID string) *docsLoadedTarget {
	content, kind, ok := docsSegmentContent(doc.Headers, doc.Footers, doc.Footnotes, segmentID)
	if !ok {
		return nil
	}
	return newDocsSegmentTarget(doc, "", segmentID, kind, content)
}

func docsSegmentContent(headers map[string]docs.Header, footers map[string]docs.Footer, footnotes map[string]docs.Footnote, segmentID string) ([]*docs.StructuralElement, string, bool) {
	if header, ok := headers[segmentID]; ok {
		return header.Content, docsSegmentKindHeader, true
	}
	if footer, ok := footers[segmentID]; ok {
		return footer.Content, docsSegmentKindFooter, true
	}
	if footnote, ok := footnotes[segmentID]; ok {
		return footnote.Content, docsSegmentKindFootnote, true
	}
	return nil, "", false
}

func newDocsSegmentTarget(doc *docs.Document, tabID, segmentID, kind string, content []*docs.StructuralElement) *docsLoadedTarget {
	return &docsLoadedTarget{
		full: doc,
		target: &docs.Document{
			DocumentId: doc.DocumentId,
			RevisionId: doc.RevisionId,
			Body:       &docs.Body{Content: content},
		},
		tabID:       tabID,
		segmentID:   segmentID,
		segmentKind: kind,
	}
}
