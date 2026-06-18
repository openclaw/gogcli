package cmd

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder for aspect detection
	_ "image/jpeg" // register JPEG decoder for aspect detection
	_ "image/png"  // register PNG decoder for aspect detection
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// SlidesInsertImageCmd inserts an image at an explicit position and size on an
// existing slide. Unlike add-slide (which lays a full-bleed image on a new
// slide), this places a sized element on a slide you already have, so callers
// can build native decks via the Slides API and still drop in a logo, chart,
// or badge at a precise location. Local files use the same temporary Drive
// upload flow as add-slide; public HTTPS URLs are passed directly to Slides.
type SlidesInsertImageCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string  `arg:"" name:"slideId" help:"Slide object ID to place the image on"`
	Image          string  `arg:"" optional:"" name:"image" help:"Local image file (PNG/JPG/GIF)" type:"existingfile"`
	URL            string  `name:"url" help:"Public HTTPS image URL to insert directly"`
	X              float64 `name:"x" default:"0" help:"Left position of the image, in --unit"`
	Y              float64 `name:"y" default:"0" help:"Top position of the image, in --unit"`
	Width          float64 `name:"width" required:"" help:"Image width, in --unit"`
	Height         float64 `name:"height" default:"0" help:"Image height, in --unit; required with --url, local files preserve aspect ratio when omitted"`
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
	if c.Width <= 0 {
		return usage("--width must be greater than 0")
	}
	if c.Height < 0 {
		return usage("--height cannot be negative")
	}

	source, err := resolveSlidesImageSource(c.Image, c.URL)
	if err != nil {
		return err
	}

	// Resolve height from the image's aspect ratio when not supplied.
	height := c.Height
	if height == 0 {
		if source.imageURL != "" {
			return usage("--height is required with --url")
		}
		ar, aspectErr := imageAspectRatio(source.localPath)
		if aspectErr != nil {
			return fmt.Errorf("determine image aspect ratio (pass --height to skip): %w", aspectErr)
		}
		height = c.Width * ar
	}

	dryRunPayload := map[string]any{
		"presentation_id": presentationID,
		"slide_id":        slideID,
		"x":               c.X,
		"y":               c.Y,
		"width":           c.Width,
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

	imageURL := source.imageURL
	if imageURL == "" {
		driveSvc, driveErr := driveService(ctx, account)
		if driveErr != nil {
			return driveErr
		}
		imgFile, openErr := os.Open(source.localPath)
		if openErr != nil {
			return fmt.Errorf("open image: %w", openErr)
		}
		defer imgFile.Close()

		driveFile, uploadErr := driveSvc.Files.Create(&drive.File{
			Name:     filepath.Base(source.localPath),
			MimeType: source.mimeType,
		}).Media(imgFile).Fields("id, webContentLink").Context(ctx).Do()
		if uploadErr != nil {
			return fmt.Errorf("upload image to Drive: %w", uploadErr)
		}
		defer func() {
			if delErr := driveSvc.Files.Delete(driveFile.Id).Context(context.WithoutCancel(ctx)).Do(); delErr != nil {
				u.Err().Linef("Warning: failed to delete temporary Drive image %s; it may remain publicly readable until removed: %v", driveFile.Id, delErr)
			}
		}()

		_, permissionErr := driveSvc.Permissions.Create(driveFile.Id, &drive.Permission{
			Type: "anyone",
			Role: "reader",
		}).Context(ctx).Do()
		if permissionErr != nil {
			return fmt.Errorf("set image permissions: %w", permissionErr)
		}
		imageURL = driveImageDownloadURL(driveFile.Id)
	}

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
							Width:  &slides.Dimension{Magnitude: c.Width, Unit: c.Unit},
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

// imageAspectRatio returns height/width for the given image file.
func imageAspectRatio(path string) (float64, error) {
	f, err := os.Open(path) //nolint:gosec // user-provided local image path is the command input.
	if err != nil {
		return 0, fmt.Errorf("open image: %w", err)
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, fmt.Errorf("decode image config: %w", err)
	}
	if cfg.Width <= 0 {
		return 0, fmt.Errorf("image has zero width")
	}
	return float64(cfg.Height) / float64(cfg.Width), nil
}
