// Package cmd provides CLI commands for Google Docs operations.
package cmd

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/docs/v1"
)

// buildColumnsRequest creates UpdateSectionStyleRequest to set column count.
// The range should span the section containing the match.
func buildColumnsRequest(be *braceExpr, sectionStart, sectionEnd int64) []*docs.Request {
	if be == nil || be.Cols <= 0 {
		return nil
	}

	// Build column properties array — one per column with equal width
	// Google Docs handles equal distribution automatically when widths not specified
	// Google Docs requires padding on each column property.
	// Default 36pt (0.5in) gap between columns.
	var colProps []*docs.SectionColumnProperties
	for i := 0; i < be.Cols; i++ {
		colProps = append(colProps, &docs.SectionColumnProperties{
			PaddingEnd: &docs.Dimension{Magnitude: 36, Unit: "PT"},
		})
	}

	return []*docs.Request{
		{
			UpdateSectionStyle: &docs.UpdateSectionStyleRequest{
				Range: &docs.Range{
					StartIndex: sectionStart,
					EndIndex:   sectionEnd,
				},
				SectionStyle: &docs.SectionStyle{
					ColumnProperties:     colProps,
					ColumnSeparatorStyle: "NONE",
				},
				Fields: "columnProperties,columnSeparatorStyle",
			},
		},
	}
}

// buildCheckboxRequests creates a checklist paragraph using BULLET_CHECKBOX preset.
// For {check=y}, the checked state cannot be toggled via API — Google Docs API
// creates unchecked checkboxes only. Users must manually check them.
func buildCheckboxRequests(be *braceExpr, start, end int64) []*docs.Request {
	if be == nil || be.Check == nil {
		return nil
	}

	// CreateParagraphBullets with BULLET_CHECKBOX preset creates checkbox lists.
	// Note: The API does not support setting the checked state programmatically.
	// All checkboxes are created unchecked; {check=y} is documented as a limitation.
	return []*docs.Request{
		{
			CreateParagraphBullets: &docs.CreateParagraphBulletsRequest{
				Range:        &docs.Range{StartIndex: start, EndIndex: end + 1},
				BulletPreset: "BULLET_CHECKBOX",
			},
		},
	}
}

// buildTOCRequest creates requests to insert a Table of Contents.
// Google Docs API does NOT support InsertTableOfContents via batchUpdate.
// This is a documented API limitation — TOC must be inserted manually via the UI.
//
// TODO: When Google adds InsertTableOfContentsRequest to the API, implement it here.
// For now, this function returns nil and the limitation is documented.
func buildTOCRequest(be *braceExpr, _ int64) []*docs.Request { //nolint:unparam // placeholder for future API support
	if be == nil || !be.HasTOC {
		return nil
	}

	// Google Docs API limitation: No InsertTableOfContentsRequest exists.
	// The API can read TOC elements but cannot create them programmatically.
	// Return nil and document as unsupported.
	return nil
}

// buildCommentRequest creates requests to add a comment/annotation to text.
// Google Docs batchUpdate API does NOT support creating comments.
// Comments must be created via the Drive Comments API (drive.comments.create).
//
// TODO: Implement via Drive API separately if needed.
// For now, this function returns nil and the limitation is documented.
func buildCommentRequest(be *braceExpr, _, _ int64) []*docs.Request { //nolint:unparam // placeholder for future API support
	if be == nil || be.Comment == "" {
		return nil
	}

	// Google Docs API limitation: Comments are not supported in batchUpdate.
	// The Drive API (v3) supports comments via drive.comments.create,
	// but that requires a separate API call outside of batchUpdate.
	// Return nil and document as unsupported in this flow.
	return nil
}

// buildBookmarkRequest creates a NamedRange (bookmark) at the matched text.
// Bookmarks can be linked via {u=#name} syntax.
func buildBookmarkRequest(be *braceExpr, start, end int64) []*docs.Request {
	if be == nil || be.Bookmark == "" {
		return nil
	}

	return []*docs.Request{
		{
			CreateNamedRange: &docs.CreateNamedRangeRequest{
				Name: be.Bookmark,
				Range: &docs.Range{
					StartIndex: start,
					EndIndex:   end,
				},
			},
		},
	}
}

// buildPersonChipRequest creates an InsertPerson request for person smart chips.
// Syntax: chip://person/email@example.com
func buildPersonChipRequest(email string, index int64) []*docs.Request {
	if email == "" {
		return nil
	}

	return []*docs.Request{
		{
			InsertPerson: &docs.InsertPersonRequest{
				Location: &docs.Location{Index: index},
				PersonProperties: &docs.PersonProperties{
					Email: email,
				},
			},
		},
	}
}

// ChipType represents the type of smart chip to insert.
type ChipType int

const (
	ChipTypeUnknown ChipType = iota
	ChipTypePerson
	ChipTypeDate
	ChipTypeFile
	ChipTypePlace
	ChipTypeDropdown
	ChipTypeChart
	ChipTypeBookmark
)

// ChipSpec holds parsed smart chip specification.
type ChipSpec struct {
	Type    ChipType
	Value   string   // email for person, date string for date, etc.
	Options []string // dropdown options
}

// parseChipURI parses a chip:// URI into a ChipSpec.
// Supported formats:
//   - chip://person/email@example.com
//   - chip://date/2026-03-15
//   - chip://file/DOC_ID
//   - chip://place/Orlando, FL
//   - chip://dropdown/Draft|Review|Done
//   - chip://bookmark/section-1
func parseChipURI(uri string) *ChipSpec {
	if !strings.HasPrefix(uri, "chip://") {
		return nil
	}

	// Remove prefix
	path := uri[7:] // "chip://" is 7 chars
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		return nil
	}

	chipType := strings.ToLower(parts[0])
	value := parts[1]

	spec := &ChipSpec{Value: value}

	switch chipType {
	case "person":
		spec.Type = ChipTypePerson
	case "date":
		spec.Type = ChipTypeDate
	case strFile:
		spec.Type = ChipTypeFile
	case "place":
		spec.Type = ChipTypePlace
	case "dropdown":
		spec.Type = ChipTypeDropdown
		// Parse pipe-separated options
		spec.Options = strings.Split(value, "|")
	case "chart":
		spec.Type = ChipTypeChart
	case "bookmark":
		spec.Type = ChipTypeBookmark
	default:
		return nil
	}

	return spec
}

// buildChipRequests creates requests for smart chip insertion based on chip:// URI.
// Supported chips:
//   - person: Uses InsertPerson API (fully supported)
//   - bookmark: Uses Link with BookmarkId (fully supported)
//
// Limited/unsupported chips (documented as API limitations):
//   - date: No InsertDate request in API
//   - file: RichLink is read-only
//   - place: No InsertPlace request in API
//   - dropdown: No InsertDropdown request in API
//   - chart: Use InsertInlineSheetsChart (requires sheetId, separate handling)
func buildChipRequests(be *braceExpr, index int64) []*docs.Request {
	if be == nil || be.URL == "" || !strings.HasPrefix(be.URL, "chip://") {
		return nil
	}

	chip := parseChipURI(be.URL)
	if chip == nil {
		return nil
	}

	switch chip.Type {
	case ChipTypePerson:
		return buildPersonChipRequest(chip.Value, index)

	case ChipTypeBookmark:
		// Bookmark chip is handled via link with BookmarkId
		// This is covered in buildBraceTextStyleRequests via {u=#name}
		// We convert chip://bookmark/name to #name for consistency
		return nil

	case ChipTypeDate:
		// Google Docs API limitation: No InsertDate request.
		// Date chips cannot be created programmatically.
		// TODO: When API support is added, implement here.
		return nil

	case ChipTypeFile:
		// Google Docs API limitation: RichLink is read-only.
		// File chips cannot be created via batchUpdate.
		// Workaround: Insert a regular hyperlink to the Drive file.
		return nil

	case ChipTypePlace:
		// Google Docs API limitation: No InsertPlace request.
		// Place chips cannot be created programmatically.
		return nil

	case ChipTypeDropdown:
		// Google Docs API limitation: No InsertDropdown request.
		// Dropdown chips cannot be created programmatically.
		return nil

	case ChipTypeChart:
		// InsertInlineSheetsChart exists but requires sheetId and chartId.
		// Parse format: chip://chart/SHEET_ID/CHART_INDEX
		// This is complex and handled separately in docs_sed_image.go
		return nil
	}

	return nil
}

// hasBraceStructuralFeatures returns true if the braceExpr has any structural features
// that require special handling beyond text/paragraph formatting.
func hasBraceStructuralFeatures(be *braceExpr) bool {
	if be == nil {
		return false
	}
	return be.Cols > 0 ||
		be.Check != nil ||
		be.HasTOC ||
		be.Comment != "" ||
		be.Bookmark != "" ||
		(be.URL != "" && strings.HasPrefix(be.URL, "chip://"))
}

// buildStructuralRequests builds all structural requests for a braceExpr.
// Returns:
//   - columnReqs: requests that modify section style (must be applied to sections)
//   - bulletReqs: checkbox/bullet requests (must be applied after text insertion)
//   - anchorReqs: bookmark/named range requests (must be applied to text ranges)
//   - chipReqs: smart chip requests (must be applied at indices)
func buildStructuralRequests(
	be *braceExpr,
	textStart, textEnd int64,
	sectionStart, sectionEnd int64,
) (columnReqs, bulletReqs, anchorReqs, chipReqs []*docs.Request) {
	if be == nil {
		return nil, nil, nil, nil
	}

	// Columns (section-level)
	if be.Cols > 0 {
		columnReqs = buildColumnsRequest(be, sectionStart, sectionEnd)
	}

	// Checkboxes (paragraph-level bullets)
	if be.Check != nil {
		bulletReqs = buildCheckboxRequests(be, textStart, textEnd)
	}

	// Bookmarks (text-level anchors)
	if be.Bookmark != "" {
		anchorReqs = buildBookmarkRequest(be, textStart, textEnd)
	}

	// Smart chips
	if be.URL != "" && strings.HasPrefix(be.URL, "chip://") {
		chipReqs = buildChipRequests(be, textStart)
	}

	return columnReqs, bulletReqs, anchorReqs, chipReqs
}

// parseDate parses a date string into components.
// Supports: YYYY-MM-DD, YYYY/MM/DD, MM-DD-YYYY, MM/DD/YYYY
func parseDate(dateStr string) (year, month, day int, ok bool) {
	// Try standard formats
	formats := []string{
		"2006-01-02",
		"2006/01/02",
		"01-02-2006",
		"01/02/2006",
		"January 2, 2006",
		"Jan 2, 2006",
	}

	for _, f := range formats {
		if t, err := time.Parse(f, dateStr); err == nil {
			return t.Year(), int(t.Month()), t.Day(), true
		}
	}

	return 0, 0, 0, false
}

// buildDateFallbackText creates a formatted date string for fallback display
// when native date chips are not available.
func buildDateFallbackText(dateStr string) string {
	if year, month, day, ok := parseDate(dateStr); ok {
		return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	}
	return dateStr
}

// resolveChipURL processes chip:// URLs and returns either:
//   - The original URL for non-chip URLs
//   - A fallback URL for chips that can be approximated with links
//   - Empty string for chips that have no link fallback
func resolveChipURL(chipURL string) (resolvedURL string, fallbackText string) {
	if !strings.HasPrefix(chipURL, "chip://") {
		return chipURL, ""
	}

	chip := parseChipURI(chipURL)
	if chip == nil {
		return "", ""
	}

	switch chip.Type {
	case ChipTypePerson:
		// Person chips are handled natively via InsertPerson
		return "", ""

	case ChipTypeBookmark:
		// Convert to bookmark link
		return "#" + chip.Value, ""

	case ChipTypeDate:
		// No link fallback for dates
		return "", buildDateFallbackText(chip.Value)

	case ChipTypeFile:
		// Convert to Drive file link
		return "https://docs.google.com/document/d/" + chip.Value, ""

	case ChipTypePlace:
		// Convert to Google Maps link
		return "https://maps.google.com/?q=" + url.QueryEscape(chip.Value), chip.Value

	case ChipTypeDropdown:
		// No link fallback, return options as text
		return "", strings.Join(chip.Options, " / ")

	case ChipTypeChart:
		// Charts need separate handling via InsertInlineSheetsChart
		return "", ""
	}

	return "", ""
}

// getCheckboxState returns a human-readable checkbox state description.
func getCheckboxState(be *braceExpr) string {
	if be == nil || be.Check == nil {
		return ""
	}
	if *be.Check {
		return "checked"
	}
	return "unchecked"
}

// buildSectionRangeForMatch finds the section boundaries containing a match.
// In Google Docs, sections are delimited by SectionBreak elements.
// If the document has no section breaks, the entire body is one section.
func buildSectionRangeForMatch(doc *docs.Document, matchStart, matchEnd int64) (sectionStart, sectionEnd int64) {
	if doc == nil || doc.Body == nil {
		return 1, matchEnd + 1
	}

	// Find section boundaries by scanning for SectionBreak elements
	sectionStart = 1 // Body starts at index 1
	sectionEnd = matchEnd + 1

	// Track the last section break we passed
	for _, elem := range doc.Body.Content {
		if elem.SectionBreak != nil {
			// If we've passed this section break and it's before the match
			if elem.EndIndex <= matchStart {
				sectionStart = elem.EndIndex
			}
			// If this section break is after the match, it's the end boundary
			if elem.StartIndex > matchEnd && sectionEnd == matchEnd+1 {
				sectionEnd = elem.StartIndex
				break
			}
		}
		// Update sectionEnd to the last element's end index
		if elem.EndIndex > sectionEnd {
			sectionEnd = elem.EndIndex
		}
	}

	// Ensure valid range
	if sectionEnd <= sectionStart {
		sectionEnd = sectionStart + 1
	}

	return sectionStart, sectionEnd
}

// stripChipPrefix removes the chip:// prefix and type from a URL
// and returns just the value portion. Used for fallback text display.
func stripChipPrefix(uri string) string {
	if !strings.HasPrefix(uri, "chip://") {
		return uri
	}

	path := uri[7:]
	if idx := strings.Index(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// isPersonChip returns true if the URL is a person chip.
func isPersonChip(uri string) bool {
	return strings.HasPrefix(uri, "chip://person/")
}

// extractPersonEmail extracts the email from a person chip URL.
func extractPersonEmail(uri string) string {
	if !isPersonChip(uri) {
		return ""
	}
	return uri[14:] // len("chip://person/") = 14
}

// parseChartChip parses a chart chip URL.
// Format: chip://chart/SHEET_ID/CHART_INDEX
// Returns sheetId and chartIndex (0-based).
func parseChartChip(uri string) (sheetID string, chartIndex int, ok bool) {
	if !strings.HasPrefix(uri, "chip://chart/") {
		return "", 0, false
	}

	path := uri[13:] // len("chip://chart/") = 13
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		return "", 0, false
	}

	sheetID = parts[0]
	idx, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}

	return sheetID, idx, true
}

// TODO: Implement full chart insertion in docs_sed_image.go using:
// InsertInlineSheetsChart (need to check if this request type exists)
// For now, chart chips are not supported via this path.
