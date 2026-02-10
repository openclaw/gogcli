package cmd

import (
	"strings"

	"google.golang.org/api/docs/v1"
)

// MarkdownToDocsRequests converts parsed markdown elements to Google Docs batch update requests
func MarkdownToDocsRequests(elements []MarkdownElement) ([]*docs.Request, string) {
	var requests []*docs.Request
	var plainText strings.Builder
	charOffset := int64(1) // Docs indices start at 1

	for i, el := range elements {
		startOffset := charOffset

		switch el.Type {
		case MDHeading1, MDHeading2, MDHeading3, MDHeading4, MDHeading5, MDHeading6:
			// Add heading text with newline
			plainText.WriteString(el.Content)
			plainText.WriteString("\n")
			charOffset += int64(len(el.Content) + 1)

			// Apply heading style after we know the position
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

			// Parse and apply inline formatting (simplified - skip for now)
			// TODO: Fix inline formatting indices


		case MDCodeBlock:
			// Add code block text
			codeContent := el.Content + "\n"
			plainText.WriteString(codeContent)
			charOffset += int64(len(codeContent))

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
			// Add blockquote text
			plainText.WriteString(el.Content)
			plainText.WriteString("\n")
			charOffset += int64(len(el.Content) + 1)

			// Apply blockquote style (indent + italic)
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
						IndentFirstLine: &docs.Dimension{
							Magnitude: 0,
							Unit:      "PT",
						},
					},
					Fields: "indentStart,indentFirstLine",
				},
			})
			requests = append(requests, &docs.Request{
				UpdateTextStyle: &docs.UpdateTextStyleRequest{
					Range: &docs.Range{
						StartIndex: startOffset,
						EndIndex:   charOffset,
					},
					TextStyle: &docs.TextStyle{
						Italic: true,
					},
					Fields: "italic",
				},
			})

		case MDListItem, MDNumberedList:
			// Add list item text
			plainText.WriteString(el.Content)
			plainText.WriteString("\n")
			charOffset += int64(len(el.Content) + 1)

			// Inline formatting: TODO - fix indices
			_ = el.Content

		case MDHorizontalRule:
			// Add horizontal rule as a line
			plainText.WriteString("────────────────────────────────────────\n")
			charOffset += 41

		case MDParagraph:
			// Add paragraph text
			plainText.WriteString(el.Content)
			plainText.WriteString("\n")
			charOffset += int64(len(el.Content) + 1)

			// Inline formatting: TODO - fix indices
			_ = el.Content
		}

		// Add spacing between elements (except for last one)
		if i < len(elements)-1 && el.Type != MDHorizontalRule {
			plainText.WriteString("\n")
			charOffset += 1
		}
	}

	return requests, plainText.String()
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

func createTextStyleRequest(start, end int64, style TextStyle) *docs.Request {
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

	fields := ""
	if style.Bold {
		fields += "bold,"
	}
	if style.Italic {
		fields += "italic,"
	}
	if style.Code {
		fields += "weightedFontFamily,"
	}
	fields = strings.TrimSuffix(fields, ",")

	return &docs.Request{
		UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{
				StartIndex: start,
				EndIndex:   end,
			},
			TextStyle: textStyle,
			Fields:    fields,
		},
	}
}

func createLinkRequest(start, end int64, url string) *docs.Request {
	return &docs.Request{
		UpdateTextStyle: &docs.UpdateTextStyleRequest{
			Range: &docs.Range{
				StartIndex: start,
				EndIndex:   end,
			},
			TextStyle: &docs.TextStyle{
				Link: &docs.Link{
					Url: url,
				},
				Underline: true,
				ForegroundColor: &docs.OptionalColor{
					Color: &docs.Color{
						RgbColor: &docs.RgbColor{
							Red:   0.07,
							Green: 0.30,
							Blue:  0.78,
						},
					},
				},
			},
			Fields: "link,underline,foregroundColor",
		},
	}
}
