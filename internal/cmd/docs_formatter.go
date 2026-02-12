package cmd

import (
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"
)

// Debug flag for markdown formatter
var debugMarkdown = false

// MarkdownToDocsRequests converts parsed markdown elements to Google Docs batch update requests
func MarkdownToDocsRequests(elements []MarkdownElement) ([]*docs.Request, string) {
	var requests []*docs.Request
	var plainText strings.Builder
	charOffset := int64(1)

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
			charOffset += prefixLen + utf16Len(strippedContent + "\n")

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
			// Handle markdown table - convert to formatted text representation
			// Google Docs API table insertion is complex and requires multiple API calls
			// For now, render as formatted text table
			if len(el.TableCells) == 0 {
				continue
			}

			rows := len(el.TableCells)
			cols := len(el.TableCells[0])
			if rows == 0 || cols == 0 {
				continue
			}

			if debugMarkdown {
				fmt.Printf("[TABLE] %d rows x %d cols - rendering as formatted text\n", rows, cols)
			}

			// Calculate column widths
			colWidths := make([]int, cols)
			for _, row := range el.TableCells {
				for colIdx, cell := range row {
					if colIdx < len(colWidths) && len(cell) > colWidths[colIdx] {
						colWidths[colIdx] = len(cell)
					}
				}
			}

			// Render table as text
			for rowIdx, row := range el.TableCells {
				var line strings.Builder
				for colIdx, cell := range row {
					if colIdx < len(colWidths) {
						// Pad cell to column width
						padding := colWidths[colIdx] - len(cell)
						line.WriteString("| ")
						line.WriteString(cell)
						line.WriteString(strings.Repeat(" ", padding))
						line.WriteString(" ")
					}
				}
				line.WriteString("|")

				plainText.WriteString(line.String())
				plainText.WriteString("\n")
				charOffset += utf16Len(line.String() + "\n")

				// Add separator after header row
				if rowIdx == 0 {
					var sep strings.Builder
					for colIdx := 0; colIdx < cols; colIdx++ {
						if colIdx < len(colWidths) {
							sep.WriteString("|-")
							sep.WriteString(strings.Repeat("-", colWidths[colIdx]))
							sep.WriteString("-")
						}
					}
					sep.WriteString("|")

					plainText.WriteString(sep.String())
					plainText.WriteString("\n")
					charOffset += utf16Len(sep.String() + "\n")
				}
			}

			// Add empty line after table
			plainText.WriteString("\n")
			charOffset += utf16Len("\n")
		}
	}

	if debugMarkdown {
		fmt.Printf("\n[FINAL] plainText length: %d\n", plainText.Len())
		fmt.Printf("[FINAL] Final charOffset: %d\n", charOffset)
		fmt.Printf("[FINAL] Total requests: %d\n", len(requests))
		fmt.Printf("\n[FINAL] plainText content:\n%s\n[END]\n", plainText.String())
		fmt.Printf("[FINAL] plainText bytes: %v\n", []byte(plainText.String()))
	}

	return requests, plainText.String()
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

// buildTextStyleFromStyle creates a docs.TextStyle from a TextStyle
func buildTextStyleFromStyle(style TextStyle) *docs.TextStyle {
	textStyle := &docs.TextStyle{}
	
	if style.Bold {
		textStyle.Bold = true
	}
	if style.Italic {
		textStyle.Italic = true
	}
	if style.Code {
		textStyle.WeightedFontFamily = &docs.WeightedFontFamily{
			FontFamily: "Courier New",
			Weight:     400,
		}
	}
	if style.Link != "" {
		textStyle.Link = &docs.Link{
			Url: style.Link,
		}
	}
	
	return textStyle
}

// getStyleFields returns the fields string for a TextStyle
func getStyleFields(style TextStyle) string {
	var fields []string
	
	if style.Bold {
		fields = append(fields, "bold")
	}
	if style.Italic {
		fields = append(fields, "italic")
	}
	if style.Code {
		fields = append(fields, "weightedFontFamily")
	}
	if style.Link != "" {
		fields = append(fields, "link")
	}
	
	return strings.Join(fields, ",")
}
