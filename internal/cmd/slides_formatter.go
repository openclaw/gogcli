package cmd

import (
	"fmt"
	"strings"

	"google.golang.org/api/slides/v1"
)

// SlidesToAPIRequests converts slide structures to Google Slides API batch update requests
func SlidesToAPIRequests(slideData []Slide) ([]*slides.Request, map[int]string) {
	var requests []*slides.Request
	slideIDs := make(map[int]string)

	for i, slide := range slideData {
		slideID := fmt.Sprintf("slide_%d", i+1)
		slideIDs[i] = slideID

		// Create blank slide
		requests = append(requests, &slides.Request{
			CreateSlide: &slides.CreateSlideRequest{
				ObjectId: slideID,
				SlideLayoutReference: &slides.LayoutReference{
					PredefinedLayout: "BLANK",
				},
			},
		})

		// Add title box
		titleID := fmt.Sprintf("title_%d", i+1)
		requests = append(requests, &slides.Request{
			CreateShape: &slides.CreateShapeRequest{
				ObjectId:  titleID,
				ShapeType: "TEXT_BOX",
				ElementProperties: &slides.PageElementProperties{
					PageObjectId: slideID,
					Transform: &slides.AffineTransform{
						ScaleX:     1,
						ScaleY:     1,
						TranslateX: 72 * 0.5, // 0.5 inches from left
						TranslateY: 72 * 0.5, // 0.5 inches from top
						Unit:       "PT",
					},
					Size: &slides.Size{
						Width:  &slides.Dimension{Magnitude: 612 - 72, Unit: "PT"},
						Height: &slides.Dimension{Magnitude: 100, Unit: "PT"},
					},
				},
			},
		})

		// Add title text
		for _, elem := range slide.Elements {
			if elem.Type == "title" {
				requests = append(requests, &slides.Request{
					InsertText: &slides.InsertTextRequest{
						ObjectId:       titleID,
						Text:           elem.Content,
						InsertionIndex: 0,
					},
				})

				// Make title bold
				requests = append(requests, &slides.Request{
					UpdateTextStyle: &slides.UpdateTextStyleRequest{
						ObjectId: titleID,
						TextRange: &slides.Range{
							Type: "ALL",
						},
						Style: &slides.TextStyle{
							Bold: true,
							FontSize: &slides.Dimension{
								Magnitude: 36,
								Unit:      "PT",
							},
						},
						Fields: "bold,fontSize",
					},
				})
			}
		}

		// Add body box
		bodyID := fmt.Sprintf("body_%d", i+1)
		requests = append(requests, &slides.Request{
			CreateShape: &slides.CreateShapeRequest{
				ObjectId:  bodyID,
				ShapeType: "TEXT_BOX",
				ElementProperties: &slides.PageElementProperties{
					PageObjectId: slideID,
					Transform: &slides.AffineTransform{
						ScaleX:     1,
						ScaleY:     1,
						TranslateX: 72 * 0.5,
						TranslateY: 72 * 1.5, // Below title
						Unit:       "PT",
					},
					Size: &slides.Size{
						Width:  &slides.Dimension{Magnitude: 612 - 72, Unit: "PT"},
						Height: &slides.Dimension{Magnitude: 300, Unit: "PT"},
					},
				},
			},
		})

		// Build body content
		var bodyContent strings.Builder
		for _, elem := range slide.Elements {
			if elem.Type != "title" {
				switch elem.Type {
				case "body":
					bodyContent.WriteString(elem.Content)
					bodyContent.WriteString("\n")
				case "bullets":
					for _, item := range elem.Items {
						bodyContent.WriteString("• ")
						bodyContent.WriteString(item)
						bodyContent.WriteString("\n")
					}
				case "code":
					bodyContent.WriteString("```\n")
					bodyContent.WriteString(elem.Content)
					bodyContent.WriteString("\n```\n")
				}
			}
		}

		// Add body text if there's content
		if bodyContent.Len() > 0 {
			requests = append(requests, &slides.Request{
				InsertText: &slides.InsertTextRequest{
					ObjectId:       bodyID,
					Text:           bodyContent.String(),
					InsertionIndex: 0,
				},
			})
		}
	}

	return requests, slideIDs
}

// CreatePresentationFromMarkdown creates a Google Slides presentation from markdown
func CreatePresentationFromMarkdown(title string, markdown string, service *slides.Service) (*slides.Presentation, error) {
	// Parse markdown to slides
	slidesData := ParseMarkdownToSlides(markdown)

	if len(slidesData) == 0 {
		return nil, fmt.Errorf("no slides found in markdown")
	}

	// Create presentation
	presentation, err := service.Presentations.Create(&slides.Presentation{
		Title: title,
	}).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to create presentation: %w", err)
	}

	// Convert to API requests
	requests, slideIDs := SlidesToAPIRequests(slidesData)

	// Execute batch update
	if len(requests) > 0 {
		_, err = service.Presentations.BatchUpdate(presentation.PresentationId, &slides.BatchUpdatePresentationRequest{
			Requests: requests,
		}).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to populate slides: %w", err)
		}
	}

	// Debug output
	if debugSlides {
		fmt.Printf("[DEBUG] Created presentation with %d slides\n", len(slidesData))
		for i, slideID := range slideIDs {
			fmt.Printf("  Slide %d: %s - %s\n", i+1, slideID, slidesData[i].Title)
		}
	}

	return presentation, nil
}
