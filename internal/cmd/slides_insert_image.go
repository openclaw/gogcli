package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/slides/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

// SlidesInsertImageCmd inserts an image at an explicit position and size on an
// existing slide. Unlike add-slide (which lays a full-bleed image on a new
// slide), this places a sized element on a slide you already have, so callers
// can build native decks via the Slides API and still drop in a logo, chart,
// or badge at a precise location. Local files use the same temporary Drive
// upload flow as add-slide. URL images remain remote; missing dimensions require
// an anonymous header read before the URL is passed to Slides.
type SlidesInsertImageCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string  `arg:"" name:"slideId" help:"Slide object ID to place the image on"`
	Image          string  `arg:"" optional:"" name:"image" help:"Local image file (PNG/JPG/GIF)" type:"existingfile"`
	URL            string  `name:"url" help:"HTTPS image URL that Slides can fetch anonymously"`
	X              float64 `name:"x" default:"0" help:"Left position of the image, in --unit"`
	Y              float64 `name:"y" default:"0" help:"Top position of the image, in --unit"`
	Width          float64 `name:"width" default:"0" help:"Image width, in --unit; derived from the source aspect ratio when only height is given"`
	Height         float64 `name:"height" default:"0" help:"Image height, in --unit; derived from the source aspect ratio when only width is given"`
	Unit           string  `name:"unit" enum:"PT,EMU" default:"PT" help:"Measurement unit for x/y/width/height (PT or EMU)"`
}

func (c *SlidesInsertImageCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	slideID := strings.TrimSpace(c.SlideID)
	if slideID == "" {
		return usage("empty slideId")
	}
	if err := validateSlidesImageDimensions(c.Width, c.Height); err != nil {
		return err
	}

	source, err := resolveSlidesImageSource(c.Image, c.URL)
	if err != nil {
		return err
	}

	width, height, err := resolveSlidesImageDimensions(ctx, source, c.Width, c.Height)
	if err != nil {
		return err
	}

	dryRunPayload := map[string]any{
		"presentation_id": presentationID,
		"slide_id":        slideID,
		"x":               c.X,
		"y":               c.Y,
		"width":           width,
		"height":          height,
		"unit":            c.Unit,
	}
	if source.imageURL != "" {
		dryRunPayload["url"] = source.imageURL
	} else {
		dryRunPayload["image"] = source.localPath
		dryRunPayload["mime_type"] = source.mimeType
	}
	if dryRunErr := dryRunExit(ctx, flags, "slides.insert-image", dryRunPayload); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	slidesSvc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}

	// Confirm the target slide exists before creating the Drive service or
	// uploading anything, so a bad slide id never touches Drive.
	pres, err := slidesSvc.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}
	if _, idx := findSlidesPageByID(pres, slideID); idx == -1 {
		return fmt.Errorf("slide %q not found in presentation", slideID)
	}

	imageURL, cleanup, err := prepareSlidesImageURL(ctx, account, source)
	if err != nil {
		return err
	}
	defer cleanup()

	imageID := fmt.Sprintf("img_%d", time.Now().UnixNano())

	err = batchUpdateSlidesImageRequests(ctx, slidesSvc, presentationID, &slides.BatchUpdatePresentationRequest{
		Requests: []*slides.Request{
			{
				CreateImage: &slides.CreateImageRequest{
					ObjectId: imageID,
					Url:      imageURL,
					ElementProperties: &slides.PageElementProperties{
						PageObjectId: slideID,
						Size: &slides.Size{
							Width:  &slides.Dimension{Magnitude: width, Unit: c.Unit},
							Height: &slides.Dimension{Magnitude: height, Unit: c.Unit},
						},
						Transform: &slides.AffineTransform{
							ScaleX:     1,
							ScaleY:     1,
							TranslateX: c.X,
							TranslateY: c.Y,
							Unit:       c.Unit,
						},
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("insert image: %w", err)
	}

	link := fmt.Sprintf("https://docs.google.com/presentation/d/%s/edit", presentationID)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"presentationId": presentationID,
			"slideObjectId":  slideID,
			"imageObjectId":  imageID,
			"link":           link,
		})
	}

	u.Out().Linef("image\t%s", imageID)
	u.Out().Linef("link\t%s", link)
	return nil
}
