package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docsedit"
)

type docsResolvedPlacement struct {
	docsedit.ResolvedPlacement
	Document *docs.Document
}

func resolveDocsPlacement(
	ctx context.Context,
	svc *docs.Service,
	docID string,
	tab string,
	placement docsedit.Placement,
) (docsResolvedPlacement, error) {
	return resolveDocsPlacementTarget(ctx, svc, docID, tab, "", placement)
}

func resolveDocsPlacementTarget(
	ctx context.Context,
	svc *docs.Service,
	docID string,
	tab string,
	segment string,
	placement docsedit.Placement,
) (docsResolvedPlacement, error) {
	var facts docsedit.PlacementFacts
	var document *docs.Document

	switch placement.Kind {
	case docsedit.PlacementAnchor:
		anchor, err := resolveDocsAtAnchor(ctx, svc, docID, docsAtAnchorFlags{
			At:         placement.Anchor.Text,
			Occurrence: placement.Anchor.Occurrence,
			MatchCase:  placement.Anchor.MatchCase,
			Tab:        tab,
			Segment:    segment,
		})
		if err != nil {
			return docsResolvedPlacement{}, err
		}
		facts.Anchor = &anchor.Match
		facts.RequiredRevisionID = anchor.RevisionID
		facts.SegmentID = anchor.SegmentID
		facts.SegmentKind = anchor.SegmentKind
		document = anchor.Document
	case docsedit.PlacementEnd:
		loaded, err := loadDocsTargetSegment(ctx, svc, docID, tab, segment)
		if err != nil {
			return docsResolvedPlacement{}, err
		}
		facts.EndIndex = docsDocumentEndIndex(loaded.target)
		facts.TabID = loaded.tabID
		facts.SegmentID = loaded.segmentID
		facts.SegmentKind = loaded.segmentKind
		document = loaded.full
	case docsedit.PlacementIndex, docsedit.PlacementRange:
		if strings.TrimSpace(tab) != "" || strings.TrimSpace(segment) != "" {
			loaded, err := loadDocsTargetSegment(ctx, svc, docID, tab, segment)
			if err != nil {
				return docsResolvedPlacement{}, err
			}
			facts.TabID = loaded.tabID
			facts.SegmentID = loaded.segmentID
			facts.SegmentKind = loaded.segmentKind
			document = loaded.full
		}
	default:
		return docsResolvedPlacement{}, fmt.Errorf("resolve Docs placement: unsupported kind %d", placement.Kind)
	}

	resolved, err := docsedit.ResolvePlacement(placement, facts)
	if err != nil {
		return docsResolvedPlacement{}, err
	}
	return docsResolvedPlacement{
		ResolvedPlacement: resolved,
		Document:          document,
	}, nil
}
