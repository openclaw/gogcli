package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SlidesReplaceSlideCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string  `arg:"" name:"slideId" help:"Slide object ID to replace"`
	Image          string  `arg:"" name:"image" help:"Local image file (PNG/JPG/GIF)" type:"existingfile"`
	Notes          *string `name:"notes" help:"New speaker notes text (omit to preserve existing notes; use --notes '' to clear)"`
	NotesFile      string  `name:"notes-file" help:"Path to file containing new speaker notes" type:"existingfile"`
}

func (c *SlidesReplaceSlideCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	// Resolve notes: --notes-file takes precedence over --notes.
	var notes string
	updateNotes := false
	if c.NotesFile != "" {
		data, err := os.ReadFile(c.NotesFile)
		if err != nil {
			return fmt.Errorf("read notes file: %w", err)
		}
		notes = string(data)
		updateNotes = true
	} else if c.Notes != nil {
		notes = *c.Notes
		updateNotes = true
	}

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

	// Validate image format.
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

	// Get presentation to find the slide and its image element.
	pres, err := slidesSvc.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}

	slideIndex := -1
	var imageObjectID string
	for i, s := range pres.Slides {
		if s.ObjectId == slideID {
			slideIndex = i
			// Find the first image element on the slide.
			for _, el := range s.PageElements {
				if el.Image != nil {
					imageObjectID = el.ObjectId
					break
				}
			}
			break
		}
	}
	if slideIndex == -1 {
		return fmt.Errorf("slide %q not found in presentation", slideID)
	}
	if imageObjectID == "" {
		return fmt.Errorf("no image found on slide %s", slideID)
	}

	// Upload new image to Drive.
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

	// Clean up the temporary Drive file when done.
	defer func() {
		_ = driveSvc.Files.Delete(driveFile.Id).Context(ctx).Do()
	}()

	// Make publicly readable so the Slides API can fetch it.
	_, err = driveSvc.Permissions.Create(driveFile.Id, &drive.Permission{
		Type: "anyone",
		Role: "reader",
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("set image permissions: %w", err)
	}

	// Obtain a public download URL.
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

	// Replace the image in-place.
	requests := []*slides.Request{
		{
			ReplaceImage: &slides.ReplaceImageRequest{
				ImageObjectId:      imageObjectID,
				ImageReplaceMethod: "CENTER_CROP",
				Url:                imageURL,
			},
		},
	}

	// Optionally update notes in the same batch.
	if updateNotes {
		var notesObjectID string
		slide := pres.Slides[slideIndex]
		if np := slide.SlideProperties.NotesPage; np != nil {
			if np.NotesProperties != nil {
				notesObjectID = np.NotesProperties.SpeakerNotesObjectId
			}
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
		if notesObjectID == "" {
			return fmt.Errorf("could not find speaker notes placeholder on slide %s", slideID)
		}

		requests = append(requests, &slides.Request{
			DeleteText: &slides.DeleteTextRequest{
				ObjectId: notesObjectID,
				TextRange: &slides.Range{
					Type: "ALL",
				},
			},
		})
		if notes != "" {
			requests = append(requests, &slides.Request{
				InsertText: &slides.InsertTextRequest{
					ObjectId: notesObjectID,
					Text:     notes,
				},
			})
		}
	}

	_, err = slidesSvc.Presentations.BatchUpdate(presentationID, &slides.BatchUpdatePresentationRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("replace slide image: %w", err)
	}

	link := fmt.Sprintf("https://docs.google.com/presentation/d/%s/edit", presentationID)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"slideNumber":    slideIndex + 1,
			"slideObjectId":  slideID,
			"presentationId": presentationID,
			"link":           link,
		})
	}

	u.Out().Printf("Replaced image on slide %d (%s)", slideIndex+1, slideID)
	if updateNotes {
		u.Out().Printf("Updated speaker notes")
	}
	u.Out().Printf("link\t%s", link)
	return nil
}
