package cmd

import (
	"strings"

	"google.golang.org/api/docs/v1"
)

// MarkdownToDocsRequests converts parsed markdown elements to Google Docs batch update requests
func MarkdownToDocsRequests(elements []MarkdownElement) ([]*docs.Request, string) {
	var requests []*docs.Request
	var plainText strings.Builder
	charOffset := int64(1)

	for _, el := range elements {
		startOffset := charOffset

		switch el.Type {
		case MDHeading1, MDHeading2, MDHeading3, MDHeading4, MDHeading5, MDHeading6:
			// Add heading text with newline
			plainText.WriteString(el.Content)
			plainText.WriteString("\n")
			charOffset += int64(len(el.Content) + 1)

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
					},
					Fields: "indentStart",
				},
			})

		case MDListItem, MDNumberedList:
			// Add list item text
			prefix := "• "
			if el.Type == MDNumberedList {
				prefix = "1. "
			}
			plainText.WriteString(prefix)
			plainText.WriteString(el.Content)
			plainText.WriteString("\n")
			charOffset += int64(len(prefix) + len(el.Content) + 1)

		case MDHorizontalRule:
			// Add horizontal rule as a separator line
			separator := "────────────────────────────────────────"
			plainText.WriteString(separator)
			plainText.WriteString("\n")
			charOffset += int64(len(separator) + 1)

		case MDParagraph:
			// Add paragraph text
			plainText.WriteString(el.Content)
			plainText.WriteString("\n")
			charOffset += int64(len(el.Content) + 1)
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
