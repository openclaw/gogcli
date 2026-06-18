package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesNewSlideCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	Layout         *string `name:"layout" enum:"BLANK,CAPTION_ONLY,TITLE,TITLE_AND_BODY,TITLE_AND_TWO_COLUMNS,TITLE_ONLY,SECTION_HEADER,SECTION_TITLE_AND_DESCRIPTION,ONE_COLUMN_TEXT,MAIN_POINT,BIG_NUMBER" help:"Predefined slide layout; defaults to BLANK"`
	LayoutID       string  `name:"layout-id" help:"Exact presentation layout object ID from 'slides info --json'; mutually exclusive with --layout"`
	Index          *int64  `name:"index" help:"Zero-based insertion index for the new slide"`
}

func (c *SlidesNewSlideCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	layout := ""
	if c.Layout != nil {
		layout = strings.TrimSpace(*c.Layout)
	}
	layoutID := strings.TrimSpace(c.LayoutID)
	if layout != "" && layoutID != "" {
		return usage("--layout and --layout-id are mutually exclusive")
	}
	if c.Index != nil && *c.Index < 0 {
		return usage("--index must be >= 0")
	}

	slideID := newSlidesStructuralObjectID("gogSlide")
	createSlide := &slides.CreateSlideRequest{
		ObjectId:             slideID,
		SlideLayoutReference: slidesStructuralLayoutReference(layout, layoutID),
	}
	if c.Index != nil {
		createSlide.InsertionIndex = *c.Index
		createSlide.ForceSendFields = []string{"InsertionIndex"}
	}

	body := &slides.BatchUpdatePresentationRequest{
		Requests: []*slides.Request{
			{CreateSlide: createSlide},
		},
	}
	payload := map[string]any{
		"presentation_id": presentationID,
		"slide_object_id": slideID,
		"batch_update":    body,
	}
	if layout != "" {
		payload["layout"] = layout
	}
	if layoutID != "" {
		payload["layout_id"] = layoutID
	}
	if c.Index != nil {
		payload["index"] = *c.Index
	}
	if err := dryRunExit(ctx, flags, "slides.new-slide", payload); err != nil {
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
		return fmt.Errorf("create slide: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		out := map[string]any{
			"presentationId": presentationID,
			"slideObjectId":  slideID,
		}
		if layoutID != "" {
			out["layoutId"] = layoutID
		} else {
			out["layout"] = slidesStructuralLayoutName(layout)
		}
		if c.Index != nil {
			out["index"] = *c.Index
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), out)
	}

	u.Out().Linef("slideObjectId\t%s", slideID)
	u.Out().Linef("presentationId\t%s", presentationID)
	if layoutID != "" {
		u.Out().Linef("layoutId\t%s", layoutID)
	} else {
		u.Out().Linef("layout\t%s", slidesStructuralLayoutName(layout))
	}
	if c.Index != nil {
		u.Out().Linef("index\t%d", *c.Index)
	}
	return nil
}

type SlidesDuplicateSlideCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string `arg:"" name:"slideId" help:"Slide object ID to duplicate (use 'slides list-slides' to find IDs)"`
	ToIndex        *int64 `name:"to-index" help:"Zero-based insertion index for the duplicated slide"`
}

func (c *SlidesDuplicateSlideCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	slideID := strings.TrimSpace(c.SlideID)
	if slideID == "" {
		return usage("empty slideId")
	}
	if c.ToIndex != nil && *c.ToIndex < 0 {
		return usage("--to-index must be >= 0")
	}

	duplicateID := newSlidesStructuralObjectID("gogDup")
	requests := []*slides.Request{
		{
			DuplicateObject: &slides.DuplicateObjectRequest{
				ObjectId:  slideID,
				ObjectIds: map[string]string{slideID: duplicateID},
			},
		},
	}
	if c.ToIndex != nil {
		requests = append(requests, &slides.Request{
			UpdateSlidesPosition: &slides.UpdateSlidesPositionRequest{
				SlideObjectIds:  []string{duplicateID},
				InsertionIndex:  *c.ToIndex,
				ForceSendFields: []string{"InsertionIndex"},
			},
		})
	}
	body := &slides.BatchUpdatePresentationRequest{Requests: requests}

	payload := map[string]any{
		"presentation_id":        presentationID,
		"source_slide_object_id": slideID,
		"slide_object_id":        duplicateID,
		"batch_update":           body,
	}
	if c.ToIndex != nil {
		payload["to_index"] = *c.ToIndex
	}
	if err := dryRunExit(ctx, flags, "slides.duplicate-slide", payload); err != nil {
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
		return fmt.Errorf("duplicate slide: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		out := map[string]any{
			"presentationId":          presentationID,
			"sourceSlideObjectId":     slideID,
			"slideObjectId":           duplicateID,
			"duplicatedSlideObjectId": duplicateID,
		}
		if c.ToIndex != nil {
			out["toIndex"] = *c.ToIndex
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), out)
	}

	u.Out().Linef("slideObjectId\t%s", duplicateID)
	u.Out().Linef("sourceSlideObjectId\t%s", slideID)
	u.Out().Linef("presentationId\t%s", presentationID)
	if c.ToIndex != nil {
		u.Out().Linef("toIndex\t%d", *c.ToIndex)
	}
	return nil
}

type SlidesMoveSlideCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string `arg:"" name:"slideId" help:"Slide object ID to move (use 'slides list-slides' to find IDs)"`
	ToIndex        *int64 `name:"to-index" required:"" help:"Zero-based insertion index where the slide should be moved"`
}

func (c *SlidesMoveSlideCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	slideID := strings.TrimSpace(c.SlideID)
	if slideID == "" {
		return usage("empty slideId")
	}
	if c.ToIndex == nil {
		return usage("--to-index is required")
	}
	if *c.ToIndex < 0 {
		return usage("--to-index must be >= 0")
	}

	body := &slides.BatchUpdatePresentationRequest{
		Requests: []*slides.Request{
			{
				UpdateSlidesPosition: &slides.UpdateSlidesPositionRequest{
					SlideObjectIds:  []string{slideID},
					InsertionIndex:  *c.ToIndex,
					ForceSendFields: []string{"InsertionIndex"},
				},
			},
		},
	}
	if err := dryRunExit(ctx, flags, "slides.move-slide", map[string]any{
		"presentation_id": presentationID,
		"slide_object_id": slideID,
		"to_index":        *c.ToIndex,
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
		return fmt.Errorf("move slide: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"presentationId": presentationID,
			"slideObjectId":  slideID,
			"toIndex":        *c.ToIndex,
		})
	}

	u.Out().Linef("slideObjectId\t%s", slideID)
	u.Out().Linef("presentationId\t%s", presentationID)
	u.Out().Linef("toIndex\t%d", *c.ToIndex)
	return nil
}

func newSlidesStructuralObjectID(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}

func slidesStructuralLayoutReference(layout, layoutID string) *slides.LayoutReference {
	if layoutID != "" {
		return &slides.LayoutReference{LayoutId: layoutID}
	}
	if layout != "" {
		return &slides.LayoutReference{PredefinedLayout: layout}
	}
	return nil
}

func slidesStructuralLayoutName(layout string) string {
	if layout == "" {
		return "BLANK"
	}
	return layout
}
