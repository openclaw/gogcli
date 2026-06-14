package docssed

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"
)

const (
	deferredBulletPresetDisc     = "BULLET_DISC_CIRCLE_SQUARE"
	deferredBulletPresetNumbered = "NUMBERED_DECIMAL_NESTED"
)

// DeferredBulletPlan is one index-stable nested-list repair batch.
type DeferredBulletPlan struct {
	Requests []*docs.Request
	More     bool
}

// PlanDeferredBullets plans the first nested-list group that needs repair.
func PlanDeferredBullets(document *docs.Document) DeferredBulletPlan {
	if document == nil || document.Body == nil {
		return DeferredBulletPlan{}
	}

	paragraphs, hasPending := deferredBulletParagraphs(document)
	if !hasPending {
		return DeferredBulletPlan{}
	}

	groups := deferredBulletGroups(paragraphs)
	if len(groups) == 0 {
		return DeferredBulletPlan{}
	}

	bodyEnd := int64(0)
	if content := document.Body.Content; len(content) > 0 {
		bodyEnd = content[len(content)-1].EndIndex
	}

	batches := make([][]*docs.Request, 0, len(groups))
	for _, group := range groups {
		requests := deferredBulletRequests(group, bodyEnd)
		if len(requests) > 0 {
			batches = append(batches, requests)
		}
	}

	if len(batches) == 0 {
		return DeferredBulletPlan{}
	}

	return DeferredBulletPlan{
		Requests: batches[0],
		More:     len(batches) > 1,
	}
}

// ApplyDeferredBullets repairs nested lists one group at a time, refetching
// after each index-shifting update.
func (e *Executor) ApplyDeferredBullets(ctx context.Context, documentID string) error {
	for {
		document, err := e.Get(ctx, documentID)
		if err != nil {
			return fmt.Errorf("get document for deferred bullets: %w", err)
		}

		plan := PlanDeferredBullets(document)
		if len(plan.Requests) == 0 {
			return nil
		}

		if _, err := e.BatchUpdate(ctx, documentID, plan.Requests); err != nil {
			return fmt.Errorf("apply deferred bullets: %w", err)
		}

		if !plan.More {
			return nil
		}
	}
}

type deferredBulletParagraph struct {
	startIndex int64
	endIndex   int64
	hasBullet  bool
	hasTab     bool
	preset     string
}

func deferredBulletParagraphs(document *docs.Document) ([]deferredBulletParagraph, bool) {
	paragraphs := make([]deferredBulletParagraph, 0, len(document.Body.Content))
	hasPending := false

	for _, element := range document.Body.Content {
		if element == nil || element.Paragraph == nil {
			continue
		}

		paragraph := deferredBulletParagraph{
			startIndex: element.StartIndex,
			endIndex:   element.EndIndex,
			hasBullet:  element.Paragraph.Bullet != nil,
		}
		if paragraph.hasBullet {
			paragraph.preset = inferDeferredBulletPreset(
				document,
				element.Paragraph.Bullet.ListId,
			)
		}

		for _, paragraphElement := range element.Paragraph.Elements {
			if paragraphElement == nil || paragraphElement.TextRun == nil {
				continue
			}
			paragraph.hasTab = strings.HasPrefix(paragraphElement.TextRun.Content, "\t")

			break
		}

		if paragraph.hasTab && !paragraph.hasBullet {
			hasPending = true
		}
		paragraphs = append(paragraphs, paragraph)
	}

	return paragraphs, hasPending
}

type deferredBulletGroup struct {
	startIndex int64
	endIndex   int64
	preset     string
}

func deferredBulletGroups(paragraphs []deferredBulletParagraph) []deferredBulletGroup {
	var groups []deferredBulletGroup

	for index := 0; index < len(paragraphs); index++ {
		paragraph := paragraphs[index]
		if !paragraph.hasTab && !paragraph.hasBullet {
			continue
		}

		group := deferredBulletGroup{
			startIndex: paragraph.startIndex,
			endIndex:   paragraph.endIndex - 1,
			preset:     paragraph.preset,
		}
		if group.preset == "" {
			group.preset = deferredBulletPresetDisc
		}
		hasTab := paragraph.hasTab

		for index+1 < len(paragraphs) {
			next := paragraphs[index+1]
			if !next.hasTab && !next.hasBullet {
				break
			}

			if next.preset != "" && next.preset != group.preset {
				break
			}

			index++
			group.endIndex = next.endIndex - 1
			hasTab = hasTab || next.hasTab

			if next.preset != "" {
				group.preset = next.preset
			}
		}

		if hasTab {
			groups = append(groups, group)
		}
	}

	return groups
}

func deferredBulletRequests(group deferredBulletGroup, bodyEnd int64) []*docs.Request {
	endIndex := group.endIndex
	if endIndex > bodyEnd-1 {
		endIndex = bodyEnd - 1
	}

	if group.startIndex >= endIndex {
		return nil
	}

	targetRange := &docs.Range{
		StartIndex: group.startIndex,
		EndIndex:   endIndex,
	}

	return []*docs.Request{
		{
			DeleteParagraphBullets: &docs.DeleteParagraphBulletsRequest{
				Range: targetRange,
			},
		},
		{
			CreateParagraphBullets: &docs.CreateParagraphBulletsRequest{
				Range:        targetRange,
				BulletPreset: group.preset,
			},
		},
	}
}

func inferDeferredBulletPreset(document *docs.Document, listID string) string {
	if document.Lists == nil {
		return deferredBulletPresetDisc
	}

	list, ok := document.Lists[listID]
	if !ok || list.ListProperties == nil {
		return deferredBulletPresetDisc
	}

	levels := list.ListProperties.NestingLevels
	if len(levels) == 0 || levels[0] == nil {
		return deferredBulletPresetDisc
	}

	switch levels[0].GlyphType {
	case "DECIMAL", "ZERO_DECIMAL", "UPPER_ALPHA", "ALPHA",
		"UPPER_ROMAN", "ROMAN":
		return deferredBulletPresetNumbered
	default:
		return deferredBulletPresetDisc
	}
}
