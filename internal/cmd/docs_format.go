package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsFormatCmd struct {
	DocID     string          `arg:"" name:"docId" help:"Doc ID"`
	Match     string          `name:"match" help:"Only format the first text match"`
	MatchAll  bool            `name:"match-all" help:"Format all matches instead of only the first"`
	MatchCase bool            `name:"match-case" help:"Use case-sensitive matching with --match"`
	Tab       string          `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID     string          `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
	Link      string          `name:"link" help:"Set hyperlink target (http://, https://, mailto:, #bookmarkId, or #heading-slug)"`
	NoLink    bool            `name:"no-link" help:"Clear hyperlink"`
	Batch     string          `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
	Format    DocsFormatFlags `embed:""`
}

type DocsFormatFlags struct {
	FontFamily    string  `name:"font-family" help:"Font family, for example Arial or Georgia"`
	FontSize      float64 `name:"font-size" help:"Font size in points"`
	TextColor     string  `name:"text-color" help:"Text color as #RRGGBB or #RGB"`
	BgColor       string  `name:"bg-color" help:"Text background color as #RRGGBB or #RGB"`
	Code          bool    `name:"code" help:"Apply code style (Courier New + grey background)"`
	Bold          bool    `name:"bold" help:"Set bold"`
	NoBold        bool    `name:"no-bold" help:"Clear bold"`
	Italic        bool    `name:"italic" help:"Set italic"`
	NoItalic      bool    `name:"no-italic" help:"Clear italic"`
	Underline     bool    `name:"underline" help:"Set underline"`
	NoUnderline   bool    `name:"no-underline" help:"Clear underline"`
	Strikethrough bool    `name:"strikethrough" aliases:"strike" help:"Set strikethrough"`
	NoStrike      bool    `name:"no-strikethrough" aliases:"no-strike" help:"Clear strikethrough"`
	Alignment     string  `name:"alignment" help:"Paragraph alignment: left, center, right, justify, start, end, justified"`
	LineSpacing   float64 `name:"line-spacing" help:"Paragraph line spacing percentage, for example 100 or 150"`
	HeadingLevel  *int    `name:"heading-level" help:"Set paragraph named style to HEADING_1..HEADING_6 (shortcut for --named-style=HEADING_N)"`
	NamedStyle    string  `name:"named-style" help:"Set paragraph named style: NORMAL_TEXT, TITLE, SUBTITLE, HEADING_1..HEADING_6"`

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
		"batch":       c.Batch,
		"format": map[string]any{
			"font_family":   c.Format.FontFamily,
			"font_size":     c.Format.FontSize,
			"text_color":    c.Format.TextColor,
			"bg_color":      c.Format.BgColor,
			"link":          c.Link,
			"no_link":       c.NoLink,
			"code":          c.Format.Code,
			"bold":          c.Format.Bold,
			"no_bold":       c.Format.NoBold,
			"italic":        c.Format.Italic,
			"no_italic":     c.Format.NoItalic,
			"underline":     c.Format.Underline,
			"no_underline":  c.Format.NoUnderline,
			"strikethrough": c.Format.Strikethrough,
			"no_strike":     c.Format.NoStrike,
			"alignment":     c.Format.Alignment,
			"line_spacing":  c.Format.LineSpacing,
			"heading_level": c.Format.HeadingLevel,
			"named_style":   c.Format.NamedStyle,
		},
	}); err != nil {
		return err
	}
	if err := validateDocsBatchTarget(flags, c.Batch, id); err != nil {
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

	format, err = format.withResolvedLink(ctx, svc, id, c.Tab)
	if err != nil {
		return err
	}

	ranges, tabID, err := c.targetRanges(ctx, svc, id)
	if err != nil {
		return err
	}
	if len(ranges) == 0 {
		return usage("no matching text found")
	}

	reqs := make([]*docs.Request, 0, len(ranges)*2)
	for _, r := range ranges {
		formatReqs, buildErr := format.buildRequests(r.startIndex, r.endIndex, tabID)
		if buildErr != nil {
			return buildErr
		}
		reqs = append(reqs, formatReqs...)
	}
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

	return c.writeResult(ctx, resp, len(reqs), len(ranges), tabID)
}

func (c *DocsFormatCmd) targetRanges(ctx context.Context, svc *docs.Service, docID string) ([]docRange, string, error) {
	if strings.TrimSpace(c.Match) == "" {
		endIndex, tabID, err := docsTargetEndIndexAndTabID(ctx, svc, docID, c.Tab)
		if err != nil {
			return nil, "", err
		}
		end := endIndex - 1
		if end <= 1 {
			return nil, tabID, nil
		}
		return []docRange{{startIndex: 1, endIndex: end}}, tabID, nil
	}

	getCall := svc.Documents.Get(docID).Context(ctx)
	if c.Tab != "" {
		getCall = getCall.IncludeTabsContent(true)
	}
	doc, err := getCall.Do()
	if err != nil {
		if isDocsNotFound(err) {
			return nil, "", fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return nil, "", err
	}

	tabID := ""
	targetDoc := doc
	if c.Tab != "" {
		tab, tabErr := findTab(flattenTabs(doc.Tabs), c.Tab)
		if tabErr != nil {
			return nil, "", tabErr
		}
		if tab.TabProperties != nil {
			tabID = tab.TabProperties.TabId
		}
		targetDoc = &docs.Document{}
		if tab.DocumentTab != nil {
			targetDoc.Body = tab.DocumentTab.Body
		}
	}

	matches := findTextMatches(targetDoc, c.Match, c.MatchCase)
	if !c.MatchAll && len(matches) > 1 {
		matches = matches[:1]
	}
	return matches, tabID, nil
}

func (c *DocsFormatCmd) writeResult(ctx context.Context, resp *docs.BatchUpdateDocumentResponse, requestCount, rangeCount int, tabID string) error {
	u := ui.FromContext(ctx)
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": resp.DocumentId,
			"requests":   requestCount,
			"ranges":     rangeCount,
		}
		if tabID != "" {
			payload["tabId"] = tabID
		}
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Linef("id\t%s", resp.DocumentId)
	u.Out().Linef("requests\t%d", requestCount)
	u.Out().Linef("ranges\t%d", rangeCount)
	if tabID != "" {
		u.Out().Linef("tabId\t%s", tabID)
	}
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Linef("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

func (f DocsFormatFlags) any() bool {
	return strings.TrimSpace(f.FontFamily) != "" ||
		f.FontSize != 0 ||
		strings.TrimSpace(f.TextColor) != "" ||
		strings.TrimSpace(f.BgColor) != "" ||
		strings.TrimSpace(f.link) != "" ||
		f.noLink ||
		f.Code ||
		f.Bold || f.NoBold ||
		f.Italic || f.NoItalic ||
		f.Underline || f.NoUnderline ||
		f.Strikethrough || f.NoStrike ||
		strings.TrimSpace(f.Alignment) != "" ||
		f.LineSpacing != 0 ||
		f.HeadingLevel != nil ||
		strings.TrimSpace(f.NamedStyle) != ""
}

func (f DocsFormatFlags) buildRequests(start, end int64, tabID string) ([]*docs.Request, error) {
	if start <= 0 || end <= start {
		return nil, fmt.Errorf("invalid format range: %d..%d", start, end)
	}
	textReq, ok, err := f.buildTextStyleRequest(start, end, tabID)
	if err != nil {
		return nil, err
	}
	paraReq, paraOK, err := f.buildParagraphStyleRequest(start, end, tabID)
	if err != nil {
		return nil, err
	}
	reqs := make([]*docs.Request, 0, 2)
	if ok {
		reqs = append(reqs, textReq)
	}
	if paraOK {
		reqs = append(reqs, paraReq)
	}
	if len(reqs) == 0 {
		return nil, usage("no formatting flags provided")
	}
	return reqs, nil
}

func (f DocsFormatFlags) buildTextStyleRequest(start, end int64, tabID string) (*docs.Request, bool, error) {
	style := &docs.TextStyle{}
	var fields []string

	if f.Code {
		if strings.TrimSpace(f.FontFamily) != "" {
			return nil, false, usage("--code cannot be combined with --font-family")
		}
		if strings.TrimSpace(f.BgColor) != "" {
			return nil, false, usage("--code cannot be combined with --bg-color")
		}
		style.WeightedFontFamily = &docs.WeightedFontFamily{FontFamily: "Courier New"}
		style.BackgroundColor = greyColor(codeBackgroundGrey)
		fields = append(fields, "weightedFontFamily", "backgroundColor")
	}
	if font := strings.TrimSpace(f.FontFamily); font != "" {
		style.WeightedFontFamily = &docs.WeightedFontFamily{FontFamily: font}
		fields = append(fields, "weightedFontFamily")
	}
	if f.FontSize < 0 {
		return nil, false, usage("--font-size must be positive")
	}
	if f.FontSize > 0 {
		style.FontSize = &docs.Dimension{Magnitude: f.FontSize, Unit: "PT"}
		fields = append(fields, "fontSize")
	}
	if color := strings.TrimSpace(f.TextColor); color != "" {
		optionalColor, err := docsFormatColor(color, "--text-color")
		if err != nil {
			return nil, false, err
		}
		style.ForegroundColor = optionalColor
		fields = append(fields, "foregroundColor")
	}
	if color := strings.TrimSpace(f.BgColor); color != "" {
		optionalColor, err := docsFormatColor(color, "--bg-color")
		if err != nil {
			return nil, false, err
		}
		style.BackgroundColor = optionalColor
		fields = append(fields, "backgroundColor")
	}
	if strings.TrimSpace(f.link) != "" && f.noLink {
		return nil, false, usage("--link and --no-link cannot be combined")
	}
	if link := strings.TrimSpace(f.link); link != "" {
		resolved := f.resolvedLink
		if resolved == nil {
			var err error
			resolved, err = docsFormatLink(link)
			if err != nil {
				return nil, false, err
			}
		}
		style.Link = resolved
		fields = append(fields, "link")
	}
	if f.noLink {
		style.NullFields = append(style.NullFields, "Link")
		fields = append(fields, "link")
	}
	addBoolStyle := func(set, unset bool, field, forceField string, apply func(bool)) error {
		if set && unset {
			return usage(fmt.Sprintf("--%s and --no-%s cannot be combined", field, field))
		}
		if set || unset {
			apply(set)
			fields = append(fields, field)
			if unset {
				style.ForceSendFields = append(style.ForceSendFields, forceField)
			}
		}
		return nil
	}
	if err := addBoolStyle(f.Bold, f.NoBold, "bold", "Bold", func(v bool) { style.Bold = v }); err != nil {
		return nil, false, err
	}
	if err := addBoolStyle(f.Italic, f.NoItalic, "italic", "Italic", func(v bool) { style.Italic = v }); err != nil {
		return nil, false, err
	}
	if err := addBoolStyle(f.Underline, f.NoUnderline, "underline", "Underline", func(v bool) { style.Underline = v }); err != nil {
		return nil, false, err
	}
	if err := addBoolStyle(f.Strikethrough, f.NoStrike, "strikethrough", "Strikethrough", func(v bool) { style.Strikethrough = v }); err != nil {
		return nil, false, err
	}

	if len(fields) == 0 {
		return nil, false, nil
	}
	return &docs.Request{UpdateTextStyle: &docs.UpdateTextStyleRequest{
		Range:     &docs.Range{StartIndex: start, EndIndex: end, TabId: tabID},
		TextStyle: style,
		Fields:    strings.Join(fields, ","),
	}}, true, nil
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

func docsFormatLink(value string) (*docs.Link, error) {
	link := strings.TrimSpace(value)
	if link == "" {
		return nil, usage("--link target cannot be empty")
	}
	if target, ok := strings.CutPrefix(link, "#"); ok {
		target = strings.TrimSpace(target)
		if target == "" {
			return nil, usage("--link target cannot be empty")
		}
		return &docs.Link{BookmarkId: target}, nil
	}
	return &docs.Link{Url: link}, nil
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

func (f DocsFormatFlags) buildParagraphStyleRequest(start, end int64, tabID string) (*docs.Request, bool, error) {
	style := &docs.ParagraphStyle{}
	var fields []string

	if align := strings.TrimSpace(f.Alignment); align != "" {
		resolved, err := docsFormatAlignment(align)
		if err != nil {
			return nil, false, err
		}
		style.Alignment = resolved
		fields = append(fields, "alignment")
	}
	if f.LineSpacing < 0 {
		return nil, false, usage("--line-spacing must be positive")
	}
	if f.LineSpacing > 0 {
		style.LineSpacing = f.LineSpacing
		fields = append(fields, "lineSpacing")
	}
	namedStyle, err := docsFormatNamedStyle(f.HeadingLevel, f.NamedStyle)
	if err != nil {
		return nil, false, err
	}
	if namedStyle != "" {
		style.NamedStyleType = namedStyle
		fields = append(fields, "namedStyleType")
	}
	if len(fields) == 0 {
		return nil, false, nil
	}
	return &docs.Request{UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
		Range:          &docs.Range{StartIndex: start, EndIndex: end, TabId: tabID},
		ParagraphStyle: style,
		Fields:         strings.Join(fields, ","),
	}}, true, nil
}

// docsFormatNamedStyle resolves the named paragraph style requested via
// --heading-level (1..6 shortcut) and/or --named-style (full enum). The two
// are mutually exclusive — supplying both is a usage error. Returns "" when
// neither was supplied.
func docsFormatNamedStyle(headingLevel *int, namedStyle string) (string, error) {
	trimmed := strings.ToUpper(strings.TrimSpace(namedStyle))
	if headingLevel != nil && trimmed != "" {
		return "", usage("--heading-level and --named-style cannot be combined")
	}
	if headingLevel != nil {
		if *headingLevel < 1 || *headingLevel > 6 {
			return "", usage("--heading-level must be between 1 and 6")
		}
		return fmt.Sprintf("HEADING_%d", *headingLevel), nil
	}
	if trimmed == "" {
		return "", nil
	}
	switch trimmed {
	case docsNamedStyleNormalText, docsNamedStyleTitle, docsNamedStyleSubtitle,
		docsNamedStyleHeading1, docsNamedStyleHeading2, docsNamedStyleHeading3,
		docsNamedStyleHeading4, docsNamedStyleHeading5, docsNamedStyleHeading6:
		return trimmed, nil
	default:
		return "", usage("--named-style must be one of NORMAL_TEXT, TITLE, SUBTITLE, HEADING_1..HEADING_6")
	}
}

func docsFormatColor(hex, flag string) (*docs.OptionalColor, error) {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return nil, usage(fmt.Sprintf("%s must be #RRGGBB or #RGB", flag))
	}
	return &docs.OptionalColor{Color: &docs.Color{RgbColor: &docs.RgbColor{Red: r, Green: g, Blue: b}}}, nil
}

func docsFormatAlignment(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "left", "start":
		return "START", nil
	case "center", "centre":
		return "CENTER", nil
	case "right", "end":
		return "END", nil
	case "justify", "justified":
		return "JUSTIFIED", nil
	default:
		return "", usage("--alignment must be left, center, right, justify, start, end, or justified")
	}
}
