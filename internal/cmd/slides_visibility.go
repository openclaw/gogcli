package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/slides/v1"

	"github.com/openclaw/gogcli/internal/ui"
)

type SlidesSkipSlideCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string `arg:"" name:"slideId" help:"Slide object ID to skip (use 'slides list-slides' to find IDs)"`
}

func (c *SlidesSkipSlideCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runSlidesSetSkipped(ctx, flags, c.PresentationID, c.SlideID, true, "slides.skip-slide")
}

type SlidesUnskipSlideCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string `arg:"" name:"slideId" help:"Slide object ID to include (use 'slides list-slides' to find IDs)"`
}

func (c *SlidesUnskipSlideCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runSlidesSetSkipped(ctx, flags, c.PresentationID, c.SlideID, false, "slides.unskip-slide")
}

func runSlidesSetSkipped(
	ctx context.Context,
	flags *RootFlags,
	presentationIDValue, slideIDValue string,
	isSkipped bool,
	op string,
) error {
	u := ui.FromContext(ctx)

	presentationID := strings.TrimSpace(presentationIDValue)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	slideID := strings.TrimSpace(slideIDValue)
	if slideID == "" {
		return usage("empty slideId")
	}

	body := slidesVisibilityBatchUpdate(slideID, isSkipped)
	if err := dryRunExit(ctx, flags, op, map[string]any{
		"presentation_id": presentationID,
		"slide_object_id": slideID,
		"is_skipped":      isSkipped,
		"batch_update":    body,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	slidesSvc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}
	if _, err := slidesSvc.Presentations.BatchUpdate(presentationID, body).Context(ctx).Do(); err != nil {
		return fmt.Errorf("update slide skipped state: %w", err)
	}

	return writeResult(ctx, u,
		kv("slideObjectId", slideID),
		kv("presentationId", presentationID),
		kv("isSkipped", isSkipped),
	)
}

func slidesVisibilityBatchUpdate(slideID string, isSkipped bool) *slides.BatchUpdatePresentationRequest {
	properties := &slides.SlideProperties{IsSkipped: isSkipped}
	if !isSkipped {
		properties.ForceSendFields = []string{"IsSkipped"}
	}

	return &slides.BatchUpdatePresentationRequest{
		Requests: []*slides.Request{
			{
				UpdateSlideProperties: &slides.UpdateSlidePropertiesRequest{
					ObjectId:        slideID,
					SlideProperties: properties,
					Fields:          "isSkipped",
				},
			},
		},
	}
}
