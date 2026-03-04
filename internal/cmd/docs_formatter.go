package cmd

import (
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"
)

// Debug flag for markdown formatter
var debugMarkdown = false

// TableData represents a table to be inserted natively
type TableData struct {
	StartIndex int64
	Cells      [][]string
}

// MarkdownToDocsRequests converts parsed markdown elements to Google Docs batch
// update requests. baseIndex is the insertion location in the document.
// Returns: requests, plainText, tableData (for native table insertion)
func MarkdownToDocsRequests(elements []MarkdownElement, baseIndex int64) ([]*docs.Request, string, []TableData) {
	var requests []*docs.Request
	var plainText strings.Builder
	var tables []TableData
	charOffset := baseIndex

	if debugMarkdown {
		fmt.Printf("[DEBUG] Starting MarkdownToDocsRequests with %d elements\n", len(elements))
	}

	for _, el := range elements {
		startOffset := charOffset

		switch el.Type {
		case MDHeading1, MDHeading2, MDHeading3, MDHeading4, MDHeading5, MDHeading6:
			// Parse inline formatting for heading content
			styles, strippedContent := ParseInlineFormatting(el.Content)

			if debugMarkdown {
				fmt.Printf("[DEBUG] Heading: content=%q stripped=%q styles=%d\n", el.Content, strippedContent, len(styles))
			}

			if debugMarkdown {
				fmt.Printf("[HEADING] Content: %q\n", el.Content)
				fmt.Printf("  Stripped: %q (len=%d)\n", strippedContent, len(strippedContent))
				fmt.Printf("  Styles: %v\n", styles)
			}

			// Add stripped heading text with newline
			plainText.WriteString(strippedContent)
			plainText.WriteString("\n")
			charOffset += utf16Len(strippedContent + "\n")

			// Apply heading style
			headingStyle := getHeadingStyle(el.Type)
			requests = append(requests, &docs.Request{
				UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
					Range: &docs.Range{
						StartIndex: startOffset,
						EndIndex:   charOffset,
					},
					ParagraphStyle: &docs.ParagraphStyle{
						NamedStyleType: headingStyle,
					},
					Fields: "namedStyleType",
				},
			})

			// Apply inline text styles
			for _, style := range styles {
				textStyleReq := buildTextStyleRequest(style, startOffset)
				if textStyleReq != nil {
					if debugMarkdown {
						fmt.Printf("  Style request: [%d, %d]\n",
							textStyleReq.UpdateTextStyle.Range.StartIndex,
							textStyleReq.UpdateTextStyle.Range.EndIndex)
					}
					requests = append(requests, textStyleReq)
				}
			}

		case MDCodeBlock:
			// Add code block text (no inline formatting in code blocks)
			codeContent := el.Content + "\n"
			plainText.WriteString(codeContent)
			charOffset += utf16Len(codeContent)

			// Apply monospace font to entire code block
			requests = append(requests, &docs.Request{
				UpdateTextStyle: &docs.UpdateTextStyleRequest{
					Range: &docs.Range{
						StartIndex: startOffset,
						EndIndex:   charOffset,
					},
					TextStyle: &docs.TextStyle{
						WeightedFontFamily: &docs.WeightedFontFamily{
							FontFamily: "Courier New",
							Weight:     400,
						},
						BackgroundColor: &docs.OptionalColor{
							Color: &docs.Color{
								RgbColor: &docs.RgbColor{
									Red:   0.95,
									Green: 0.95,
									Blue:  0.95,
								},
							},
						},
					},
					Fields: "weightedFontFamily,backgroundColor",
				},
			})

		case MDBlockquote:
			// Parse inline formatting for blockquote content
			styles, strippedContent := ParseInlineFormatting(el.Content)

			if debugMarkdown {
				fmt.Printf("[BLOCKQUOTE] Content: %q -> stripped=%q\n", el.Content, strippedContent)
			}

			// Add stripped blockquote text
			plainText.WriteString(strippedContent)
			plainText.WriteString("\n")
			charOffset += utf16Len(strippedContent + "\n")

			// Apply blockquote style (indent)
			requests = append(requests, &docs.Request{
				UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
					Range: &docs.Range{
						StartIndex: startOffset,
						EndIndex:   charOffset,
					},
					ParagraphStyle: &docs.ParagraphStyle{
						IndentStart: &docs.Dimension{
							Magnitude: 36,
							Unit:      "PT",
						},
					},
					Fields: "indentStart",
				},
			})

			// Apply inline text styles
			for _, style := range styles {
				textStyleReq := buildTextStyleRequest(style, startOffset)
				if textStyleReq != nil {
					if debugMarkdown {
						fmt.Printf("  Style request: [%d, %d] (base=%d, style=[%d,%d])\n",
							textStyleReq.UpdateTextStyle.Range.StartIndex,
							textStyleReq.UpdateTextStyle.Range.EndIndex,
							startOffset, style.Start, style.End)
					}
					requests = append(requests, textStyleReq)
				}
			}

		case MDListItem, MDNumberedList:
			// Parse inline formatting for list item content
			styles, strippedContent := ParseInlineFormatting(el.Content)

			if debugMarkdown {
				fmt.Printf("[LIST] Content: %q -> stripped=%q styles=%d\n", el.Content, strippedContent, len(styles))
			}

			// Add list item with prefix
			prefix := "• "
			if el.Type == MDNumberedList {
				prefix = "1. "
			}
			prefixLen := utf16Len(prefix)
			plainText.WriteString(prefix)
			plainText.WriteString(strippedContent)
			plainText.WriteString("\n")
			charOffset += prefixLen + utf16Len(strippedContent+"\n")

			// Apply inline text styles (offset by prefix length)
			for _, style := range styles {
				textStyleReq := buildTextStyleRequest(style, startOffset+prefixLen)
				if textStyleReq != nil {
					requests = append(requests, textStyleReq)
				}
			}

		case MDHorizontalRule:
			// Add horizontal rule as a separator line using ASCII dashes
			separator := strings.Repeat("-", 40)
			plainText.WriteString(separator)
			plainText.WriteString("\n")
			charOffset += utf16Len(separator + "\n")

		case MDParagraph:
			// Parse inline formatting for paragraph content
			styles, strippedContent := ParseInlineFormatting(el.Content)

			if debugMarkdown {
				fmt.Printf("[PARAGRAPH] Content: %q\n", el.Content)
				fmt.Printf("  Stripped: %q (len=%d)\n", strippedContent, len(strippedContent))
				fmt.Printf("  Styles: %v\n", styles)
				fmt.Printf("  startOffset: %d, len+1: %d\n", startOffset, len(strippedContent)+1)
			}

			// Add stripped paragraph text
			plainText.WriteString(strippedContent)
			plainText.WriteString("\n")
			charOffset += utf16Len(strippedContent + "\n")

			if debugMarkdown {
				fmt.Printf("  charOffset after: %d, plainText.Len: %d\n", charOffset, plainText.Len())
			}

			// Apply inline text styles
			for _, style := range styles {
				textStyleReq := buildTextStyleRequest(style, startOffset)
				if textStyleReq != nil {
					if debugMarkdown {
						fmt.Printf("  Style request: [%d, %d]\n",
							textStyleReq.UpdateTextStyle.Range.StartIndex,
							textStyleReq.UpdateTextStyle.Range.EndIndex)
					}
					requests = append(requests, textStyleReq)
				}
			}

		case MDEmptyLine:
			// Add empty line
			plainText.WriteString("\n")
			charOffset += utf16Len("\n")

		case MDTable:
			// Handle markdown table - save for native insertion
			if len(el.TableCells) == 0 {
				continue
			}

			rows := len(el.TableCells)
			cols := len(el.TableCells[0])
			if rows == 0 || cols == 0 {
				continue
			}

			if debugMarkdown {
				fmt.Printf("[TABLE] %d rows x %d cols at offset %d - saving for native insertion\n", rows, cols, charOffset)
			}

			// Save table data for native insertion
			tables = append(tables, TableData{
				StartIndex: charOffset,
				Cells:      el.TableCells,
			})

			// Add a placeholder newline (table will be inserted here)
			plainText.WriteString("\n")
			charOffset += utf16Len("\n")
		}
	}

	if debugMarkdown {
		fmt.Printf("\n[FINAL] plainText length: %d\n", plainText.Len())
		fmt.Printf("[FINAL] Final charOffset: %d\n", charOffset)
		fmt.Printf("[FINAL] Total requests: %d\n", len(requests))
		fmt.Printf("[FINAL] Total tables: %d\n", len(tables))
		fmt.Printf("\n[FINAL] plainText content:\n%s\n[END]\n", plainText.String())
	}

	return requests, plainText.String(), tables
}

// buildTextStyleRequest creates a text style update request from a TextStyle
func buildTextStyleRequest(style TextStyle, baseOffset int64) *docs.Request {
	// Validate indices
	if style.Start < 0 || style.End < 0 || style.End <= style.Start {
		return nil
	}

	textStyle := &docs.TextStyle{}
	var fields []string

	if style.Bold {
		textStyle.Bold = true
		fields = append(fields, "bold")
	}
	if style.Italic {
		textStyle.Italic = true
		fields = append(fields, "italic")
	}
	if style.Code {
		textStyle.WeightedFontFamily = &docs.WeightedFontFamily{
			FontFamily: "Courier New",
			Weight:     400,
		}
		fields = append(fields, "weightedFontFamily")
	}
	if style.Link != "" {
		textStyle.Link = &docs.Link{
			Url: style.Link,
		}
		fields = append(fields, "link")
	}

	if len(fields) == 0 {
		return nil
	}

	return &docs.Request{
		UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{
				StartIndex: baseOffset + int64(style.Start),
				EndIndex:   baseOffset + int64(style.End),
			},
			TextStyle: textStyle,
			Fields:    strings.Join(fields, ","),
		},
	}
}

// ParseTextColor parses a color string and returns an OptionalColor.
// Accepts hex colors (#RRGGBB, #RGB) and named colors (red, blue, green, etc.).
func ParseTextColor(color string) (*docs.OptionalColor, error) {
	color = strings.TrimSpace(strings.ToLower(color))
	if color == "" {
		return nil, fmt.Errorf("empty color")
	}

	// Named colors
	namedColors := map[string][3]float64{
		"red":     {1.0, 0.0, 0.0},
		"green":   {0.0, 0.5, 0.0},
		"blue":    {0.0, 0.0, 1.0},
		"black":   {0.0, 0.0, 0.0},
		"white":   {1.0, 1.0, 1.0},
		"gray":    {0.5, 0.5, 0.5},
		"grey":    {0.5, 0.5, 0.5},
		"orange":  {1.0, 0.647, 0.0},
		"purple":  {0.5, 0.0, 0.5},
		"cyan":    {0.0, 1.0, 1.0},
		"magenta": {1.0, 0.0, 1.0},
		"yellow":  {1.0, 1.0, 0.0},
		"brown":   {0.6, 0.3, 0.0},
		"teal":    {0.0, 0.5, 0.5},
		"navy":    {0.0, 0.0, 0.5},
	}

	if rgb, ok := namedColors[color]; ok {
		return &docs.OptionalColor{
			Color: &docs.Color{
				RgbColor: &docs.RgbColor{
					Red:   rgb[0],
					Green: rgb[1],
					Blue:  rgb[2],
				},
			},
		}, nil
	}

	// Hex colors
	hex := strings.TrimPrefix(color, "#")
	var r, g, b uint8
	switch len(hex) {
	case 3:
		_, err := fmt.Sscanf(hex, "%1x%1x%1x", &r, &g, &b)
		if err != nil {
			return nil, fmt.Errorf("invalid hex color %q: %w", color, err)
		}
		r = r*16 + r
		g = g*16 + g
		b = b*16 + b
	case 6:
		_, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
		if err != nil {
			return nil, fmt.Errorf("invalid hex color %q: %w", color, err)
		}
	default:
		return nil, fmt.Errorf("unknown color %q (use hex #RRGGBB or name: red, blue, green, ...)", color)
	}

	return &docs.OptionalColor{
		Color: &docs.Color{
			RgbColor: &docs.RgbColor{
				Red:   float64(r) / 255.0,
				Green: float64(g) / 255.0,
				Blue:  float64(b) / 255.0,
			},
		},
	}, nil
}

// BuildColorRequest creates an UpdateTextStyleRequest that sets the foreground
// color of text in the given range.
func BuildColorRequest(startIndex, endIndex int64, color *docs.OptionalColor) *docs.Request {
	return &docs.Request{
		UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{
				StartIndex: startIndex,
				EndIndex:   endIndex,
			},
			TextStyle: &docs.TextStyle{
				ForegroundColor: color,
			},
			Fields: "foregroundColor",
		},
	}
}

func getHeadingStyle(elType MarkdownElementType) string {
	switch elType {
	case MDHeading1:
		return "HEADING_1"
	case MDHeading2:
		return "HEADING_2"
	case MDHeading3:
		return "HEADING_3"
	case MDHeading4:
		return "HEADING_4"
	case MDHeading5:
		return "HEADING_5"
	case MDHeading6:
		return "HEADING_6"
	default:
		return "NORMAL_TEXT"
	}
}
