package cmd

import (
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"
)

// docsSoftLineBreak is the Google Docs InsertText character for a line break
// inside the current paragraph. Live Docs API readback returns it inside the
// same textRun, which lets fenced code blocks keep one shaded paragraph.
const docsSoftLineBreak = "\v"

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
func MarkdownToDocsRequests(elements []MarkdownElement, baseIndex int64, tabID string) ([]*docs.Request, string, []TableData) {
	var requests []*docs.Request
	var plainText strings.Builder
	var tables []TableData
	charOffset := baseIndex

	if debugMarkdown {
		fmt.Printf("[DEBUG] Starting MarkdownToDocsRequests with %d elements\n", len(elements))
	}

	for i := 0; i < len(elements); i++ {
		el := elements[i]
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
						TabId:      tabID,
					},
					ParagraphStyle: &docs.ParagraphStyle{
						NamedStyleType: headingStyle,
					},
					Fields: "namedStyleType",
				},
			})

			// Apply inline text styles
			for _, style := range styles {
				textStyleReq := buildTextStyleRequest(style, startOffset, tabID)
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
			// Render the fenced code block as a single contiguous paragraph.
			// Embedded line breaks become soft line breaks (vertical tab) so
			// Docs keeps them inside one paragraph, which lets us apply a
			// single paragraph-level background shading across the whole block
			// instead of emitting one Courier-styled paragraph per source line.
			// See #594.
			codeBody := strings.ReplaceAll(el.Content, "\n", docsSoftLineBreak)
			codeContent := codeBody + "\n"
			plainText.WriteString(codeContent)
			charOffset += utf16Len(codeContent)

			// Apply monospace font to entire code block text run.
			requests = append(requests, &docs.Request{
				UpdateTextStyle: &docs.UpdateTextStyleRequest{
					Range: &docs.Range{
						StartIndex: startOffset,
						EndIndex:   charOffset,
						TabId:      tabID,
					},
					TextStyle: &docs.TextStyle{
						WeightedFontFamily: &docs.WeightedFontFamily{
							FontFamily: "Courier New",
							Weight:     400,
						},
					},
					Fields: "weightedFontFamily",
				},
			})

			// Apply a paragraph-level light-grey background so the whole
			// fenced block renders as one shaded code block, matching the
			// output of `docs create --file --markdown`. Without this the
			// block looks like plain Courier text on the default background.
			requests = append(requests, &docs.Request{
				UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
					Range: &docs.Range{
						StartIndex: startOffset,
						EndIndex:   charOffset,
						TabId:      tabID,
					},
					ParagraphStyle: &docs.ParagraphStyle{
						Shading: &docs.Shading{
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
					},
					Fields: "shading.backgroundColor",
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
						TabId:      tabID,
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
				textStyleReq := buildTextStyleRequest(style, startOffset, tabID)
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
			blockEnd := startOffset
			bulletPreset := bulletPresetDisc
			if el.Type == MDNumberedList {
				bulletPreset = bulletPresetNumbered
			}
			blockType := el.Type
			var listPresetRequests []*docs.Request
			var listStyleRequests []*docs.Request

			for ; i < len(elements); i++ {
				el = elements[i]
				if el.Type != MDListItem && el.Type != MDNumberedList {
					i--
					break
				}
				if el.Type != blockType && el.Level == 0 {
					i--
					break
				}

				styles, strippedContent := ParseInlineFormatting(el.Content)
				leadingTabs := strings.Repeat("\t", el.Level)
				itemStart := charOffset
				itemEnd := itemStart + utf16Len(strippedContent+"\n")

				if debugMarkdown {
					fmt.Printf("[LIST] Content: %q -> stripped=%q styles=%d\n", el.Content, strippedContent, len(styles))
				}

				// Emit list items as bare paragraphs with leading tabs for
				// nesting, then promote the whole contiguous list block to a
				// native Google Docs list. Keeping the range whole is what
				// preserves Docs nesting levels; mixed child marker kinds get a
				// later preset override using post-tab-removal item ranges.
				// CreateParagraphBullets consumes those tabs, so inline styles
				// below use post-consumption itemStart offsets.
				listText := leadingTabs + strippedContent + "\n"
				plainText.WriteString(listText)
				blockEnd += utf16Len(listText)

				if el.Type != blockType {
					itemPreset := bulletPresetDisc
					if el.Type == MDNumberedList {
						itemPreset = bulletPresetNumbered
					}
					listPresetRequests = append(listPresetRequests, &docs.Request{
						CreateParagraphBullets: &docs.CreateParagraphBulletsRequest{
							Range: &docs.Range{
								StartIndex: itemStart,
								EndIndex:   itemEnd,
								TabId:      tabID,
							},
							BulletPreset: itemPreset,
						},
					})
				}

				for _, style := range styles {
					textStyleReq := buildTextStyleRequest(style, itemStart, tabID)
					if textStyleReq != nil {
						listStyleRequests = append(listStyleRequests, textStyleReq)
					}
				}
				charOffset = itemEnd
			}

			requests = append(requests, &docs.Request{
				CreateParagraphBullets: &docs.CreateParagraphBulletsRequest{
					Range: &docs.Range{
						StartIndex: startOffset,
						EndIndex:   blockEnd,
						TabId:      tabID,
					},
					BulletPreset: bulletPreset,
				},
			})
			requests = append(requests, listPresetRequests...)
			requests = append(requests, listStyleRequests...)

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
				textStyleReq := buildTextStyleRequest(style, startOffset, tabID)
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
			// Native tables and heading styles already supply visual spacing.
			// Emitting an adjacent source blank line would double those gaps.
			if (i > 0 && (elements[i-1].Type == MDTable || isMarkdownHeadingElement(elements[i-1].Type))) ||
				(i+1 < len(elements) && (elements[i+1].Type == MDTable || isMarkdownHeadingElement(elements[i+1].Type))) {
				continue
			}
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
func buildTextStyleRequest(style TextStyle, baseOffset int64, tabID string) *docs.Request {
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
	if style.Strikethrough {
		textStyle.Strikethrough = true
		fields = append(fields, "strikethrough")
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
				TabId:      tabID,
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
