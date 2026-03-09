//go:build live

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/docs/v1"
)

// TestV10LiveVerification fetches the live test document and verifies
// that all v10 sedmat features rendered correctly by inspecting the
// document model (paragraphs, styles, tables, images).
//
// Prerequisites:
//   - Run v10 seed + test against the doc first
//   - Set GOG_LIVE_DOC_ID and GOG_LIVE_ACCOUNT env vars
//
// Run: go test ./internal/cmd/ -tags live -run TestV10Live -v -count=1
func TestV10LiveVerification(t *testing.T) {
	docID := os.Getenv("GOG_TEST_DOC_ID")
	if docID == "" {
		t.Skip("GOG_TEST_DOC_ID not set; skipping live test")
	}
	account := os.Getenv("GOG_TEST_ACCOUNT")
	if account == "" {
		t.Skip("GOG_TEST_ACCOUNT not set; skipping live test")
	}

	// Fetch raw document JSON via gog binary (has OAuth tokens configured)
	// TestMain overrides HOME to a temp dir; restore real HOME so
	// both the gog subprocess and the direct API can find credentials.
	if realHome := os.Getenv("GOG_LIVE_HOME"); realHome != "" {
		prev := os.Getenv("HOME")
		os.Setenv("HOME", realHome)
		t.Cleanup(func() { os.Setenv("HOME", prev) })
	}

	gogBin := os.Getenv("GOG_BIN")
	if gogBin == "" {
		gogBin = "gog"
	}

	ctx := context.Background()
	_ = ctx

	cmd := exec.Command(gogBin, "docs", "cat", docID, "-a", account, "-j")
	out, err := cmd.Output()
	if err != nil {
		// Fall back: try direct API
		docsSvc, err2 := newDocsService(context.Background(), account)
		if err2 != nil {
			t.Fatalf("can't fetch doc: gog failed (%v) and direct API failed (%v)", err, err2)
		}
		doc, err2 := docsSvc.Documents.Get(docID).Context(context.Background()).Do()
		require.NoError(t, err2, "fetch document")
		require.NotNil(t, doc.Body)
		_ = doc
		t.Skip("gog cat -j not available, need raw doc JSON")
		return
	}

	// gog docs cat -j returns plain text, not the doc structure.
	// We need the raw API response. Let's use direct API with proper auth setup.
	// For now, use a helper binary to dump the doc.
	_ = out

	// Actually, let's just build a small helper that uses the gog OAuth tokens
	docsSvc, err := newDocsServiceFromConfig(account)
	require.NoError(t, err, "create docs service")

	doc, err := docsSvc.Documents.Get(docID).Context(context.Background()).Do()
	require.NoError(t, err, "fetch document")
	require.NotNil(t, doc.Body, "document body")

	// Build helpers
	paras := extractParagraphs(doc)
	tables := extractTables(doc)
	images := extractImages(doc)

	t.Run("Headings", func(t *testing.T) {
		assertParaStyle(t, paras, "SEDMAT Comprehensive Test v10", "TITLE")
		assertParaStyle(t, paras, "Text Formatting", "SUBTITLE")
		assertParaStyle(t, paras, "Heading Level Three", "HEADING_3")
		assertParaStyle(t, paras, "Heading Level Four", "HEADING_4")
		assertParaStyle(t, paras, "Heading Level Five", "HEADING_5")
		assertParaStyle(t, paras, "Heading Level Six", "HEADING_6")
	})

	t.Run("InlineStyles", func(t *testing.T) {
		assertTextStyle(t, paras, "This text is bold", "bold", true)
		assertTextStyle(t, paras, "This text is italic", "italic", true)
		assertTextStyle(t, paras, "Bold and italic combined", "bold", true)
		assertTextStyle(t, paras, "Bold and italic combined", "italic", true)
		assertTextStyle(t, paras, "Strikethrough text", "strikethrough", true)
		assertTextStyle(t, paras, "inline code snippet", "code", true)
		assertTextStyle(t, paras, "Underlined text here", "underline", true)
	})

	t.Run("Link", func(t *testing.T) {
		assertLink(t, paras, "Visit Deft.md", "https://deft.md")
	})

	t.Run("BulletLists", func(t *testing.T) {
		assertHasBullet(t, paras, "First bullet point")
		assertHasBullet(t, paras, "Second bullet point")
		assertHasBullet(t, paras, "Third bullet point")
	})

	t.Run("NumberedLists", func(t *testing.T) {
		assertHasBullet(t, paras, "First numbered item")
		assertHasBullet(t, paras, "Second numbered item")
	})

	t.Run("NestedBullets", func(t *testing.T) {
		assertHasBullet(t, paras, "Top level bullet")
		assertHasBullet(t, paras, "Nested bullet level 1")
		assertHasBullet(t, paras, "Nested bullet level 2")
		assertNestingLevel(t, paras, "Nested bullet level 1", 1)
		assertNestingLevel(t, paras, "Nested bullet level 2", 2)
	})

	t.Run("NestedNumbered", func(t *testing.T) {
		assertHasBullet(t, paras, "Top level numbered")
		assertHasBullet(t, paras, "Nested numbered level 1")
		assertNestingLevel(t, paras, "Nested numbered level 1", 1)
	})

	t.Run("RegexReplacements", func(t *testing.T) {
		assertParaContains(t, paras, "Hello World 2026")
		assertParaContains(t, paras, "$500.00")
		assertParaContains(t, paras, "Global: XXX")
		assertParaContains(t, paras, "Three-As")
		assertParaContains(t, paras, "Smith, John")
		assertParaContains(t, paras, "$49.99 each")
		assertParaContains(t, paras, "$100 + $200 = $300")
	})

	t.Run("HorizontalRule", func(t *testing.T) {
		// Hrule is a paragraph with bottom border
		assertParaExists(t, paras, "Text after the horizontal rule")
	})

	t.Run("Blockquotes", func(t *testing.T) {
		assertParaContains(t, paras, "simple blockquote")
		assertParaContains(t, paras, "Steve Jobs")
	})

	t.Run("CodeBlocks", func(t *testing.T) {
		assertParaContains(t, paras, "function greet")
		assertParaContains(t, paras, "fmt.Println")
	})

	t.Run("Superscript", func(t *testing.T) {
		assertSuperscript(t, paras, "E = mc", "2")
	})

	t.Run("Subscript", func(t *testing.T) {
		assertSubscript(t, paras, "H", "2")
	})

	t.Run("Tables", func(t *testing.T) {
		require.GreaterOrEqual(t, len(tables), 3, "at least 3 tables")

		// First pipe table: 3x2
		assert.Equal(t, 3, tables[0].rows, "pipe table rows")
		assert.Equal(t, 2, tables[0].cols, "pipe table cols")
		assert.Contains(t, tables[0].cellText(0, 0), "Col A")
		assert.Contains(t, tables[0].cellText(1, 1), "Data 2")

		// Second pipe table: 4x3 with bold headers
		assert.Equal(t, 4, tables[1].rows, "feature table rows")
		assert.Equal(t, 3, tables[1].cols, "feature table cols")
	})

	t.Run("Images", func(t *testing.T) {
		require.GreaterOrEqual(t, len(images), 1, "at least 1 image")
	})

	t.Run("Footnotes", func(t *testing.T) {
		require.NotNil(t, doc.Footnotes, "footnotes exist")
		assert.GreaterOrEqual(t, len(doc.Footnotes), 2, "at least 2 footnotes")
	})

	// ── v10 Style Attributes ──

	t.Run("AttrFont", func(t *testing.T) {
		assertFontFamily(t, paras, "Font: Georgia text", "Georgia")
	})

	t.Run("AttrSize", func(t *testing.T) {
		assertFontSize(t, paras, "Size: 20pt text", 20)
	})

	t.Run("AttrColor", func(t *testing.T) {
		assertTextColor(t, paras, "Color: Red text", 1.0, 0, 0)
	})

	t.Run("AttrBg", func(t *testing.T) {
		assertBgColor(t, paras, "Highlight: Yellow bg", 1.0, 1.0, 0)
	})

	t.Run("AttrCombo", func(t *testing.T) {
		assertFontFamily(t, paras, "Combo: Blue Georgia 16pt", "Georgia")
		assertFontSize(t, paras, "Combo: Blue Georgia 16pt", 16)
		assertTextColor(t, paras, "Combo: Blue Georgia 16pt", 0, 0, 1.0)
	})

	t.Run("AttrBoldFont", func(t *testing.T) {
		assertTextStyle(t, paras, "Bold Montserrat 18pt", "bold", true)
		assertFontFamily(t, paras, "Bold Montserrat 18pt", "Montserrat")
		assertFontSize(t, paras, "Bold Montserrat 18pt", 18)
	})

	t.Run("AttrHeadingFont", func(t *testing.T) {
		assertParaStyle(t, paras, "Styled Heading", "HEADING_3")
	})

	t.Run("NewSuperSyntax", func(t *testing.T) {
		// {super=TM} should produce TM in superscript
		assertSuperscript(t, paras, "", "TM")
	})

	t.Run("NewSubSyntax", func(t *testing.T) {
		// {sub=0} should produce 0 in subscript
		assertSubscript(t, paras, "", "0")
	})

	t.Run("NewSuperInline", func(t *testing.T) {
		// E = mc{super=2}
		assertSuperscript(t, paras, "E = mc", "2")
	})

	t.Run("NewSubInline", func(t *testing.T) {
		// H{sub=2}O
		assertSubscript(t, paras, "H", "2")
	})

	t.Run("Commands", func(t *testing.T) {
		assertParaStyle(t, paras, "Results", "SUBTITLE")
		// DELETE THIS LINE should be gone
		for _, p := range paras {
			assert.NotContains(t, p.text, "DELETE THIS LINE", "deleted line should be gone")
		}
		assertParaContains(t, paras, "Line one of insert")
		assertParaContains(t, paras, "Line two of insert")
	})

	fmt.Printf("\n✅ Live verification complete: %d paragraphs, %d tables, %d images, %d footnotes checked\n",
		len(paras), len(tables), len(images), len(doc.Footnotes))
}

// ── Helpers ──

type paraInfo struct {
	text      string
	style     string // paragraph style (TITLE, HEADING_3, etc.)
	bullet    *docs.Bullet
	runs      []*docs.TextRun
	paraStyle *docs.ParagraphStyle
}

type tableInfo struct {
	rows  int
	cols  int
	cells [][]string // [row][col] = text
}

func (t *tableInfo) cellText(row, col int) string {
	if row < len(t.cells) && col < len(t.cells[row]) {
		return t.cells[row][col]
	}
	return ""
}

type imageInfo struct {
	uri    string
	width  float64
	height float64
}

func extractParagraphs(doc *docs.Document) []paraInfo {
	var paras []paraInfo
	var walk func([]*docs.StructuralElement)
	walk = func(content []*docs.StructuralElement) {
		for _, elem := range content {
			if elem.Paragraph != nil {
				p := paraInfo{
					bullet:    elem.Paragraph.Bullet,
					paraStyle: elem.Paragraph.ParagraphStyle,
				}
				if elem.Paragraph.ParagraphStyle != nil {
					p.style = elem.Paragraph.ParagraphStyle.NamedStyleType
				}
				for _, pe := range elem.Paragraph.Elements {
					if pe.TextRun != nil {
						p.text += pe.TextRun.Content
						p.runs = append(p.runs, pe.TextRun)
					}
				}
				p.text = strings.TrimSpace(p.text)
				if p.text != "" {
					paras = append(paras, p)
				}
			}
			if elem.Table != nil {
				for _, row := range elem.Table.TableRows {
					for _, cell := range row.TableCells {
						walk(cell.Content)
					}
				}
			}
		}
	}
	if doc.Body != nil {
		walk(doc.Body.Content)
	}
	return paras
}

func extractTables(doc *docs.Document) []tableInfo {
	var tables []tableInfo
	if doc.Body == nil {
		return tables
	}
	for _, elem := range doc.Body.Content {
		if elem.Table != nil {
			ti := tableInfo{
				rows: int(elem.Table.Rows),
				cols: int(elem.Table.Columns),
			}
			for _, row := range elem.Table.TableRows {
				var rowCells []string
				for _, cell := range row.TableCells {
					cellText := ""
					for _, ce := range cell.Content {
						if ce.Paragraph != nil {
							for _, pe := range ce.Paragraph.Elements {
								if pe.TextRun != nil {
									cellText += pe.TextRun.Content
								}
							}
						}
					}
					rowCells = append(rowCells, strings.TrimSpace(cellText))
				}
				ti.cells = append(ti.cells, rowCells)
			}
			tables = append(tables, ti)
		}
	}
	return tables
}

func extractImages(doc *docs.Document) []imageInfo {
	var images []imageInfo
	var walk func([]*docs.StructuralElement)
	walk = func(content []*docs.StructuralElement) {
		for _, elem := range content {
			if elem.Paragraph != nil {
				for _, pe := range elem.Paragraph.Elements {
					if pe.InlineObjectElement != nil {
						objID := pe.InlineObjectElement.InlineObjectId
						if obj, ok := doc.InlineObjects[objID]; ok {
							img := imageInfo{}
							if obj.InlineObjectProperties != nil && obj.InlineObjectProperties.EmbeddedObject != nil {
								eo := obj.InlineObjectProperties.EmbeddedObject
								if eo.ImageProperties != nil {
									img.uri = eo.ImageProperties.ContentUri
								}
								if eo.Size != nil {
									if eo.Size.Width != nil {
										img.width = eo.Size.Width.Magnitude
									}
									if eo.Size.Height != nil {
										img.height = eo.Size.Height.Magnitude
									}
								}
							}
							images = append(images, img)
						}
					}
				}
			}
		}
	}
	if doc.Body != nil {
		walk(doc.Body.Content)
	}
	return images
}

func findPara(paras []paraInfo, substr string) *paraInfo {
	for i := range paras {
		if strings.Contains(paras[i].text, substr) {
			return &paras[i]
		}
	}
	return nil
}

func assertParaExists(t *testing.T, paras []paraInfo, substr string) {
	t.Helper()
	assert.NotNil(t, findPara(paras, substr), "paragraph containing %q should exist", substr)
}

func assertParaContains(t *testing.T, paras []paraInfo, substr string) {
	t.Helper()
	assertParaExists(t, paras, substr)
}

func assertParaStyle(t *testing.T, paras []paraInfo, substr, expectedStyle string) {
	t.Helper()
	p := findPara(paras, substr)
	if assert.NotNil(t, p, "paragraph containing %q should exist", substr) {
		assert.Equal(t, expectedStyle, p.style, "paragraph %q style", substr)
	}
}

func assertTextStyle(t *testing.T, paras []paraInfo, substr, prop string, expected bool) {
	t.Helper()
	p := findPara(paras, substr)
	if !assert.NotNil(t, p, "paragraph containing %q should exist", substr) {
		return
	}
	found := false
	for _, run := range p.runs {
		if run.TextStyle == nil {
			continue
		}
		switch prop {
		case "bold":
			if run.TextStyle.Bold {
				found = true
			}
		case "italic":
			if run.TextStyle.Italic {
				found = true
			}
		case "strikethrough":
			if run.TextStyle.Strikethrough {
				found = true
			}
		case "underline":
			if run.TextStyle.Underline {
				found = true
			}
		case "code":
			if run.TextStyle.WeightedFontFamily != nil &&
				strings.Contains(strings.ToLower(run.TextStyle.WeightedFontFamily.FontFamily), "courier") {
				found = true
			}
		}
	}
	assert.True(t, found, "paragraph %q should have %s=%v", substr, prop, expected)
}

func assertLink(t *testing.T, paras []paraInfo, substr, expectedURL string) {
	t.Helper()
	p := findPara(paras, substr)
	if !assert.NotNil(t, p, "paragraph containing %q should exist", substr) {
		return
	}
	found := false
	for _, run := range p.runs {
		if run.TextStyle != nil && run.TextStyle.Link != nil && strings.Contains(run.TextStyle.Link.Url, expectedURL) {
			found = true
		}
	}
	assert.True(t, found, "paragraph %q should have link to %q", substr, expectedURL)
}

func assertHasBullet(t *testing.T, paras []paraInfo, substr string) {
	t.Helper()
	p := findPara(paras, substr)
	if assert.NotNil(t, p, "paragraph containing %q should exist", substr) {
		assert.NotNil(t, p.bullet, "paragraph %q should have a bullet", substr)
	}
}

func assertNestingLevel(t *testing.T, paras []paraInfo, substr string, level int) {
	t.Helper()
	p := findPara(paras, substr)
	if !assert.NotNil(t, p, "paragraph containing %q should exist", substr) {
		return
	}
	if assert.NotNil(t, p.bullet, "paragraph %q should have a bullet", substr) {
		assert.Equal(t, int64(level), p.bullet.NestingLevel, "paragraph %q nesting level", substr)
	}
}

func assertFontFamily(t *testing.T, paras []paraInfo, substr, expectedFont string) {
	t.Helper()
	p := findPara(paras, substr)
	if !assert.NotNil(t, p, "paragraph containing %q should exist", substr) {
		return
	}
	found := false
	for _, run := range p.runs {
		if run.TextStyle != nil && run.TextStyle.WeightedFontFamily != nil &&
			strings.EqualFold(run.TextStyle.WeightedFontFamily.FontFamily, expectedFont) {
			found = true
		}
	}
	assert.True(t, found, "paragraph %q should have font %q", substr, expectedFont)
}

func assertFontSize(t *testing.T, paras []paraInfo, substr string, expectedSize float64) {
	t.Helper()
	p := findPara(paras, substr)
	if !assert.NotNil(t, p, "paragraph containing %q should exist", substr) {
		return
	}
	found := false
	for _, run := range p.runs {
		if run.TextStyle != nil && run.TextStyle.FontSize != nil &&
			run.TextStyle.FontSize.Magnitude == expectedSize {
			found = true
		}
	}
	assert.True(t, found, "paragraph %q should have font size %v", substr, expectedSize)
}

func assertTextColor(t *testing.T, paras []paraInfo, substr string, r, g, b float64) {
	t.Helper()
	p := findPara(paras, substr)
	if !assert.NotNil(t, p, "paragraph containing %q should exist", substr) {
		return
	}
	found := false
	for _, run := range p.runs {
		if run.TextStyle != nil && run.TextStyle.ForegroundColor != nil &&
			run.TextStyle.ForegroundColor.Color != nil &&
			run.TextStyle.ForegroundColor.Color.RgbColor != nil {
			rgb := run.TextStyle.ForegroundColor.Color.RgbColor
			if colorClose(rgb.Red, r) && colorClose(rgb.Green, g) && colorClose(rgb.Blue, b) {
				found = true
			}
		}
	}
	assert.True(t, found, "paragraph %q should have color (%.1f,%.1f,%.1f)", substr, r, g, b)
}

func assertBgColor(t *testing.T, paras []paraInfo, substr string, r, g, b float64) {
	t.Helper()
	p := findPara(paras, substr)
	if !assert.NotNil(t, p, "paragraph containing %q should exist", substr) {
		return
	}
	found := false
	for _, run := range p.runs {
		if run.TextStyle != nil && run.TextStyle.BackgroundColor != nil &&
			run.TextStyle.BackgroundColor.Color != nil &&
			run.TextStyle.BackgroundColor.Color.RgbColor != nil {
			rgb := run.TextStyle.BackgroundColor.Color.RgbColor
			if colorClose(rgb.Red, r) && colorClose(rgb.Green, g) && colorClose(rgb.Blue, b) {
				found = true
			}
		}
	}
	assert.True(t, found, "paragraph %q should have bg color (%.1f,%.1f,%.1f)", substr, r, g, b)
}

func assertSuperscript(t *testing.T, paras []paraInfo, context, text string) {
	t.Helper()
	for _, p := range paras {
		for _, run := range p.runs {
			if run.TextStyle != nil && run.TextStyle.BaselineOffset == "SUPERSCRIPT" &&
				strings.Contains(strings.TrimSpace(run.Content), text) {
				if context == "" || strings.Contains(p.text, context) {
					return // found
				}
			}
		}
	}
	t.Errorf("superscript %q (context %q) not found", text, context)
}

func assertSubscript(t *testing.T, paras []paraInfo, context, text string) {
	t.Helper()
	for _, p := range paras {
		for _, run := range p.runs {
			if run.TextStyle != nil && run.TextStyle.BaselineOffset == "SUBSCRIPT" &&
				strings.Contains(strings.TrimSpace(run.Content), text) {
				if context == "" || strings.Contains(p.text, context) {
					return // found
				}
			}
		}
	}
	t.Errorf("subscript %q (context %q) not found", text, context)
}

func colorClose(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 0.05
}

// newDocsServiceFromConfig creates a Google Docs service using stored OAuth
// credentials for the given account (used by live tests only).
func newDocsServiceFromConfig(account string) (*docs.Service, error) {
	return newDocsService(context.Background(), account)
}
