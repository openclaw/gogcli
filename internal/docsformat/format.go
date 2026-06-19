package docsformat

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"
)

const codeBackgroundGrey = 0.95

const (
	BulletPresetDisc     = "BULLET_DISC_CIRCLE_SQUARE"
	BulletPresetNumbered = "NUMBERED_DECIMAL_ALPHA_ROMAN"
)

const (
	namedStyleNormalText = "NORMAL_TEXT"
	namedStyleTitle      = "TITLE"
	namedStyleSubtitle   = "SUBTITLE"
	namedStyleHeading1   = "HEADING_1"
	namedStyleHeading2   = "HEADING_2"
	namedStyleHeading3   = "HEADING_3"
	namedStyleHeading4   = "HEADING_4"
	namedStyleHeading5   = "HEADING_5"
	namedStyleHeading6   = "HEADING_6"
)

type ValidationError string

func (e ValidationError) Error() string {
	return string(e)
}

type Options struct {
	FontFamily        string
	FontSize          float64
	TextColor         string
	Background        string
	Link              string
	ClearLink         bool
	ResolvedLink      *docs.Link
	Code              bool
	Bold              bool
	ClearBold         bool
	Italic            bool
	ClearItalic       bool
	Underline         bool
	ClearUnderline    bool
	Strikethrough     bool
	ClearStrike       bool
	Alignment         string
	LineSpacing       float64
	HeadingLevel      *int
	NamedStyle        string
	Bullets           bool
	Ordered           bool
	BulletPreset      string
	ClearBullets      bool
	IndentStart       *float64
	IndentFirstLine   *float64
	IndentEnd         *float64
	SpaceAbove        *float64
	SpaceBelow        *float64
	KeepWithNext      *bool
	KeepLinesTogether *bool
	// PostBulletParagraphStart/End identify the same affected paragraphs after
	// CreateParagraphBullets removes any leading nesting tabs.
	PostBulletParagraphStart int64
	PostBulletParagraphEnd   int64
}

func (o Options) Any() bool {
	return strings.TrimSpace(o.FontFamily) != "" ||
		o.FontSize != 0 ||
		strings.TrimSpace(o.TextColor) != "" ||
		strings.TrimSpace(o.Background) != "" ||
		strings.TrimSpace(o.Link) != "" ||
		o.ClearLink ||
		o.Code ||
		o.Bold || o.ClearBold ||
		o.Italic || o.ClearItalic ||
		o.Underline || o.ClearUnderline ||
		o.Strikethrough || o.ClearStrike ||
		strings.TrimSpace(o.Alignment) != "" ||
		o.LineSpacing != 0 ||
		o.HeadingLevel != nil ||
		strings.TrimSpace(o.NamedStyle) != "" ||
		o.Bullets || o.Ordered || strings.TrimSpace(o.BulletPreset) != "" || o.ClearBullets ||
		o.IndentStart != nil || o.IndentFirstLine != nil || o.IndentEnd != nil ||
		o.SpaceAbove != nil || o.SpaceBelow != nil ||
		o.KeepWithNext != nil || o.KeepLinesTogether != nil
}

func BuildRequests(options Options, start, end int64, tabID string) ([]*docs.Request, error) {
	if start <= 0 || end <= start {
		return nil, invalidf("invalid format range: %d..%d", start, end)
	}

	textReq, hasText, err := buildTextStyleRequest(options, start, end, tabID)
	if err != nil {
		return nil, err
	}

	paragraphStart, paragraphEnd := start, end

	hasPostBulletRange := options.PostBulletParagraphStart > 0 && options.PostBulletParagraphEnd > options.PostBulletParagraphStart
	if hasPostBulletRange {
		paragraphStart = options.PostBulletParagraphStart
		paragraphEnd = options.PostBulletParagraphEnd
	}

	paragraphReq, hasParagraph, err := buildParagraphStyleRequest(options, paragraphStart, paragraphEnd, tabID)
	if err != nil {
		return nil, err
	}

	bulletReq, hasBullets, err := buildParagraphBulletsRequest(options, start, end, tabID)
	if err != nil {
		return nil, err
	}

	requests := make([]*docs.Request, 0, 3)
	if hasText {
		requests = append(requests, textReq)
	}

	// Removing bullets preserves list nesting by adding paragraph indentation.
	// Apply explicit paragraph controls afterward so the caller's values win.
	if hasBullets && (options.ClearBullets || hasPostBulletRange) {
		requests = append(requests, bulletReq)
	}

	if hasParagraph {
		requests = append(requests, paragraphReq)
	}

	if hasBullets && !options.ClearBullets && !hasPostBulletRange {
		requests = append(requests, bulletReq)
	}

	if len(requests) == 0 {
		return nil, ValidationError("no formatting flags provided")
	}

	return requests, nil
}

func buildTextStyleRequest(options Options, start, end int64, tabID string) (*docs.Request, bool, error) {
	style := &docs.TextStyle{}
	var fields []string

	if options.Code {
		if strings.TrimSpace(options.FontFamily) != "" {
			return nil, false, ValidationError("--code cannot be combined with --font-family")
		}

		if strings.TrimSpace(options.Background) != "" {
			return nil, false, ValidationError("--code cannot be combined with --bg-color")
		}
		style.WeightedFontFamily = &docs.WeightedFontFamily{FontFamily: "Courier New"}
		style.BackgroundColor = greyColor(codeBackgroundGrey)

		fields = append(fields, "weightedFontFamily", "backgroundColor")
	}

	if font := strings.TrimSpace(options.FontFamily); font != "" {
		style.WeightedFontFamily = &docs.WeightedFontFamily{FontFamily: font}

		fields = append(fields, "weightedFontFamily")
	}

	if options.FontSize < 0 {
		return nil, false, ValidationError("--font-size must be positive")
	}

	if options.FontSize > 0 {
		style.FontSize = &docs.Dimension{Magnitude: options.FontSize, Unit: "PT"}

		fields = append(fields, "fontSize")
	}

	if color := strings.TrimSpace(options.TextColor); color != "" {
		optionalColor, err := Color(color, "--text-color")
		if err != nil {
			return nil, false, err
		}
		style.ForegroundColor = optionalColor

		fields = append(fields, "foregroundColor")
	}

	if color := strings.TrimSpace(options.Background); color != "" {
		optionalColor, err := Color(color, "--bg-color")
		if err != nil {
			return nil, false, err
		}
		style.BackgroundColor = optionalColor

		fields = append(fields, "backgroundColor")
	}

	if strings.TrimSpace(options.Link) != "" && options.ClearLink {
		return nil, false, ValidationError("--link and --no-link cannot be combined")
	}

	if link := strings.TrimSpace(options.Link); link != "" {
		resolved := options.ResolvedLink
		if resolved == nil {
			var err error

			resolved, err = formatLink(link)
			if err != nil {
				return nil, false, err
			}
		}
		style.Link = resolved

		fields = append(fields, "link")
	}

	if options.ClearLink {
		style.NullFields = append(style.NullFields, "Link")
		fields = append(fields, "link")
	}

	addBoolStyle := func(set, unset bool, field, forceField string, apply func(bool)) error {
		if set && unset {
			return invalidf("--%s and --no-%s cannot be combined", field, field)
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
	if err := addBoolStyle(options.Bold, options.ClearBold, "bold", "Bold", func(v bool) { style.Bold = v }); err != nil {
		return nil, false, err
	}

	if err := addBoolStyle(options.Italic, options.ClearItalic, "italic", "Italic", func(v bool) { style.Italic = v }); err != nil {
		return nil, false, err
	}

	if err := addBoolStyle(options.Underline, options.ClearUnderline, "underline", "Underline", func(v bool) { style.Underline = v }); err != nil {
		return nil, false, err
	}

	if err := addBoolStyle(options.Strikethrough, options.ClearStrike, "strikethrough", "Strikethrough", func(v bool) { style.Strikethrough = v }); err != nil {
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

func buildParagraphStyleRequest(options Options, start, end int64, tabID string) (*docs.Request, bool, error) {
	style := &docs.ParagraphStyle{}
	var fields []string

	if align := strings.TrimSpace(options.Alignment); align != "" {
		resolved, err := formatAlignment(align)
		if err != nil {
			return nil, false, err
		}
		style.Alignment = resolved

		fields = append(fields, "alignment")
	}

	if options.LineSpacing < 0 {
		return nil, false, ValidationError("--line-spacing must be positive")
	}

	if options.LineSpacing > 0 {
		style.LineSpacing = options.LineSpacing

		fields = append(fields, "lineSpacing")
	}

	namedStyle, err := formatNamedStyle(options.HeadingLevel, options.NamedStyle)
	if err != nil {
		return nil, false, err
	}

	if namedStyle != "" {
		style.NamedStyleType = namedStyle

		fields = append(fields, "namedStyleType")
	}

	addDimension := func(value *float64, flag, field string, apply func(*docs.Dimension)) error {
		if value == nil {
			return nil
		}

		if *value < 0 {
			return invalidf("--%s must be non-negative", flag)
		}

		dimension := &docs.Dimension{Magnitude: *value, Unit: "PT"}
		if *value == 0 {
			dimension.ForceSendFields = append(dimension.ForceSendFields, "Magnitude")
		}

		apply(dimension)

		fields = append(fields, field)

		return nil
	}
	if err := addDimension(options.IndentStart, "indent-start", "indentStart", func(v *docs.Dimension) { style.IndentStart = v }); err != nil {
		return nil, false, err
	}

	if err := addDimension(options.IndentFirstLine, "indent-first-line", "indentFirstLine", func(v *docs.Dimension) { style.IndentFirstLine = v }); err != nil {
		return nil, false, err
	}

	if err := addDimension(options.IndentEnd, "indent-end", "indentEnd", func(v *docs.Dimension) { style.IndentEnd = v }); err != nil {
		return nil, false, err
	}

	if err := addDimension(options.SpaceAbove, "space-above", "spaceAbove", func(v *docs.Dimension) { style.SpaceAbove = v }); err != nil {
		return nil, false, err
	}

	if err := addDimension(options.SpaceBelow, "space-below", "spaceBelow", func(v *docs.Dimension) { style.SpaceBelow = v }); err != nil {
		return nil, false, err
	}

	addBool := func(value *bool, field, forceField string, apply func(bool)) {
		if value == nil {
			return
		}

		apply(*value)

		fields = append(fields, field)
		if !*value {
			style.ForceSendFields = append(style.ForceSendFields, forceField)
		}
	}
	addBool(options.KeepWithNext, "keepWithNext", "KeepWithNext", func(v bool) { style.KeepWithNext = v })
	addBool(options.KeepLinesTogether, "keepLinesTogether", "KeepLinesTogether", func(v bool) { style.KeepLinesTogether = v })

	if len(fields) == 0 {
		return nil, false, nil
	}

	return &docs.Request{UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
		Range:          &docs.Range{StartIndex: start, EndIndex: end, TabId: tabID},
		ParagraphStyle: style,
		Fields:         strings.Join(fields, ","),
	}}, true, nil
}

func buildParagraphBulletsRequest(options Options, start, end int64, tabID string) (*docs.Request, bool, error) {
	presetFlags := 0
	if options.Bullets {
		presetFlags++
	}

	if options.Ordered {
		presetFlags++
	}

	if strings.TrimSpace(options.BulletPreset) != "" {
		presetFlags++
	}

	if presetFlags > 1 {
		return nil, false, ValidationError("--bullets, --ordered, and --bullet-preset are mutually exclusive")
	}

	if options.ClearBullets && presetFlags > 0 {
		return nil, false, ValidationError("--no-bullets cannot be combined with --bullets, --ordered, or --bullet-preset")
	}

	range_ := &docs.Range{StartIndex: start, EndIndex: end, TabId: tabID}
	if options.ClearBullets {
		return &docs.Request{DeleteParagraphBullets: &docs.DeleteParagraphBulletsRequest{Range: range_}}, true, nil
	}

	if presetFlags == 0 {
		return nil, false, nil
	}

	preset := BulletPresetDisc
	if options.Ordered {
		preset = BulletPresetNumbered
	}

	if custom := strings.ToUpper(strings.TrimSpace(options.BulletPreset)); custom != "" {
		preset = custom
	}

	if !validBulletPreset(preset) {
		return nil, false, ValidationError("--bullet-preset must be a supported Google Docs bullet glyph preset")
	}

	return &docs.Request{CreateParagraphBullets: &docs.CreateParagraphBulletsRequest{
		Range:        range_,
		BulletPreset: preset,
	}}, true, nil
}

func validBulletPreset(value string) bool {
	switch value {
	case "BULLET_DISC_CIRCLE_SQUARE",
		"BULLET_DIAMONDX_ARROW3D_SQUARE",
		"BULLET_CHECKBOX",
		"BULLET_ARROW_DIAMOND_DISC",
		"BULLET_STAR_CIRCLE_SQUARE",
		"BULLET_ARROW3D_CIRCLE_SQUARE",
		"BULLET_LEFTTRIANGLE_DIAMOND_DISC",
		"BULLET_DIAMONDX_HOLLOWDIAMOND_SQUARE",
		"BULLET_DIAMOND_CIRCLE_SQUARE",
		"NUMBERED_DECIMAL_ALPHA_ROMAN",
		"NUMBERED_DECIMAL_ALPHA_ROMAN_PARENS",
		"NUMBERED_DECIMAL_NESTED",
		"NUMBERED_UPPERALPHA_ALPHA_ROMAN",
		"NUMBERED_UPPERROMAN_UPPERALPHA_DECIMAL",
		"NUMBERED_ZERODECIMAL_ALPHA_ROMAN":
		return true
	default:
		return false
	}
}

func formatLink(value string) (*docs.Link, error) {
	link := strings.TrimSpace(value)
	if link == "" {
		return nil, ValidationError("--link target cannot be empty")
	}

	if target, ok := strings.CutPrefix(link, "#"); ok {
		target = strings.TrimSpace(target)
		if target == "" {
			return nil, ValidationError("--link target cannot be empty")
		}

		return &docs.Link{BookmarkId: target}, nil
	}

	return &docs.Link{Url: link}, nil
}

func formatNamedStyle(headingLevel *int, namedStyle string) (string, error) {
	trimmed := strings.ToUpper(strings.TrimSpace(namedStyle))
	if headingLevel != nil && trimmed != "" {
		return "", ValidationError("--heading-level and --named-style cannot be combined")
	}

	if headingLevel != nil {
		if *headingLevel < 1 || *headingLevel > 6 {
			return "", ValidationError("--heading-level must be between 1 and 6")
		}

		return fmt.Sprintf("HEADING_%d", *headingLevel), nil
	}

	if trimmed == "" {
		return "", nil
	}

	switch trimmed {
	case namedStyleNormalText, namedStyleTitle, namedStyleSubtitle,
		namedStyleHeading1, namedStyleHeading2, namedStyleHeading3,
		namedStyleHeading4, namedStyleHeading5, namedStyleHeading6:
		return trimmed, nil
	default:
		return "", ValidationError("--named-style must be one of NORMAL_TEXT, TITLE, SUBTITLE, HEADING_1..HEADING_6")
	}
}

// Color parses a Docs color flag in #RRGGBB or #RGB form.
func Color(hex, flag string) (*docs.OptionalColor, error) {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return nil, invalidf("%s must be #RRGGBB or #RGB", flag)
	}

	return &docs.OptionalColor{Color: &docs.Color{RgbColor: &docs.RgbColor{Red: r, Green: g, Blue: b}}}, nil
}

func formatAlignment(value string) (string, error) {
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
		return "", ValidationError("--alignment must be left, center, right, justify, start, end, or justified")
	}
}

func greyColor(intensity float64) *docs.OptionalColor {
	return &docs.OptionalColor{Color: &docs.Color{RgbColor: &docs.RgbColor{
		Red: intensity, Green: intensity, Blue: intensity,
	}}}
}

func parseHexColor(hex string) (r, g, b float64, ok bool) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}

	if len(hex) != 6 {
		return 0, 0, 0, false
	}

	rgb, err := strconv.ParseUint(hex, 16, 24)
	if err != nil {
		return 0, 0, 0, false
	}

	return float64((rgb>>16)&0xFF) / 255.0, float64((rgb>>8)&0xFF) / 255.0, float64(rgb&0xFF) / 255.0, true
}

func invalidf(format string, args ...any) error {
	return ValidationError(fmt.Sprintf(format, args...))
}
