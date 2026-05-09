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
	Format    DocsFormatFlags `embed:""`
}

type DocsFormatFlags struct {
	FontFamily    string  `name:"font-family" help:"Font family, for example Arial or Georgia"`
	FontSize      float64 `name:"font-size" help:"Font size in points"`
	TextColor     string  `name:"text-color" help:"Text color as #RRGGBB or #RGB"`
	BgColor       string  `name:"bg-color" help:"Text background color as #RRGGBB or #RGB"`
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
}

func (c *DocsFormatCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	if !c.Format.any() {
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

	svc, err := requireDocsService(ctx, flags)
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
		formatReqs, buildErr := c.Format.buildRequests(r.startIndex, r.endIndex, tabID)
		if buildErr != nil {
			return buildErr
		}
		reqs = append(reqs, formatReqs...)
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

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("requests\t%d", requestCount)
	u.Out().Printf("ranges\t%d", rangeCount)
	if tabID != "" {
		u.Out().Printf("tabId\t%s", tabID)
	}
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

func (f DocsFormatFlags) any() bool {
	return strings.TrimSpace(f.FontFamily) != "" ||
		f.FontSize != 0 ||
		strings.TrimSpace(f.TextColor) != "" ||
		strings.TrimSpace(f.BgColor) != "" ||
		f.Bold || f.NoBold ||
		f.Italic || f.NoItalic ||
		f.Underline || f.NoUnderline ||
		f.Strikethrough || f.NoStrike ||
		strings.TrimSpace(f.Alignment) != "" ||
		f.LineSpacing != 0
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
	if len(fields) == 0 {
		return nil, false, nil
	}
	return &docs.Request{UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
		Range:          &docs.Range{StartIndex: start, EndIndex: end, TabId: tabID},
		ParagraphStyle: style,
		Fields:         strings.Join(fields, ","),
	}}, true, nil
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
