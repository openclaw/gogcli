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

type SlidesListSlidesCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
}

func (c *SlidesListSlidesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}

	slidesSvc, err := newSlidesService(ctx, account)
	if err != nil {
		return err
	}

	pres, err := slidesSvc.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		items := make([]map[string]any, len(pres.Slides))
		for i, s := range pres.Slides {
			items[i] = map[string]any{
				"number":   i + 1,
				"objectId": s.ObjectId,
			}
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"presentationId": presentationID,
			"title":          pres.Title,
			"slideCount":     len(pres.Slides),
			"slides":         items,
		})
	}

	u.Out().Printf("Presentation: %s (%d slides)", pres.Title, len(pres.Slides))
	u.Out().Println("")

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tOBJECT ID")
	for i, s := range pres.Slides {
		fmt.Fprintf(tw, "%d\t%s\n", i+1, s.ObjectId)
	}
	_ = tw.Flush()
	return nil
}
