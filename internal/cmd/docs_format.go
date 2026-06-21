package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docsedit"
	"github.com/steipete/gogcli/internal/docsformat"
	"github.com/steipete/gogcli/internal/docssed"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsFormatCmd struct {
	DocID     string          `arg:"" name:"docId" help:"Doc ID"`
	Match     string          `name:"match" help:"Only format the first text match"`
	MatchAll  bool            `name:"match-all" help:"Format all matches instead of only the first"`
	MatchCase bool            `name:"match-case" help:"Use case-sensitive matching with --match"`
	Tab       string          `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	Segment   string          `name:"segment" help:"Target an exact header, footer, or footnote segment ID"`
	TabID     string          `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
	Link      string          `name:"link" help:"Set hyperlink target (http://, https://, mailto:, #bookmarkId, or #heading-slug)"`
	NoLink    bool            `name:"no-link" help:"Clear hyperlink"`
	Batch     string          `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
	Format    DocsFormatFlags `embed:""`
}

type DocsFormatFlags struct {
	FontFamily        string   `name:"font-family" help:"Font family, for example Arial or Georgia"`
	FontSize          float64  `name:"font-size" help:"Font size in points"`
	TextColor         string   `name:"text-color" help:"Text color as #RRGGBB or #RGB"`
	BgColor           string   `name:"bg-color" help:"Text background color as #RRGGBB or #RGB"`
	Code              bool     `name:"code" help:"Apply code style (Courier New + grey background)"`
	Bold              bool     `name:"bold" help:"Set bold"`
	NoBold            bool     `name:"no-bold" help:"Clear bold"`
	Italic            bool     `name:"italic" help:"Set italic"`
	NoItalic          bool     `name:"no-italic" help:"Clear italic"`
	Underline         bool     `name:"underline" help:"Set underline"`
	NoUnderline       bool     `name:"no-underline" help:"Clear underline"`
	Strikethrough     bool     `name:"strikethrough" aliases:"strike" help:"Set strikethrough"`
	NoStrike          bool     `name:"no-strikethrough" aliases:"no-strike" help:"Clear strikethrough"`
	Alignment         string   `name:"alignment" help:"Paragraph alignment: left, center, right, justify, start, end, justified"`
	LineSpacing       float64  `name:"line-spacing" help:"Paragraph line spacing percentage, for example 100 or 150"`
	HeadingLevel      *int     `name:"heading-level" help:"Set paragraph named style to HEADING_1..HEADING_6 (shortcut for --named-style=HEADING_N)"`
	NamedStyle        string   `name:"named-style" help:"Set paragraph named style: NORMAL_TEXT, TITLE, SUBTITLE, HEADING_1..HEADING_6"`
	Bullets           bool     `name:"bullets" help:"Create a bulleted list with the default disc preset"`
	Ordered           bool     `name:"ordered" help:"Create a numbered list with the default decimal preset"`
	BulletPreset      string   `name:"bullet-preset" placeholder:"PRESET" help:"Create a list with a Google Docs bullet glyph preset"`
	NoBullets         bool     `name:"no-bullets" help:"Remove bullets or numbering"`
	IndentStart       *float64 `name:"indent-start" placeholder:"PT" help:"Paragraph start indentation in points"`
	IndentFirstLine   *float64 `name:"indent-first-line" placeholder:"PT" help:"Paragraph first-line indentation in points"`
	IndentEnd         *float64 `name:"indent-end" placeholder:"PT" help:"Paragraph end indentation in points"`
	SpaceAbove        *float64 `name:"space-above" placeholder:"PT" help:"Space above the paragraph in points"`
	SpaceBelow        *float64 `name:"space-below" placeholder:"PT" help:"Space below the paragraph in points"`
	KeepWithNext      *bool    `name:"keep-with-next" negatable:"" help:"Keep the paragraph with the next paragraph when possible; use --no-keep-with-next to clear"`
	KeepLinesTogether *bool    `name:"keep-lines-together" negatable:"" help:"Keep all paragraph lines on one page or column when possible; use --no-keep-lines-together to clear"`

	link         string
	noLink       bool
	resolvedLink *docs.Link
}

const (
	docsNamedStyleNormalText = "NORMAL_TEXT"
	docsNamedStyleTitle      = "TITLE"
	docsNamedStyleSubtitle   = "SUBTITLE"
	docsNamedStyleHeading1   = "HEADING_1"
	docsNamedStyleHeading2   = "HEADING_2"
	docsNamedStyleHeading3   = "HEADING_3"
	docsNamedStyleHeading4   = "HEADING_4"
	docsNamedStyleHeading5   = "HEADING_5"
	docsNamedStyleHeading6   = "HEADING_6"
)

func (c *DocsFormatCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	format := c.Format.withLinkFlags(c.Link, c.NoLink)
	if !format.any() {
		return usage("no formatting flags provided")
	}
	if c.MatchAll && strings.TrimSpace(c.Match) == "" {
		return usage("--match-all requires --match")
	}

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab
	if _, err := format.buildRequests(1, 2, c.Tab); err != nil {
		return err
	}

	if err := dryRunExit(ctx, flags, "docs.format", map[string]any{
		"document_id": id,
		"match":       c.Match,
		"match_all":   c.MatchAll,
		"match_case":  c.MatchCase,
		"tab":         c.Tab,
		"segment":     c.Segment,
		"batch":       c.Batch,
		"format": map[string]any{
			"font_family":         c.Format.FontFamily,
			"font_size":           c.Format.FontSize,
			"text_color":          c.Format.TextColor,
			"bg_color":            c.Format.BgColor,
			"link":                c.Link,
			"no_link":             c.NoLink,
			"code":                c.Format.Code,
			"bold":                c.Format.Bold,
			"no_bold":             c.Format.NoBold,
			"italic":              c.Format.Italic,
			"no_italic":           c.Format.NoItalic,
			"underline":           c.Format.Underline,
			"no_underline":        c.Format.NoUnderline,
			"strikethrough":       c.Format.Strikethrough,
			"no_strike":           c.Format.NoStrike,
			"alignment":           c.Format.Alignment,
			"line_spacing":        c.Format.LineSpacing,
			"heading_level":       c.Format.HeadingLevel,
			"named_style":         c.Format.NamedStyle,
			"bullets":             c.Format.Bullets,
			"ordered":             c.Format.Ordered,
			"bullet_preset":       c.Format.BulletPreset,
			"no_bullets":          c.Format.NoBullets,
			"indent_start":        c.Format.IndentStart,
			"indent_first_line":   c.Format.IndentFirstLine,
			"indent_end":          c.Format.IndentEnd,
			"space_above":         c.Format.SpaceAbove,
			"space_below":         c.Format.SpaceBelow,
			"keep_with_next":      c.Format.KeepWithNext,
			"keep_lines_together": c.Format.KeepLinesTogether,
		},
	}); err != nil {
		return err
	}
	if err := validateDocsBatchTarget(ctx, flags, c.Batch, id); err != nil {
		return err
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	batchRevision, err := captureDocsBatchRevision(ctx, svc, c.Batch, id)
	if err != nil {
		return err
	}

	ranges, target, err := c.targetRanges(ctx, svc, id)
	if err != nil {
		return err
	}
	c.Tab = target.TabID
	c.Segment = target.SegmentID
	format, err = format.withResolvedLink(ctx, svc, id, c.Tab)
	if err != nil {
		return err
	}
	if len(ranges) == 0 {
		return usage("no matching text found")
	}
	reqs := make([]*docs.Request, 0, len(ranges)*2)
	if format.createsBullets() {
		// Apply exact-range text styles before bullet creation can remove nesting
		// tabs, then group selected adjacent paragraphs into native list runs.
		textFormat := format.textOnly()
		if textFormat.any() {
			for _, r := range ranges {
				formatReqs, buildErr := textFormat.buildRequests(r.StartIndex, r.EndIndex, target.TabID)
				if buildErr != nil {
					return buildErr
				}
				reqs = append(reqs, formatReqs...)
			}
		}

		paragraphFormat := format.paragraphOnly()
		bulletTargets := groupDocsFormatBulletTargets(ranges)
		adjustDocsFormatBulletTargets(bulletTargets, paragraphFormat.hasParagraphStyle())
		for _, r := range bulletTargets {
			formatReqs, buildErr := paragraphFormat.buildTargetRequests(r, target.TabID)
			if buildErr != nil {
				return buildErr
			}
			reqs = append(reqs, formatReqs...)
		}
	} else {
		for _, r := range ranges {
			formatReqs, buildErr := format.buildTargetRequests(r, target.TabID)
			if buildErr != nil {
				return buildErr
			}
			reqs = append(reqs, formatReqs...)
		}
	}
	applyDocsRequestTarget(reqs, target)
	if queued, queueErr := queueDocsBatchRequests(ctx, flags, c.Batch, id, "docs.format", batchRevision, reqs, false); queued || queueErr != nil {
		return queueErr
	}

	resp, err := svc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{Requests: reqs}).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}

	return c.writeResult(ctx, resp, len(reqs), len(ranges), target)
}

func (c *DocsFormatCmd) targetRanges(ctx context.Context, svc *docs.Service, docID string) ([]docsFormatTargetRange, docsRequestTarget, error) {
	loaded, err := loadDocsTargetSegment(ctx, svc, docID, c.Tab, c.Segment)
	if err != nil {
		return nil, docsRequestTarget{}, err
	}
	targetDoc := loaded.target
	target := docsRequestTarget{TabID: loaded.tabID, SegmentID: loaded.segmentID, SegmentKind: loaded.segmentKind}

	var ranges []docsedit.TextRange
	if strings.TrimSpace(c.Match) == "" {
		start := int64(1)
		if target.SegmentID != "" {
			start = 0
		}
		end := docsDocumentEndIndex(targetDoc) - 1
		if end > start {
			ranges = []docsedit.TextRange{{StartIndex: start, EndIndex: end}}
		}
	} else {
		ranges = docsedit.FindTextRanges(targetDoc, c.Match, docsedit.SearchOptions{
			MatchCase:            c.MatchCase,
			PreserveHTMLEntities: true,
			RequireTextSegment:   true,
		})
		if !c.MatchAll && len(ranges) > 1 {
			ranges = ranges[:1]
		}
	}

	paragraphs := []docssed.DocumentParagraph(nil)
	if projection := docssed.ProjectDocument(targetDoc); projection.Legacy != nil {
		paragraphs = projection.Legacy.Paragraphs
	}
	targets := make([]docsFormatTargetRange, 0, len(ranges))
	for _, r := range ranges {
		target := docsFormatTargetRange{TextRange: r}
		if c.Format.createsBullets() {
			target.BulletParagraphs = docsFormatBulletParagraphs(paragraphs, r.StartIndex, r.EndIndex)
		}
		targets = append(targets, target)
	}
	return targets, target, nil
}

type docsFormatTargetRange struct {
	docsedit.TextRange
	PostBulletStart  int64
	PostBulletEnd    int64
	BulletParagraphs []docsFormatBulletParagraph
}

type docsFormatBulletParagraph struct {
	StartIndex  int64
	EndIndex    int64
	LeadingTabs int64
}

func docsFormatBulletParagraphs(paragraphs []docssed.DocumentParagraph, start, end int64) []docsFormatBulletParagraph {
	var affected []docsFormatBulletParagraph
	for _, paragraph := range paragraphs {
		if paragraph.StartIndex >= end || paragraph.EndIndex <= start {
			continue
		}
		item := docsFormatBulletParagraph{StartIndex: paragraph.StartIndex, EndIndex: paragraph.EndIndex}
		for _, r := range paragraph.Text {
			if r != '\t' {
				break
			}
			item.LeadingTabs++
		}
		affected = append(affected, item)
	}
	return affected
}

func adjustDocsFormatBulletTargets(targets []docsFormatTargetRange, withParagraphStyle bool) {
	seenParagraphs := make(map[int64]bool)
	var removedTabIndexes []int64
	shiftedIndex := func(index int64) int64 {
		shift := int64(0)
		for _, removed := range removedTabIndexes {
			if removed < index {
				shift++
			}
		}
		return index - shift
	}

	for i := range targets {
		target := &targets[i]
		target.StartIndex = shiftedIndex(target.StartIndex)
		target.EndIndex = shiftedIndex(target.EndIndex)

		for _, paragraph := range target.BulletParagraphs {
			if seenParagraphs[paragraph.StartIndex] {
				continue
			}
			seenParagraphs[paragraph.StartIndex] = true
			for offset := int64(0); offset < paragraph.LeadingTabs; offset++ {
				removedTabIndexes = append(removedTabIndexes, paragraph.StartIndex+offset)
			}
		}
		if !withParagraphStyle || len(target.BulletParagraphs) == 0 {
			continue
		}

		target.PostBulletStart = shiftedIndex(target.BulletParagraphs[0].StartIndex)
		target.PostBulletEnd = shiftedIndex(target.BulletParagraphs[len(target.BulletParagraphs)-1].EndIndex)
	}
}

func groupDocsFormatBulletTargets(targets []docsFormatTargetRange) []docsFormatTargetRange {
	seen := make(map[int64]bool)
	var groups []docsFormatTargetRange
	for _, target := range targets {
		for _, paragraph := range target.BulletParagraphs {
			if seen[paragraph.StartIndex] {
				continue
			}
			seen[paragraph.StartIndex] = true

			last := len(groups) - 1
			if last >= 0 && groups[last].BulletParagraphs[len(groups[last].BulletParagraphs)-1].EndIndex == paragraph.StartIndex {
				groups[last].EndIndex = paragraph.EndIndex
				groups[last].BulletParagraphs = append(groups[last].BulletParagraphs, paragraph)
				continue
			}
			groups = append(groups, docsFormatTargetRange{
				TextRange:        docsedit.TextRange{StartIndex: paragraph.StartIndex, EndIndex: paragraph.EndIndex},
				BulletParagraphs: []docsFormatBulletParagraph{paragraph},
			})
		}
	}
	return groups
}

func (c *DocsFormatCmd) writeResult(ctx context.Context, resp *docs.BatchUpdateDocumentResponse, requestCount, rangeCount int, target docsRequestTarget) error {
	u := ui.FromContext(ctx)
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": resp.DocumentId,
			"requests":   requestCount,
			"ranges":     rangeCount,
		}
		if target.TabID != "" {
			payload["tabId"] = target.TabID
		}
		if target.SegmentID != "" {
			payload["segmentId"] = target.SegmentID
			payload["segmentType"] = target.SegmentKind
		}
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("id\t%s", resp.DocumentId)
	u.Out().Linef("requests\t%d", requestCount)
	u.Out().Linef("ranges\t%d", rangeCount)
	if target.TabID != "" {
		u.Out().Linef("tabId\t%s", target.TabID)
	}
	if target.SegmentID != "" {
		u.Out().Linef("segmentId\t%s", target.SegmentID)
		u.Out().Linef("segmentType\t%s", target.SegmentKind)
	}
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Linef("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

func (f DocsFormatFlags) any() bool {
	return f.options().Any()
}

func (f DocsFormatFlags) createsBullets() bool {
	return f.Bullets || f.Ordered || strings.TrimSpace(f.BulletPreset) != ""
}

func (f DocsFormatFlags) hasParagraphStyle() bool {
	return strings.TrimSpace(f.Alignment) != "" || f.LineSpacing != 0 || f.HeadingLevel != nil ||
		strings.TrimSpace(f.NamedStyle) != "" || f.IndentStart != nil || f.IndentFirstLine != nil ||
		f.IndentEnd != nil || f.SpaceAbove != nil || f.SpaceBelow != nil ||
		f.KeepWithNext != nil || f.KeepLinesTogether != nil
}

func (f DocsFormatFlags) textOnly() DocsFormatFlags {
	return DocsFormatFlags{
		FontFamily:    f.FontFamily,
		FontSize:      f.FontSize,
		TextColor:     f.TextColor,
		BgColor:       f.BgColor,
		Code:          f.Code,
		Bold:          f.Bold,
		NoBold:        f.NoBold,
		Italic:        f.Italic,
		NoItalic:      f.NoItalic,
		Underline:     f.Underline,
		NoUnderline:   f.NoUnderline,
		Strikethrough: f.Strikethrough,
		NoStrike:      f.NoStrike,
		link:          f.link,
		noLink:        f.noLink,
		resolvedLink:  f.resolvedLink,
	}
}

func (f DocsFormatFlags) paragraphOnly() DocsFormatFlags {
	return DocsFormatFlags{
		Alignment:         f.Alignment,
		LineSpacing:       f.LineSpacing,
		HeadingLevel:      f.HeadingLevel,
		NamedStyle:        f.NamedStyle,
		Bullets:           f.Bullets,
		Ordered:           f.Ordered,
		BulletPreset:      f.BulletPreset,
		IndentStart:       f.IndentStart,
		IndentFirstLine:   f.IndentFirstLine,
		IndentEnd:         f.IndentEnd,
		SpaceAbove:        f.SpaceAbove,
		SpaceBelow:        f.SpaceBelow,
		KeepWithNext:      f.KeepWithNext,
		KeepLinesTogether: f.KeepLinesTogether,
	}
}

func (f DocsFormatFlags) buildRequests(start, end int64, tabID string) ([]*docs.Request, error) {
	requests, err := docsformat.BuildRequests(f.options(), start, end, tabID)
	if err != nil {
		return nil, usage(err.Error())
	}
	return requests, nil
}

func (f DocsFormatFlags) buildTargetRequests(target docsFormatTargetRange, tabID string) ([]*docs.Request, error) {
	options := f.options()
	options.PostBulletParagraphStart = target.PostBulletStart
	options.PostBulletParagraphEnd = target.PostBulletEnd
	requests, err := docsformat.BuildRequests(options, target.StartIndex, target.EndIndex, tabID)
	if err != nil {
		return nil, usage(err.Error())
	}
	return requests, nil
}

func (f DocsFormatFlags) options() docsformat.Options {
	return docsformat.Options{
		FontFamily:        f.FontFamily,
		FontSize:          f.FontSize,
		TextColor:         f.TextColor,
		Background:        f.BgColor,
		Link:              f.link,
		ClearLink:         f.noLink,
		ResolvedLink:      f.resolvedLink,
		Code:              f.Code,
		Bold:              f.Bold,
		ClearBold:         f.NoBold,
		Italic:            f.Italic,
		ClearItalic:       f.NoItalic,
		Underline:         f.Underline,
		ClearUnderline:    f.NoUnderline,
		Strikethrough:     f.Strikethrough,
		ClearStrike:       f.NoStrike,
		Alignment:         f.Alignment,
		LineSpacing:       f.LineSpacing,
		HeadingLevel:      f.HeadingLevel,
		NamedStyle:        f.NamedStyle,
		Bullets:           f.Bullets,
		Ordered:           f.Ordered,
		BulletPreset:      f.BulletPreset,
		ClearBullets:      f.NoBullets,
		IndentStart:       f.IndentStart,
		IndentFirstLine:   f.IndentFirstLine,
		IndentEnd:         f.IndentEnd,
		SpaceAbove:        f.SpaceAbove,
		SpaceBelow:        f.SpaceBelow,
		KeepWithNext:      f.KeepWithNext,
		KeepLinesTogether: f.KeepLinesTogether,
	}
}

func docsFormatColor(hex, flag string) (*docs.OptionalColor, error) {
	color, err := docsformat.Color(hex, flag)
	if err != nil {
		return nil, usage(err.Error())
	}
	return color, nil
}

func (f DocsFormatFlags) withLinkFlags(link string, noLink bool) DocsFormatFlags {
	f.link = link
	f.noLink = noLink
	return f
}

func (f DocsFormatFlags) withResolvedLink(ctx context.Context, svc *docs.Service, docID, tab string) (DocsFormatFlags, error) {
	link := strings.TrimSpace(f.link)
	if link == "" || f.noLink || !strings.HasPrefix(link, "#") {
		return f, nil
	}
	target := strings.TrimSpace(strings.TrimPrefix(link, "#"))
	if target == "" {
		return f, usage("--link target cannot be empty")
	}
	getCall := svc.Documents.Get(docID).Context(ctx)
	if tab != "" {
		getCall = getCall.IncludeTabsContent(true)
	}
	doc, err := getCall.Do()
	if err != nil {
		if isDocsNotFound(err) {
			return f, fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return f, err
	}
	f.resolvedLink = docsFormatInternalLink(doc, tab, target)
	return f, nil
}

func docsFormatInternalLink(doc *docs.Document, tab, target string) *docs.Link {
	content, resolvedTabID, err := markdownHeadingLinkContent(doc, tab)
	if err == nil && len(content) > 0 {
		_, autoHeadingBySlug, explicitHeadingBySlug := markdownHeadingLinkTargets(content, resolvedTabID, nil, 0, 0)
		if heading, ok := explicitHeadingBySlug[target]; ok {
			return docsFormatHeadingLink(heading)
		}
		if heading, ok := autoHeadingBySlug[target]; ok {
			return docsFormatHeadingLink(heading)
		}
	}
	if strings.HasPrefix(target, "h.") {
		return docsFormatHeadingLink(markdownHeadingTarget{headingID: target, tabID: resolvedTabID})
	}
	return &docs.Link{BookmarkId: target}
}

func docsFormatHeadingLink(target markdownHeadingTarget) *docs.Link {
	if target.tabID != "" {
		return &docs.Link{Heading: &docs.HeadingLink{Id: target.headingID, TabId: target.tabID}}
	}
	return &docs.Link{HeadingId: target.headingID}
}
