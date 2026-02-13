package cmd

import (
	"strings"
)

// SlideLayout represents the layout type for a slide
type SlideLayout string

const (
	LayoutTitleOnly          SlideLayout = "TITLE"
	LayoutTitleAndBody       SlideLayout = "TITLE_AND_BODY"
	LayoutTitleAndTwoColumns SlideLayout = "TITLE_AND_TWO_COLUMNS"
	LayoutSectionHeader      SlideLayout = "SECTION_HEADER"
	LayoutBlank              SlideLayout = "BLANK"
)

// SlideElement represents an element on a slide
type SlideElement struct {
	Type     string // "title", "body", "bullets", "code"
	Content  string
	Items    []string // for bullet lists
	IsBold   bool
	IsItalic bool
}

// Slide represents a single slide
type Slide struct {
	Title    string
	Layout   SlideLayout
	Elements []SlideElement
}

// ParseMarkdownToSlides parses markdown into slide structures
func ParseMarkdownToSlides(markdown string) []Slide {
	var slides []Slide

	// Split by slide separators (--- on its own line)
	lines := strings.Split(markdown, "\n")
	var currentSlide strings.Builder
	inSlide := false

	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if currentSlide.Len() > 0 {
				slide := parseSlide(currentSlide.String())
				if slide.Title != "" {
					slides = append(slides, slide)
				}
				currentSlide.Reset()
			}
			inSlide = false
		} else {
			if !inSlide {
				inSlide = true
			}
			if currentSlide.Len() > 0 {
				currentSlide.WriteString("\n")
			}
			currentSlide.WriteString(line)
		}
	}

	// Handle the last slide
	if currentSlide.Len() > 0 {
		slide := parseSlide(currentSlide.String())
		if slide.Title != "" {
			slides = append(slides, slide)
		}
	}

	return slides
}

// parseSlide parses a single slide's markdown
func parseSlide(text string) Slide {
	slide := Slide{
		Layout: LayoutTitleAndBody,
	}

	lines := strings.Split(text, "\n")
	var currentElement *SlideElement
	var inCodeBlock bool
	var codeContent strings.Builder

	for _, line := range lines {
		// Handle code blocks
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End code block
				if currentElement != nil {
					currentElement.Content = codeContent.String()
					slide.Elements = append(slide.Elements, *currentElement)
				}
				inCodeBlock = false
				currentElement = nil
				codeContent.Reset()
			} else {
				// Start code block
				inCodeBlock = true
				currentElement = &SlideElement{
					Type: "code",
				}
			}
			continue
		}

		if inCodeBlock {
			if codeContent.Len() > 0 {
				codeContent.WriteString("\n")
			}
			codeContent.WriteString(line)
			continue
		}

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Title (## heading for slides)
		if strings.HasPrefix(line, "## ") {
			title := strings.TrimPrefix(line, "## ")
			// Remove formatting markers
			title = stripInlineFormatting(title)
			slide.Title = title
			slide.Elements = append(slide.Elements, SlideElement{
				Type:    "title",
				Content: title,
			})
			continue
		}

		// Bullet points
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			item := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")
			item = stripInlineFormatting(item)

			// Find or create bullets element
			var bulletsElement *SlideElement
			for i := range slide.Elements {
				if slide.Elements[i].Type == "bullets" {
					bulletsElement = &slide.Elements[i]
					break
				}
			}

			if bulletsElement == nil {
				slide.Elements = append(slide.Elements, SlideElement{
					Type:  "bullets",
					Items: []string{item},
				})
			} else {
				bulletsElement.Items = append(bulletsElement.Items, item)
			}
			continue
		}

		// Regular paragraph
		content := stripInlineFormatting(line)
		slide.Elements = append(slide.Elements, SlideElement{
			Type:    "body",
			Content: content,
		})
	}

	// Determine layout based on content
	slide.Layout = determineLayout(slide)

	return slide
}

// stripInlineFormatting removes markdown formatting from text
func stripInlineFormatting(text string) string {
	// Remove bold/italic markers
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "__", "")
	text = strings.ReplaceAll(text, "*", "")
	text = strings.ReplaceAll(text, "_", "")

	// Remove code markers
	text = strings.ReplaceAll(text, "`", "")

	// Remove links but keep text [text](url) -> text
	// Simple approach: just remove brackets and parens for now

	return text
}

// determineLayout chooses the best layout for a slide
func determineLayout(slide Slide) SlideLayout {
	hasTitle := false
	hasBullets := false
	hasBody := false
	hasCode := false

	for _, elem := range slide.Elements {
		switch elem.Type {
		case "title":
			hasTitle = true
		case "bullets":
			hasBullets = true
		case "body":
			hasBody = true
		case "code":
			hasCode = true
		}
	}

	// No title = blank layout
	if !hasTitle {
		return LayoutBlank
	}

	// Code slides often need more space
	if hasCode {
		return LayoutTitleAndBody
	}

	// Bullets or body = title + body
	if hasBullets || hasBody {
		return LayoutTitleAndBody
	}

	// Just a title = title only
	return LayoutTitleOnly
}
