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
	var facts docsedit.PlacementFacts
	var document *docs.Document

	switch placement.Kind {
	case docsedit.PlacementAnchor:
		anchor, err := resolveDocsAtAnchor(ctx, svc, docID, docsAtAnchorFlags{
			At:         placement.Anchor.Text,
			Occurrence: placement.Anchor.Occurrence,
			MatchCase:  placement.Anchor.MatchCase,
			Tab:        tab,
		})
		if err != nil {
			return docsResolvedPlacement{}, err
		}
		facts.Anchor = &anchor.Match
		facts.RequiredRevisionID = anchor.RevisionID
		document = anchor.Document
	case docsedit.PlacementEnd:
		endIndex, tabID, err := docsTargetEndIndexAndTabID(ctx, svc, docID, tab)
		if err != nil {
			return docsResolvedPlacement{}, err
		}
		facts.EndIndex = endIndex
		facts.TabID = tabID
	case docsedit.PlacementIndex, docsedit.PlacementRange:
		if strings.TrimSpace(tab) != "" {
			tabID, err := resolveDocsTabID(ctx, svc, docID, tab)
			if err != nil {
				return docsResolvedPlacement{}, err
			}
			facts.TabID = tabID
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
