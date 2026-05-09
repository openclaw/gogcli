package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"google.golang.org/api/slides/v1"
)

const (
	imageExtJPG   = ".jpg"
	imageExtJPEG  = ".jpeg"
	imageExtGIF   = ".gif"
	imageMimeJPEG = "image/jpeg"
	imageMimeGIF  = "image/gif"

	placeholderTypeBody = "BODY"
)

func resolveSlidesNotesInput(notes *string, notesFile string) (string, bool, error) {
	if notesFile != "" {
		// The caller's Kong field uses type:"existingfile"; this helper only
		// centralizes the read for the two Slides note commands.
		//nolint:gosec
		data, err := os.ReadFile(notesFile)
		if err != nil {
			return "", false, fmt.Errorf("read notes file: %w", err)
		}
		return string(data), true, nil
	}
	if notes != nil {
		return *notes, true, nil
	}
	return "", false, nil
}

func findSlidesPageByID(pres *slides.Presentation, slideID string) (*slides.Page, int) {
	if pres == nil {
		return nil, -1
	}
	for i, slide := range pres.Slides {
		if slide != nil && slide.ObjectId == slideID {
			return slide, i
		}
	}
	return nil, -1
}

func findSpeakerNotesObjectID(slide *slides.Page) string {
	if slide == nil || slide.SlideProperties == nil || slide.SlideProperties.NotesPage == nil {
		return ""
	}

	notesPage := slide.SlideProperties.NotesPage
	if notesPage.NotesProperties != nil && notesPage.NotesProperties.SpeakerNotesObjectId != "" {
		return notesPage.NotesProperties.SpeakerNotesObjectId
	}

	for _, el := range notesPage.PageElements {
		if el != nil && el.Shape != nil && el.Shape.Placeholder != nil &&
			el.Shape.Placeholder.Type == placeholderTypeBody {
			return el.ObjectId
		}
	}
	return ""
}

func buildSlidesClearAndInsertTextRequests(objectID string, text string) []*slides.Request {
	requests := []*slides.Request{
		{
			DeleteText: &slides.DeleteTextRequest{
				ObjectId: objectID,
				TextRange: &slides.Range{
					Type: "ALL",
				},
			},
		},
	}
	if text != "" {
		requests = append(requests, &slides.Request{
			InsertText: &slides.InsertTextRequest{
				ObjectId: objectID,
				Text:     text,
			},
		})
	}
	return requests
}

// writeSlidesBatchUpdateDryRun serializes a BatchUpdatePresentationRequest to
// stdout as pretty JSON. Used by slides commands that honor --dry-run.
func writeSlidesBatchUpdateDryRun(ctx context.Context, body *slides.BatchUpdatePresentationRequest) error {
	if body == nil {
		return errors.New("nil batch update request")
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(body); err != nil {
		return fmt.Errorf("encode dry-run request: %w", err)
	}
	_ = ctx // reserved for future use (e.g., UI-aware formatting)
	return nil
}
