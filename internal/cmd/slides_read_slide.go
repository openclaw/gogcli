package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesReadSlideCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string `arg:"" name:"slideId" help:"Slide object ID (use 'slides list-slides' to find IDs)"`
}

func (c *SlidesReadSlideCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	slideID := strings.TrimSpace(c.SlideID)
	if slideID == "" {
		return usage("empty slideId")
	}

	slidesSvc, err := newSlidesService(ctx, account)
	if err != nil {
		return err
	}

	pres, err := slidesSvc.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}

	// Find the target slide and its position
	slideIndex := -1
	for i, s := range pres.Slides {
		if s.ObjectId == slideID {
			slideIndex = i
			break
		}
	}
	if slideIndex == -1 {
		return fmt.Errorf("slide %q not found in presentation", slideID)
	}

	slide := pres.Slides[slideIndex]

	// Extract speaker notes
	var notesText string
	if np := slide.SlideProperties.NotesPage; np != nil {
		for _, el := range np.PageElements {
			if el.Shape != nil && el.Shape.Text != nil {
				if el.Shape.Placeholder != nil && el.Shape.Placeholder.Type == placeholderTypeBody {
					for _, te := range el.Shape.Text.TextElements {
						if te.TextRun != nil {
							notesText += te.TextRun.Content
						}
					}
				}
			}
		}
	}
	notesText = strings.TrimRight(notesText, "\n")

	// Extract text elements from the slide itself
	var textElements []map[string]any
	for _, el := range slide.PageElements {
		if el.Shape != nil && el.Shape.Text != nil {
			var text string
			for _, te := range el.Shape.Text.TextElements {
				if te.TextRun != nil {
					text += te.TextRun.Content
				}
			}
			text = strings.TrimRight(text, "\n")
			if text != "" {
				textElements = append(textElements, map[string]any{
					"objectId": el.ObjectId,
					"text":     text,
				})
			}
		}
	}

	// Extract image references
	var images []map[string]any
	for _, el := range slide.PageElements {
		if el.Image != nil {
			img := map[string]any{
				"objectId": el.ObjectId,
			}
			if el.Image.ContentUrl != "" {
				img["contentUrl"] = el.Image.ContentUrl
			}
			images = append(images, img)
		}
	}

	if outfmt.IsJSON(ctx) {
		result := map[string]any{
			"presentationId": presentationID,
			"slideNumber":    slideIndex + 1,
			"slideObjectId":  slideID,
			"notes":          notesText,
			"textElements":   textElements,
			"images":         images,
		}
		return outfmt.WriteJSON(ctx, os.Stdout, result)
	}

	u.Out().Printf("Slide %d  (%s)", slideIndex+1, slideID)
	u.Out().Println("")

	if notesText != "" {
		u.Out().Println("Speaker Notes:")
		u.Out().Println(notesText)
		u.Out().Println("")
	} else {
		u.Out().Println("Speaker Notes: (none)")
		u.Out().Println("")
	}

	if len(textElements) > 0 {
		u.Out().Println("Text Elements:")
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "OBJECT ID\tTEXT")
		for _, te := range textElements {
			fmt.Fprintf(tw, "%s\t%s\n", te["objectId"], te["text"])
		}
		_ = tw.Flush()
		u.Out().Println("")
	}

	if len(images) > 0 {
		u.Out().Println("Images:")
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "OBJECT ID\tURL")
		for _, img := range images {
			url := "(none)"
			if u, ok := img["contentUrl"].(string); ok {
				url = u
			}
			fmt.Fprintf(tw, "%s\t%s\n", img["objectId"], url)
		}
		_ = tw.Flush()
	}

	return nil
}
