package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// compilePattern compiles the sedExpr's pattern into a regexp.
func (e *sedExpr) compilePattern() (*regexp.Regexp, error) {
	return regexp.Compile(e.pattern)
}

// batchUpdate executes a Documents.BatchUpdate with retry-on-quota.
// Returns the response (may be nil on success with no replies).
func batchUpdate(ctx context.Context, docsSvc *docs.Service, docID string, reqs []*docs.Request) (*docs.BatchUpdateDocumentResponse, error) {
	if len(reqs) == 0 {
		return &docs.BatchUpdateDocumentResponse{DocumentId: docID}, nil
	}
	var resp *docs.BatchUpdateDocumentResponse
	err := retryOnQuota(ctx, func() error {
		var e error
		resp, e = docsSvc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
			Requests: reqs,
		}).Context(ctx).Do()
		return e
	})
	return resp, err
}

// getDoc fetches a document with retry-on-quota.
func getDoc(ctx context.Context, docsSvc *docs.Service, docID string) (*docs.Document, error) {
	var doc *docs.Document
	err := retryOnQuota(ctx, func() error {
		var e error
		doc, e = docsSvc.Documents.Get(docID).Context(ctx).Do()
		return e
	})
	return doc, err
}

// codeBackgroundGrey is the RGB value for inline code background shading.
const codeBackgroundGrey = 0.95

// borderGrey is the grey intensity for borders (blockquotes, horizontal rules).
const borderGrey = 0.8

// indentPointsPerLevel is the number of points per indent level in Google Docs.
const indentPointsPerLevel = 36.0

// hrulePaddingPt is the padding in points below a horizontal rule border.
const hrulePaddingPt = 6.0

// blockquoteBorderWidthPt is the border width in points for blockquotes.
const blockquoteBorderWidthPt = 3.0

// blockquoteIndentPt is the left indent in points for blockquotes.
const blockquoteIndentPt = 36.0

// blockquotePaddingPt is the left padding in points for blockquotes.
const blockquotePaddingPt = 12.0

// bulletPresetDisc is the default unordered bullet preset.
const bulletPresetDisc = "BULLET_DISC_CIRCLE_SQUARE"

// Table cell operation constants.
const (
	opAppend = "append"
	opInsert = "insert"
	opDelete = "delete"
)

// Table merge/split operation constants.
const (
	mergeOp   = "merge"
	unmergeOp = "unmerge"
	splitOp   = "split"
)

// buildImageSizeSpec returns a Size for the image spec, or nil if no dimensions set.
func buildImageSizeSpec(spec *ImageSpec) *docs.Size {
	if spec.Width == 0 && spec.Height == 0 {
		return nil
	}
	size := &docs.Size{}
	if spec.Width > 0 {
		size.Width = &docs.Dimension{Magnitude: float64(spec.Width), Unit: "PT"}
	}
	if spec.Height > 0 {
		size.Height = &docs.Dimension{Magnitude: float64(spec.Height), Unit: "PT"}
	}
	return size
}

// buildCellReplaceRequests builds delete+insert+format requests for replacing table cell content.
// If deleteEnd > startIdx, deletes old content. Inserts plainText at startIdx and applies formats.
func buildCellReplaceRequests(startIdx, deleteEnd int64, plainText string, formats []string) []*docs.Request {
	var requests []*docs.Request
	if startIdx < deleteEnd {
		requests = append(requests, &docs.Request{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{StartIndex: startIdx, EndIndex: deleteEnd},
			},
		})
	}
	if plainText != "" {
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: startIdx},
				Text:     plainText,
			},
		})
		if len(formats) > 0 {
			end := startIdx + int64(len(plainText))
			requests = append(requests, buildTextStyleRequests(formats, startIdx, end)...)
		}
	}
	return requests
}

// sedOutputKV is an ordered key-value pair for sedOutputOK.
type sedOutputKV struct {
	Key   string
	Value any
}

// sedOutputOK writes the standard sed output (status=ok, docId, plus extra key-value pairs).
// Keys are output in the order provided.
func sedOutputOK(ctx context.Context, u *ui.UI, id string, extra ...sedOutputKV) error {
	result := map[string]any{"status": "ok", "docId": id}
	for _, kv := range extra {
		result[kv.Key] = kv.Value
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, result)
	}
	u.Out().Printf("status\tok")
	u.Out().Printf("docId\t%s", id)
	for _, kv := range extra {
		u.Out().Printf("%s\t%v", kv.Key, kv.Value)
	}
	return nil
}

// buildTextStyleRequests creates UpdateTextStyle requests from format strings.
// Handles: bold, italic, strikethrough, code, underline, link:URL, smallcaps,
// font:NAME, size:N, color:#HEX, bg:#HEX
// Returns empty slice if no text-level formats found.
func buildTextStyleRequests(formats []string, start, end int64) []*docs.Request {
	textStyle := &docs.TextStyle{}
	var textFields []string

	for _, f := range formats {
		switch f {
		case "bold":
			textStyle.Bold = true
			textFields = append(textFields, "bold")
		case "italic":
			textStyle.Italic = true
			textFields = append(textFields, "italic")
		case "strikethrough":
			textStyle.Strikethrough = true
			textFields = append(textFields, "strikethrough")
		case inlineTypeCode:
			textStyle.WeightedFontFamily = &docs.WeightedFontFamily{FontFamily: "Courier New"}
			textStyle.BackgroundColor = greyColor(codeBackgroundGrey)
			textFields = append(textFields, "weightedFontFamily", "backgroundColor")
		case "underline":
			textStyle.Underline = true
			textFields = append(textFields, "underline")
		case "superscript":
			textStyle.BaselineOffset = "SUPERSCRIPT"
			textFields = append(textFields, "baselineOffset")
		case "subscript":
			textStyle.BaselineOffset = "SUBSCRIPT"
			textFields = append(textFields, "baselineOffset")
		case "smallcaps":
			textStyle.SmallCaps = true
			textFields = append(textFields, "smallCaps")
		default:
			switch {
			case strings.HasPrefix(f, "link:"):
				linkURL := f[5:]
				if strings.HasPrefix(linkURL, "#") {
					textStyle.Link = &docs.Link{BookmarkId: linkURL[1:]}
				} else {
					textStyle.Link = &docs.Link{Url: linkURL}
				}
				textFields = append(textFields, "link")
			case strings.HasPrefix(f, "font:"):
				fontName := f[5:]
				textStyle.WeightedFontFamily = &docs.WeightedFontFamily{FontFamily: fontName}
				textFields = append(textFields, "weightedFontFamily")
			case strings.HasPrefix(f, "size:"):
				sizeStr := f[5:]
				if size, err := strconv.ParseFloat(sizeStr, 64); err == nil && size > 0 {
					textStyle.FontSize = &docs.Dimension{Magnitude: size, Unit: "PT"}
					textFields = append(textFields, "fontSize")
				}
			case strings.HasPrefix(f, "color:"):
				colorVal := f[6:]
				if r, g, b, ok := parseHexColor(colorVal); ok {
					textStyle.ForegroundColor = &docs.OptionalColor{
						Color: &docs.Color{RgbColor: &docs.RgbColor{Red: r, Green: g, Blue: b}},
					}
					textFields = append(textFields, "foregroundColor")
				}
			case strings.HasPrefix(f, "bg:"):
				bgVal := f[3:]
				if r, g, b, ok := parseHexColor(bgVal); ok {
					textStyle.BackgroundColor = &docs.OptionalColor{
						Color: &docs.Color{RgbColor: &docs.RgbColor{Red: r, Green: g, Blue: b}},
					}
					textFields = append(textFields, "backgroundColor")
				}
			}
		}
	}

	if len(textFields) == 0 {
		return nil
	}

	return []*docs.Request{
		{
			UpdateTextStyle: &docs.UpdateTextStyleRequest{
				Range:     &docs.Range{StartIndex: start, EndIndex: end},
				TextStyle: textStyle,
				Fields:    strings.Join(textFields, ","),
			},
		},
	}
}

// buildParagraphStyleRequests creates UpdateParagraphStyle and/or CreateParagraphBullets
// requests from format strings. The end parameter should already include the +1 for
// paragraph newline (caller decides).
// Handles: heading1-6, bullet, checkbox, numbered.
// Returns empty slice if no paragraph-level formats found.
func buildParagraphStyleRequests(formats []string, start, end int64) []*docs.Request {
	var headingLevel int
	var bulletPreset string
	var isBlockquote bool

	for _, f := range formats {
		if strings.HasPrefix(f, "heading") && len(f) == 8 {
			level := int(f[7] - '0')
			if level >= 1 && level <= 6 {
				headingLevel = level
			}
		}
		switch f {
		case "bullet":
			bulletPreset = bulletPresetDisc
		case "checkbox":
			bulletPreset = "BULLET_CHECKBOX"
		case "numbered":
			bulletPreset = "NUMBERED_DECIMAL_NESTED"
		case "blockquote":
			isBlockquote = true
		}
	}

	var requests []*docs.Request

	if headingLevel > 0 {
		namedStyle := fmt.Sprintf("HEADING_%d", headingLevel)
		requests = append(requests, &docs.Request{
			UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
				Range: &docs.Range{StartIndex: start, EndIndex: end},
				ParagraphStyle: &docs.ParagraphStyle{
					NamedStyleType: namedStyle,
				},
				Fields: "namedStyleType",
			},
		})
	}

	if bulletPreset != "" {
		requests = append(requests, &docs.Request{
			CreateParagraphBullets: &docs.CreateParagraphBulletsRequest{
				Range:        &docs.Range{StartIndex: start, EndIndex: end},
				BulletPreset: bulletPreset,
			},
		})
		// Nesting is handled by prepending \t characters before the text content.
		// When CreateParagraphBullets is applied, Google Docs converts leading
		// tabs into nesting levels automatically. The tabs must already be in
		// the text BEFORE the bullet request. See buildNestedListText().
	}

	if isBlockquote {
		requests = append(requests, &docs.Request{
			UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
				Range: &docs.Range{StartIndex: start, EndIndex: end},
				ParagraphStyle: &docs.ParagraphStyle{
					IndentStart: &docs.Dimension{Magnitude: blockquoteIndentPt, Unit: "PT"},
					BorderLeft: &docs.ParagraphBorder{
						Color:     greyColor(borderGrey),
						Width:     &docs.Dimension{Magnitude: blockquoteBorderWidthPt, Unit: "PT"},
						DashStyle: "SOLID",
						Padding:   &docs.Dimension{Magnitude: blockquotePaddingPt, Unit: "PT"},
					},
				},
				Fields: "indentStart,borderLeft",
			},
		})
	}

	return requests
}

// buildHruleBorderRequest returns an UpdateParagraphStyle request that styles a paragraph
// as a horizontal rule (bottom border only).
func buildHruleBorderRequest(start, end int64) *docs.Request {
	return &docs.Request{
		UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
			Range: &docs.Range{StartIndex: start, EndIndex: end},
			ParagraphStyle: &docs.ParagraphStyle{
				BorderBottom: &docs.ParagraphBorder{
					Color:     greyColor(borderGrey),
					Width:     &docs.Dimension{Magnitude: 1, Unit: "PT"},
					DashStyle: "SOLID",
					Padding:   &docs.Dimension{Magnitude: hrulePaddingPt, Unit: "PT"},
				},
			},
			Fields: "borderBottom",
		},
	}
}

// greyColor returns an OptionalColor with the given greyscale intensity (0.0=black, 1.0=white).
func greyColor(intensity float64) *docs.OptionalColor {
	return &docs.OptionalColor{Color: &docs.Color{RgbColor: &docs.RgbColor{Red: intensity, Green: intensity, Blue: intensity}}}
}

// containsFormat returns true if the format slice contains the given format string.
func containsFormat(formats []string, f string) bool {
	for _, v := range formats {
		if v == f {
			return true
		}
	}
	return false
}

// inlineScriptRe matches inline superscript/subscript markers:
//   - {super=text} and {sub=text} (preferred)
//   - ^{text} and ~{text} (deprecated, still supported)
// (Legacy buildInlineScriptRequests and buildAttrRequests removed — use brace syntax instead)

// parseHexColor converts #RRGGBB or #RGB to normalized 0.0-1.0 RGB floats.
func parseHexColor(hex string) (r, g, b float64, ok bool) {
	hex = strings.TrimPrefix(hex, "#")
	// Expand #RGB shorthand to #RRGGBB
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	// Parse all 6 hex digits as a single uint to avoid 3 separate ParseUint calls.
	rgb, err := strconv.ParseUint(hex, 16, 24)
	if err != nil {
		return 0, 0, 0, false
	}
	return float64((rgb>>16)&0xFF) / 255.0, float64((rgb>>8)&0xFF) / 255.0, float64(rgb&0xFF) / 255.0, true
}

// Markdown escape placeholders — package-level to avoid per-call allocation.
const (
	escAsterisk  = "\x00ESC_ASTERISK\x00"
	escHash      = "\x00ESC_HASH\x00"
	escTilde     = "\x00ESC_TILDE\x00"
	escBacktick  = "\x00ESC_BACKTICK\x00"
	escDash      = "\x00ESC_DASH\x00"
	escPlus      = "\x00ESC_PLUS\x00"
	escBackslash = "\x00ESC_BACKSLASH\x00"
)

// Package-level replacers for markdown escape/unescape — allocated once.
var (
	mdEscaper = strings.NewReplacer(
		"\\\\", escBackslash, "\\*", escAsterisk, "\\#", escHash,
		"\\~", escTilde, "\\`", escBacktick, "\\-", escDash,
		"\\+", escPlus, "\\n", "\n",
	)
	mdUnescaper = strings.NewReplacer(
		escAsterisk, "*", escHash, "#", escTilde, "~",
		escBacktick, "`", escDash, "-", escPlus, "+",
		escBackslash, "\\",
	)
)

// escapeMarkdown replaces escaped markdown characters with placeholders.
func escapeMarkdown(s string) string { return mdEscaper.Replace(s) }

// unescapeMarkdown restores escaped markdown characters from placeholders.
func unescapeMarkdown(s string) string { return mdUnescaper.Replace(s) }

// nativeBlockMarkers are markdown format markers that prevent native API replacement.
// Package-level to avoid per-call allocation.
var nativeBlockMarkers = []string{
	"**", "*", "~~", "`",
	"# ", "## ", "### ", "#### ", "##### ", "###### ",
	"- ", "+ ",
	"> ",
	"[^",
}

// canUseNativeReplace returns true if the replacement string contains no markdown
// formatting or brace expressions that require manual (per-run) application,
// allowing the faster native Google Docs FindReplace API to be used instead.
func canUseNativeReplace(replacement string) bool {
	// Check for SEDMAT brace formatting ({b}, {c=red}, etc.)
	if hasBraceFormatting(replacement) {
		return false
	}
	// Check for image syntax (both ![alt](url) and !(url) shorthand)
	if strings.HasPrefix(replacement, "![") {
		return false
	}
	if strings.HasPrefix(replacement, "!(") && strings.HasSuffix(replacement, ")") {
		inner := replacement[2 : len(replacement)-1]
		if strings.HasPrefix(inner, "http://") || strings.HasPrefix(inner, "https://") {
			return false
		}
	}
	for _, marker := range nativeBlockMarkers {
		if strings.Contains(replacement, marker) {
			return false
		}
	}
	// Horizontal rule
	trimmedRepl := strings.TrimSpace(replacement)
	if trimmedRepl == "---" || trimmedRepl == "***" || trimmedRepl == "___" {
		return false
	}
	// Numbered list pattern
	if len(replacement) >= 3 && replacement[0] >= '0' && replacement[0] <= '9' &&
		replacement[1] == '.' && replacement[2] == ' ' {
		return false
	}
	// Escape sequences
	if strings.Contains(replacement, "\\n") {
		return false
	}
	// Backreferences ($1, ${1}, etc.)
	for i := 0; i < len(replacement)-1; i++ {
		if replacement[i] == '$' {
			next := replacement[i+1]
			if (next >= '1' && next <= '9') || next == '{' {
				return false
			}
		}
	}
	// Link syntax [text](url)
	if strings.Contains(replacement, "](") {
		return false
	}
	return true
}
