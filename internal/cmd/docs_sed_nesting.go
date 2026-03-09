package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/docs/v1"
)

// applyDeferredBullets re-fetches the document and finds paragraphs that have
// leading \t characters (indicating pending bullet creation with nesting).
// It groups consecutive paragraphs by bullet preset and applies a single
// CreateParagraphBullets request per group, which allows Google Docs to
// interpret the tabs as nesting levels.
//
// Per the Google Docs API: "The nesting level of each paragraph is determined
// by counting leading tabs in front of each paragraph."
func (c *DocsSedCmd) applyDeferredBullets(ctx context.Context, docsSvc *docs.Service, id string) error {
	var doc *docs.Document
	err := retryOnQuota(ctx, func() error {
		var e error
		doc, e = docsSvc.Documents.Get(id).Context(ctx).Do()
		return e
	})
	if err != nil {
		return err
	}

	if doc.Body == nil {
		return nil
	}

	// Find paragraphs that need bullets. These are paragraphs with leading \t
	// that don't already have a bullet (bullets were deferred).
	// Also include non-tab paragraphs that are adjacent to tab paragraphs
	// and match the same list type (they're the L0 parents).
	type pendingBullet struct {
		startIndex int64
		endIndex   int64
		preset     string
		hasTab     bool
	}
	var pending []pendingBullet

	for _, elem := range doc.Body.Content {
		if elem.Paragraph == nil {
			continue
		}
		// Skip paragraphs that already have bullets
		if elem.Paragraph.Bullet != nil {
			continue
		}

		// Check text content for leading tab or bullet-like content
		for _, pe := range elem.Paragraph.Elements {
			if pe.TextRun == nil {
				continue
			}
			content := pe.TextRun.Content
			hasTab := strings.HasPrefix(content, "\t")
			if hasTab {
				// This paragraph has a deferred nested bullet
				pending = append(pending, pendingBullet{
					startIndex: elem.StartIndex,
					endIndex:   elem.EndIndex,
					preset:     bulletPresetDisc, // default, will be refined
					hasTab:     true,
				})
			}
			break // only check first text run
		}
	}

	if len(pending) == 0 {
		return nil
	}

	// Now we need to also find the L0 parent paragraphs. These are non-tab
	// paragraphs immediately before a tab paragraph that were also bulleted
	// (they already have bullets from their own runManualInner call).
	// We need to include them in the same CreateParagraphBullets range.
	//
	// Strategy: expand each group to include adjacent bulleted paragraphs.
	// Re-scan all paragraphs and build a map of paragraph ranges.
	type paraInfo struct {
		startIndex int64
		endIndex   int64
		hasBullet  bool
		hasTab     bool
		preset     string
	}
	var allParas []paraInfo
	for _, elem := range doc.Body.Content {
		if elem.Paragraph == nil {
			continue
		}
		pi := paraInfo{
			startIndex: elem.StartIndex,
			endIndex:   elem.EndIndex,
			hasBullet:  elem.Paragraph.Bullet != nil,
		}
		if pi.hasBullet {
			pi.preset = inferBulletPreset(doc, elem.Paragraph.Bullet.ListId)
		}
		for _, pe := range elem.Paragraph.Elements {
			if pe.TextRun != nil {
				pi.hasTab = strings.HasPrefix(pe.TextRun.Content, "\t")
				break
			}
		}
		allParas = append(allParas, pi)
	}

	// Find groups: consecutive paragraphs where at least one has a tab
	// and all are either bulleted or have tabs (need bullets).
	type group struct {
		start, end int64
		preset     string
	}
	var groups []group

	for i := 0; i < len(allParas); i++ {
		p := allParas[i]
		if !p.hasTab && !p.hasBullet {
			continue
		}

		// Start a potential group
		groupStart := p.startIndex
		groupEnd := p.endIndex - 1
		preset := p.preset
		if preset == "" {
			preset = bulletPresetDisc
		}
		hasAnyTab := p.hasTab

		// Extend forward through adjacent bulleted/tab paragraphs of the same type.
		// Break when the preset changes (e.g., bullet → numbered).
		for i+1 < len(allParas) {
			next := allParas[i+1]
			if !next.hasTab && !next.hasBullet {
				break
			}
			// Break if the next paragraph has a different preset (different list type)
			if next.preset != "" && preset != "" && next.preset != preset {
				break
			}
			i++
			groupEnd = next.endIndex - 1
			if next.hasTab {
				hasAnyTab = true
			}
			if next.preset != "" {
				preset = next.preset
			}
		}

		// Only create a group if it contains at least one tab paragraph
		if hasAnyTab {
			groups = append(groups, group{start: groupStart, end: groupEnd, preset: preset})
		}
	}

	if len(groups) == 0 {
		return nil
	}

	// Get document body end index for clamping
	var bodyEnd int64
	if len(doc.Body.Content) > 0 {
		bodyEnd = doc.Body.Content[len(doc.Body.Content)-1].EndIndex
	}

	// For each group: delete existing bullets (if any), then re-create merged
	var requests []*docs.Request
	for i := range groups {
		g := &groups[i]
		// Clamp end to the last content index. The paragraph endIndex is
		// exclusive, and the body's final newline sits at the segment boundary.
		// Use endIndex-1 to stay within valid range.
		if g.end > bodyEnd-1 {
			g.end = bodyEnd - 1
		}
		if g.start >= g.end {
			continue
		}
		// Delete existing bullets first (some L0 items already have them)
		requests = append(requests, &docs.Request{
			DeleteParagraphBullets: &docs.DeleteParagraphBulletsRequest{
				Range: &docs.Range{StartIndex: g.start, EndIndex: g.end},
			},
		})
		// Re-create with merged range — tabs become nesting levels
		requests = append(requests, &docs.Request{
			CreateParagraphBullets: &docs.CreateParagraphBulletsRequest{
				Range:        &docs.Range{StartIndex: g.start, EndIndex: g.end},
				BulletPreset: g.preset,
			},
		})
	}

	// Process first group only — then recursively handle remaining groups
	// by re-fetching the doc (indices shift when tabs are consumed by bullets).
	if len(requests) >= 2 {
		err = retryOnQuota(ctx, func() error {
			_, e := docsSvc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{
				Requests: requests[:2],
			}).Context(ctx).Do()
			return e
		})
		if err != nil {
			return err
		}
		// If more groups remain, re-run to pick them up with fresh indices
		if len(requests) > 2 {
			return c.applyDeferredBullets(ctx, docsSvc, id)
		}
	}
	return nil
}

// inferBulletPreset determines the bullet preset from the list properties.
func inferBulletPreset(doc *docs.Document, listID string) string {
	if doc.Lists == nil {
		return bulletPresetDisc
	}
	list, ok := doc.Lists[listID]
	if !ok || list.ListProperties == nil {
		return bulletPresetDisc
	}

	levels := list.ListProperties.NestingLevels
	if len(levels) > 0 && levels[0] != nil {
		switch levels[0].GlyphType {
		case "DECIMAL", "ZERO_DECIMAL", "UPPER_ALPHA", "ALPHA",
			"UPPER_ROMAN", "ROMAN":
			return "NUMBERED_DECIMAL_NESTED"
		}
	}

	return bulletPresetDisc
}
