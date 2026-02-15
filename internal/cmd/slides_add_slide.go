package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesAddSlideCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	Image          string `arg:"" name:"image" help:"Local image file (PNG/JPG)" type:"existingfile"`
	Notes          string `name:"notes" help:"Speaker notes text"`
	NotesFile      string `name:"notes-file" help:"Path to file containing speaker notes" type:"existingfile"`
	Before         string `name:"before" help:"Insert before this slide ID (appends to end if omitted)" optional:""`
}

func (c *SlidesAddSlideCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	// Resolve notes: --notes-file takes precedence over --notes
	var notes string
	if c.NotesFile != "" {
		data, err := os.ReadFile(c.NotesFile)
		if err != nil {
			return fmt.Errorf("read notes file: %w", err)
		}
		notes = string(data)
	} else {
		notes = c.Notes
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}

	// Validate image format
	ext := strings.ToLower(filepath.Ext(c.Image))
	var mimeType string
	switch ext {
	case extPNG:
		mimeType = mimePNG
	case imageExtJPG, imageExtJPEG:
		mimeType = imageMimeJPEG
	case imageExtGIF:
		mimeType = imageMimeGIF
	default:
		return fmt.Errorf("unsupported image format %q (use PNG, JPG, or GIF)", ext)
	}

	slidesSvc, err := newSlidesService(ctx, account)
	if err != nil {
		return err
	}
	driveSvc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	// Upload image to Drive as a temporary file
	imgFile, err := os.Open(c.Image)
	if err != nil {
		return fmt.Errorf("open image: %w", err)
	}
	defer imgFile.Close()

	driveFile, err := driveSvc.Files.Create(&drive.File{
		Name:     filepath.Base(c.Image),
		MimeType: mimeType,
	}).Media(imgFile).Fields("id, webContentLink").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("upload image to Drive: %w", err)
	}

	// Clean up the temporary Drive file when done
	defer func() {
		_ = driveSvc.Files.Delete(driveFile.Id).Context(ctx).Do()
	}()

	// Make publicly readable so the Slides API can fetch it
	_, err = driveSvc.Permissions.Create(driveFile.Id, &drive.Permission{
		Type: "anyone",
		Role: "reader",
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("set image permissions: %w", err)
	}

	// Obtain a public download URL
	imageURL := driveFile.WebContentLink
	if imageURL == "" {
		got, getErr := driveSvc.Files.Get(driveFile.Id).Fields("webContentLink").Context(ctx).Do()
		if getErr != nil {
			return fmt.Errorf("get image URL: %w", getErr)
		}
		imageURL = got.WebContentLink
	}
	if imageURL == "" {
		return fmt.Errorf("could not obtain public URL for uploaded image")
	}

	// Get presentation to read page size and current slide count
	pres, err := slidesSvc.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}

	pageWidth := pres.PageSize.Width
	pageHeight := pres.PageSize.Height
	initialSlideCount := len(pres.Slides)

	// Resolve insertion index from --before flag
	var insertionIndex int64
	useBefore := c.Before != ""
	if useBefore {
		found := false
		for i, s := range pres.Slides {
			if s.ObjectId == c.Before {
				insertionIndex = int64(i)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("slide %q not found in presentation", c.Before)
		}
	}

	// Generate a unique slide object ID
	slideID := fmt.Sprintf("s_%d", time.Now().UnixNano())

	createSlideReq := &slides.CreateSlideRequest{
		ObjectId: slideID,
		SlideLayoutReference: &slides.LayoutReference{
			PredefinedLayout: "BLANK",
		},
	}
	if useBefore {
		createSlideReq.InsertionIndex = insertionIndex
		createSlideReq.ForceSendFields = []string{"InsertionIndex"}
	}

	// Create the slide with a full-bleed image in one batch
	_, err = slidesSvc.Presentations.BatchUpdate(presentationID, &slides.BatchUpdatePresentationRequest{
		Requests: []*slides.Request{
			{
				CreateSlide: createSlideReq,
			},
			{
				CreateImage: &slides.CreateImageRequest{
					Url: imageURL,
					ElementProperties: &slides.PageElementProperties{
						PageObjectId: slideID,
						Size: &slides.Size{
							Width:  pageWidth,
							Height: pageHeight,
						},
						Transform: &slides.AffineTransform{
							ScaleX: 1,
							ScaleY: 1,
							Unit:   "EMU",
						},
					},
				},
			},
		},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create slide: %w", err)
	}

	// Add speaker notes if provided
	if notes != "" {
		// Re-fetch the presentation to get the notes page details
		pres, err = slidesSvc.Presentations.Get(presentationID).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("read back presentation: %w", err)
		}

		var notesObjectID string
		for _, slide := range pres.Slides {
			if slide.ObjectId != slideID {
				continue
			}
			if np := slide.SlideProperties.NotesPage; np != nil {
				// Prefer the direct property
				if np.NotesProperties != nil {
					notesObjectID = np.NotesProperties.SpeakerNotesObjectId
				}
				// Fallback: search page elements for BODY placeholder
				if notesObjectID == "" {
					for _, el := range np.PageElements {
						if el.Shape != nil && el.Shape.Placeholder != nil &&
							el.Shape.Placeholder.Type == placeholderTypeBody {
							notesObjectID = el.ObjectId
							break
						}
					}
				}
			}
			break
		}

		if notesObjectID == "" {
			return fmt.Errorf("could not find speaker notes placeholder on new slide")
		}

		_, err = slidesSvc.Presentations.BatchUpdate(presentationID, &slides.BatchUpdatePresentationRequest{
			Requests: []*slides.Request{
				{
					InsertText: &slides.InsertTextRequest{
						ObjectId: notesObjectID,
						Text:     notes,
					},
				},
			},
		}).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("insert speaker notes: %w", err)
		}
	}

	slideNum := initialSlideCount + 1
	if useBefore {
		slideNum = int(insertionIndex) + 1
	}
	link := fmt.Sprintf("https://docs.google.com/presentation/d/%s/edit", presentationID)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"slideNumber":    slideNum,
			"slideObjectId":  slideID,
			"presentationId": presentationID,
			"link":           link,
		})
	}

	u.Out().Printf("slide\t%d", slideNum)
	u.Out().Printf("link\t%s", link)
	return nil
}
