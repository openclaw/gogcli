package cmd

import (
	"fmt"
	"strings"

	"google.golang.org/api/slides/v1"
)

// SlidesToAPIRequests converts slide structures to Google Slides API batch update requests
func SlidesToAPIRequests(slides []Slide) ([]*slides.Request, map[int]string) {
	var requests []*slides.Request
	slideIDs := make(map[int]string)
	
	for i, slide := range slides {
		slideID := fmt.Sprintf("slide_%d", i+1)
		slideIDs[i] = slideID
		
		// Create slide
		requests = append(requests, &slides.Request{
			CreateSlide: &slides.CreateSlideRequest{
				ObjectId: slideID,
				SlideLayoutReference: &slides.LayoutReference{
					PredefinedLayout: string(slide.Layout),
				},
			},
		})
		
		// Add text elements
		for _, elem := range slide.Elements {
			switch elem.Type {
			case "title":
				// Insert title text
				requests = append(requests, &slides.Request{
					InsertText: &slides.InsertTextRequest{
						ObjectId: slideID,
						Text:     elem.Content,
					},
				})
				
			case "bullets":
				// Insert bullet points
				text := strings.Join(elem.Items, "\n")
				requests = append(requests, &slides.Request{
					InsertText: &slides.InsertTextRequest{
						ObjectId: slideID,
						Text:     text,
					},
				})
				
				// Create bullet list formatting
				for idx, item := range elem.Items {
					startIndex := int64(0)
					for j := 0; j < idx; j++ {
						startIndex += int64(len(elem.Items[j]) + 1) // +1 for newline
					}
					endIndex := startIndex + int64(len(item))
					
					requests = append(requests, &slides.Request{
						CreateParagraphBullets: &slides.CreateParagraphBulletsRequest{
							ObjectId: slideID,
							TextRange: &slides.Range{
								Type:       "FIXED_RANGE",
								StartIndex: &startIndex,
								EndIndex:   &endIndex,
							},
							BulletPreset: "BULLET_DISC_CIRCLE_SQUARE",
						},
					})
				}
				
			case "body":
				// Insert body text
				requests = append(requests, &slides.Request{
					InsertText: &slides.InsertTextRequest{
						ObjectId: slideID,
						Text:     elem.Content,
					},
				})
				
			case "code":
				// Insert code as monospace text
				requests = append(requests, &slides.Request{
					InsertText: &slides.InsertTextRequest{
						ObjectId: slideID,
						Text:     elem.Content,
					},
				})
				
				// Apply monospace font
				startIndex := int64(0)
				endIndex := int64(len(elem.Content))
				
				requests = append(requests, &slides.Request{
					UpdateTextStyle: &slides.UpdateTextStyleRequest{
						ObjectId: slideID,
						TextRange: &slides.Range{
							Type:       "FIXED_RANGE",
							StartIndex: &startIndex,
							EndIndex:   &endIndex,
						},
						Style: &slides.TextStyle{
							FontFamily: "Courier New",
						},
						Fields: "fontFamily",
					},
				})
			}
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
