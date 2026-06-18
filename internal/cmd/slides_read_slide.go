package cmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesReadSlideCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string `arg:"" name:"slideId" help:"Slide object ID (use 'slides list-slides' to find IDs)"`
	Detail         bool   `name:"detail" help:"Include normalized element geometry, text runs/styles, paragraphs, and table cells"`
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

	slidesSvc, err := slidesService(ctx, account)
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
	if slide.SlideProperties != nil && slide.SlideProperties.NotesPage != nil {
		np := slide.SlideProperties.NotesPage
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

	elements := slidesElementDetails(slide.PageElements)
	textElements, images, tables := slidesReadSlideSummaries(elements)

	if outfmt.IsJSON(ctx) {
		result := map[string]any{
			"presentationId": presentationID,
			"slideNumber":    slideIndex + 1,
			"slideObjectId":  slideID,
			"notes":          notesText,
			"textElements":   textElements,
			"images":         images,
			"tables":         tables,
		}
		if c.Detail {
			result["elements"] = elements
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}

	u.Out().Linef("Slide %d  (%s)", slideIndex+1, slideID)
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
		tw := tabwriter.NewWriter(stdoutWriter(ctx), 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "OBJECT ID\tTEXT")
		for _, te := range textElements {
			fmt.Fprintf(tw, "%s\t%s\n", te["objectId"], te["text"])
		}
		_ = tw.Flush()
		u.Out().Println("")
	}

	if len(images) > 0 {
		u.Out().Println("Images:")
		tw := tabwriter.NewWriter(stdoutWriter(ctx), 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "OBJECT ID\tSOURCE URL\tCONTENT URL")
		for _, img := range images {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", img["objectId"], valueOrNone(img["sourceUrl"]), valueOrNone(img["contentUrl"]))
		}
		_ = tw.Flush()
		u.Out().Println("")
	}

	if len(tables) > 0 {
		u.Out().Println("Table Cells:")
		tw := tabwriter.NewWriter(stdoutWriter(ctx), 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "OBJECT ID\tROW\tCOLUMN\tTEXT")
		for _, table := range tables {
			cells, _ := table["cells"].([]map[string]any)
			for _, cell := range cells {
				fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", table["objectId"], cell["rowIndex"], cell["columnIndex"], cell["text"])
			}
		}
		_ = tw.Flush()
		u.Out().Println("")
	}

	if c.Detail && len(elements) > 0 {
		u.Out().Println("Elements:")
		tw := tabwriter.NewWriter(stdoutWriter(ctx), 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "OBJECT ID\tTYPE\tX\tY\tWIDTH\tHEIGHT")
		for _, element := range elements {
			if element.Geometry == nil {
				fmt.Fprintf(tw, "%s\t%s\t-\t-\t-\t-\n", element.ObjectID, element.Type)
				continue
			}
			fmt.Fprintf(tw, "%s\t%s\t%.2f\t%.2f\t%.2f\t%.2f\n", element.ObjectID, element.Type, element.Geometry.X, element.Geometry.Y, element.Geometry.Width, element.Geometry.Height)
		}
		_ = tw.Flush()
	}

	return nil
}

func slidesReadSlideSummaries(elements []slidesElementDetail) ([]map[string]any, []map[string]any, []map[string]any) {
	textElements := []map[string]any{}
	images := []map[string]any{}
	tables := []map[string]any{}

	for _, element := range elements {
		if element.Shape != nil && element.Shape.Text != nil && element.Shape.Text.Content != "" {
			textElements = append(textElements, map[string]any{
				"objectId": element.ObjectID,
				"text":     element.Shape.Text.Content,
			})
		}
		if element.Image != nil {
			image := map[string]any{"objectId": element.ObjectID}
			if element.Image.ContentURL != "" {
				image["contentUrl"] = element.Image.ContentURL
			}
			if element.Image.SourceURL != "" {
				image["sourceUrl"] = element.Image.SourceURL
			}
			images = append(images, image)
		}
		if element.Table == nil {
			continue
		}
		cells := []map[string]any{}
		for _, cell := range element.Table.Cells {
			text := ""
			if cell.Text != nil {
				text = cell.Text.Content
			}
			cellSummary := map[string]any{
				"rowIndex":    cell.RowIndex,
				"columnIndex": cell.ColumnIndex,
				"rowSpan":     cell.RowSpan,
				"columnSpan":  cell.ColumnSpan,
				"text":        text,
			}
			cells = append(cells, cellSummary)
			if text != "" {
				textElements = append(textElements, map[string]any{
					"objectId":    element.ObjectID,
					"rowIndex":    cell.RowIndex,
					"columnIndex": cell.ColumnIndex,
					"text":        text,
				})
			}
		}
		tables = append(tables, map[string]any{
			"objectId": element.ObjectID,
			"rows":     element.Table.Rows,
			"columns":  element.Table.Columns,
			"cells":    cells,
		})
	}

	return textElements, images, tables
}

func valueOrNone(value any) string {
	if text, ok := value.(string); ok && text != "" {
		return text
	}
	return "(none)"
}
