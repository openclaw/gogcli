package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesLocateCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	Text           string `arg:"" name:"text" help:"Literal text to locate"`
	Page           string `name:"page" help:"Limit matches to one slide object ID"`
	MatchCase      bool   `name:"match-case" help:"Use case-sensitive matching"`
	All            bool   `name:"all" help:"Return all matches"`
	Occurrence     *int   `name:"occurrence" help:"Return the Nth occurrence (1-based; default first)"`
	FailEmpty      bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no matches"`
}

type slidesLocateResult struct {
	Matches []slidesTextLocation `json:"matches"`
}

type slidesTextLocation struct {
	SlideObjectID string `json:"slideObjectId"`
	SlideNumber   int    `json:"slideNumber"`
	ObjectID      string `json:"objectId"`
	Kind          string `json:"kind"`
	RowIndex      *int64 `json:"rowIndex,omitempty"`
	ColumnIndex   *int64 `json:"columnIndex,omitempty"`
	StartIndex    int64  `json:"startIndex"`
	EndIndex      int64  `json:"endIndex"`
	Text          string `json:"text"`
}

func (c *SlidesLocateCmd) Run(ctx context.Context, flags *RootFlags) error {
	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	if c.Text == "" {
		return usage("empty text")
	}
	if c.All && c.Occurrence != nil {
		return usage("--all and --occurrence are mutually exclusive")
	}
	if c.Occurrence != nil && *c.Occurrence <= 0 {
		return usage("--occurrence must be > 0")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}
	presentation, err := svc.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}

	pageID := strings.TrimSpace(c.Page)
	locations := []slidesTextLocation{}
	pageFound := pageID == ""
	for slideIndex, slide := range presentation.Slides {
		if slide == nil || (pageID != "" && slide.ObjectId != pageID) {
			continue
		}
		pageFound = true
		locations = append(locations, locateSlidesTextInElements(slide.PageElements, c.Text, c.MatchCase, slide.ObjectId, slideIndex+1)...)
	}
	if !pageFound {
		return fmt.Errorf("slide %q not found in presentation", pageID)
	}

	locations = selectSlidesTextLocations(locations, c.All, c.Occurrence)
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), slidesLocateResult{Matches: locations}); err != nil {
			return err
		}
		return failEmptyIfNoSlidesLocations(c.FailEmpty, locations)
	}

	u := ui.FromContext(ctx)
	for _, location := range locations {
		cell := ""
		if location.RowIndex != nil && location.ColumnIndex != nil {
			cell = fmt.Sprintf("%d:%d", *location.RowIndex, *location.ColumnIndex)
		}
		u.Out().Linef("%s\t%s\t%s\t%d\t%d\t%s", location.SlideObjectID, location.ObjectID, cell, location.StartIndex, location.EndIndex, location.Text)
	}
	return failEmptyIfNoSlidesLocations(c.FailEmpty, locations)
}

func selectSlidesTextLocations(locations []slidesTextLocation, all bool, occurrence *int) []slidesTextLocation {
	if all {
		return locations
	}
	selected := 1
	if occurrence != nil {
		selected = *occurrence
	}
	if selected > len(locations) {
		return []slidesTextLocation{}
	}
	return locations[selected-1 : selected]
}

func failEmptyIfNoSlidesLocations(failEmpty bool, locations []slidesTextLocation) error {
	if len(locations) == 0 {
		return failEmptyExit(failEmpty)
	}
	return nil
}

func locateSlidesTextInElements(elements []*slides.PageElement, needle string, matchCase bool, slideID string, slideNumber int) []slidesTextLocation {
	locations := []slidesTextLocation{}
	for _, element := range elements {
		if element == nil {
			continue
		}
		if element.Shape != nil && element.Shape.Text != nil {
			for _, match := range locateSlidesText(element.Shape.Text, needle, matchCase) {
				locations = append(locations, slidesTextLocation{
					SlideObjectID: slideID,
					SlideNumber:   slideNumber,
					ObjectID:      element.ObjectId,
					Kind:          "shape",
					StartIndex:    match.StartIndex,
					EndIndex:      match.EndIndex,
					Text:          match.Text,
				})
			}
		}
		if element.Table != nil {
			for rowIndex, row := range element.Table.TableRows {
				if row == nil {
					continue
				}
				for columnIndex, cell := range row.TableCells {
					if cell == nil || cell.Text == nil {
						continue
					}
					cellRow := int64(rowIndex)
					cellColumn := int64(columnIndex)
					if cell.Location != nil {
						cellRow = cell.Location.RowIndex
						cellColumn = cell.Location.ColumnIndex
					}
					for _, match := range locateSlidesText(cell.Text, needle, matchCase) {
						locations = append(locations, slidesTextLocation{
							SlideObjectID: slideID,
							SlideNumber:   slideNumber,
							ObjectID:      element.ObjectId,
							Kind:          "tableCell",
							RowIndex:      int64Ptr(cellRow),
							ColumnIndex:   int64Ptr(cellColumn),
							StartIndex:    match.StartIndex,
							EndIndex:      match.EndIndex,
							Text:          match.Text,
						})
					}
				}
			}
		}
		if element.ElementGroup != nil {
			locations = append(locations, locateSlidesTextInElements(element.ElementGroup.Children, needle, matchCase, slideID, slideNumber)...)
		}
	}
	return locations
}

type slidesTextMatch struct {
	StartIndex int64
	EndIndex   int64
	Text       string
}

type slidesTextSegment struct {
	Text       string
	Boundaries map[int]int64
}

func locateSlidesText(content *slides.TextContent, needle string, matchCase bool) []slidesTextMatch {
	pattern := regexp.QuoteMeta(needle)
	if !matchCase {
		pattern = "(?i:" + pattern + ")"
	}
	re := regexp.MustCompile(pattern)
	matches := []slidesTextMatch{}
	for _, segment := range slidesTextSegments(content) {
		for _, indexes := range re.FindAllStringIndex(segment.Text, -1) {
			startIndex, startOK := segment.Boundaries[indexes[0]]
			endIndex, endOK := segment.Boundaries[indexes[1]]
			if !startOK || !endOK {
				continue
			}
			matches = append(matches, slidesTextMatch{
				StartIndex: startIndex,
				EndIndex:   endIndex,
				Text:       segment.Text[indexes[0]:indexes[1]],
			})
		}
	}
	return matches
}

func slidesTextSegments(content *slides.TextContent) []slidesTextSegment {
	segments := []slidesTextSegment{}
	if content == nil {
		return segments
	}

	var text strings.Builder
	boundaries := map[int]int64{}
	var lastEnd int64
	hasText := false
	flush := func() {
		if !hasText {
			return
		}
		segments = append(segments, slidesTextSegment{Text: text.String(), Boundaries: boundaries})
		text.Reset()
		boundaries = map[int]int64{}
		hasText = false
	}

	for _, element := range content.TextElements {
		if element == nil {
			continue
		}
		var runContent string
		switch {
		case element.TextRun != nil:
			runContent = element.TextRun.Content
		case element.AutoText != nil:
			runContent = element.AutoText.Content
		default:
			continue
		}
		if runContent == "" {
			continue
		}
		if hasText && element.StartIndex != lastEnd {
			flush()
		}

		byteOffset := text.Len()
		apiIndex := element.StartIndex
		boundaries[byteOffset] = apiIndex
		for _, r := range runContent {
			text.WriteRune(r)
			if r > 0xffff {
				apiIndex += int64(len(utf16.Encode([]rune{r})))
			} else {
				apiIndex++
			}
			boundaries[text.Len()] = apiIndex
		}
		lastEnd = element.EndIndex
		if lastEnd <= element.StartIndex {
			lastEnd = apiIndex
		}
		boundaries[text.Len()] = lastEnd
		hasText = true
	}
	flush()
	return segments
}
