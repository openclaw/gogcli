package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docsedit"
)

type docsAtAnchorFlags struct {
	At         string
	AtProvided bool
	Occurrence *int
	MatchCase  bool
	Tab        string
	Segment    string
}

type docsResolvedAtAnchor struct {
	Match       docsedit.TextRange
	RevisionID  string
	Document    *docs.Document
	SegmentID   string
	SegmentKind string
}

const docsAtIndexAnchorStart = "at:start"

func (a docsAtAnchorFlags) options() docsedit.AnchorOptions {
	return docsedit.AnchorOptions{
		Text:       a.At,
		Provided:   a.AtProvided,
		Occurrence: a.Occurrence,
		MatchCase:  a.MatchCase,
	}
}

func resolveDocsAtAnchor(ctx context.Context, svc *docs.Service, docID string, anchor docsAtAnchorFlags) (docsResolvedAtAnchor, error) {
	if anchor.At == "" {
		return docsResolvedAtAnchor{}, usage("empty --at")
	}
	if err := docsedit.ValidateAnchor(anchor.options()); err != nil {
		return docsResolvedAtAnchor{}, usage(err.Error())
	}

	loaded, err := loadDocsTargetSegment(ctx, svc, docID, anchor.Tab, anchor.Segment)
	if err != nil {
		return docsResolvedAtAnchor{}, err
	}
	matches := docsedit.FindTextRanges(loaded.target, anchor.At, docsedit.SearchOptions{
		MatchCase:            anchor.MatchCase,
		NormalizeWhitespace:  false,
		PreserveHTMLEntities: true,
		RequireTextSegment:   true,
		TabID:                loaded.tabID,
	})
	match, err := selectDocsAtAnchorMatch(anchor.At, matches, anchor.Occurrence)
	if err != nil {
		return docsResolvedAtAnchor{}, err
	}
	return docsResolvedAtAnchor{
		Match: match, RevisionID: loaded.full.RevisionId, Document: loaded.full,
		SegmentID: loaded.segmentID, SegmentKind: loaded.segmentKind,
	}, nil
}

func selectDocsAtAnchorMatch(at string, matches []docsedit.TextRange, occurrence *int) (docsedit.TextRange, error) {
	if len(matches) == 0 {
		return docsedit.TextRange{}, &ExitError{
			Code: emptyResultsExitCode,
			Err:  fmt.Errorf("anchor not found: %q", at),
		}
	}
	if occurrence != nil {
		if *occurrence <= 0 {
			return docsedit.TextRange{}, usage("--occurrence must be > 0")
		}
		if *occurrence > len(matches) {
			return docsedit.TextRange{}, &ExitError{
				Code: emptyResultsExitCode,
				Err:  fmt.Errorf("anchor %q occurrence %d not found; matches: %s", at, *occurrence, formatDocsAnchorOccurrences(matches)),
			}
		}
		return matches[*occurrence-1], nil
	}
	if len(matches) > 1 {
		return docsedit.TextRange{}, usagef("ambiguous --at %q matched %d occurrences; pass --occurrence 1..%d (matches: %s)", at, len(matches), len(matches), formatDocsAnchorOccurrences(matches))
	}
	return matches[0], nil
}

func formatDocsAnchorOccurrences(matches []docsedit.TextRange) string {
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		part := fmt.Sprintf("%d:%d", match.StartIndex, match.EndIndex)
		if match.TabID != "" {
			part += "@" + match.TabID
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

func addDocsAtAnchorDryRunPayload(payload map[string]any, anchor docsAtAnchorFlags) {
	if anchor.At == "" {
		return
	}
	payload["at"] = anchor.At
	if anchor.Occurrence != nil {
		payload["occurrence"] = *anchor.Occurrence
	}
	if anchor.MatchCase {
		payload["matchCase"] = true
	}
}

func docsRequiredRevisionWriteControl(revisionID string) *docs.WriteControl {
	if revisionID == "" {
		return nil
	}
	return &docs.WriteControl{RequiredRevisionId: revisionID}
}
