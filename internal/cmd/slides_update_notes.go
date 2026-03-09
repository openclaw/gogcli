package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/ui"
)

type SlidesUpdateNotesCmd struct {
	PresentationID string  `arg:"" name:"presentationId" help:"Presentation ID"`
	SlideID        string  `arg:"" name:"slideId" help:"Slide object ID"`
	Notes          *string `name:"notes" help:"Speaker notes text (use --notes '' to clear notes)"`
	NotesFile      string  `name:"notes-file" help:"Path to file containing speaker notes" type:"existingfile"`
}

func (c *SlidesUpdateNotesCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	if !updateNotes {
		return usage("provide --notes or --notes-file")
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

	slidesSvc, err := newSlidesService(ctx, account)
	if err != nil {
		return err
	}

	pres, err := slidesSvc.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}

	// Find the target slide.
	var found bool
	var notesObjectID string
	for _, s := range pres.Slides {
		if s.ObjectId != slideID {
			continue
		}
		found = true
		if np := s.SlideProperties.NotesPage; np != nil {
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
		break
	}

	if !found {
		return fmt.Errorf("slide %q not found in presentation", slideID)
	}
	if notesObjectID == "" {
		return fmt.Errorf("could not find speaker notes placeholder on slide %s", slideID)
	}

	requests := []*slides.Request{
		{
			DeleteText: &slides.DeleteTextRequest{
				ObjectId: notesObjectID,
				TextRange: &slides.Range{
					Type: "ALL",
				},
			},
		},
	}
	if notes != "" {
		requests = append(requests, &slides.Request{
			InsertText: &slides.InsertTextRequest{
				ObjectId: notesObjectID,
				Text:     notes,
			},
		})
	}

	_, err = slidesSvc.Presentations.BatchUpdate(presentationID, &slides.BatchUpdatePresentationRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update speaker notes: %w", err)
	}

	u.Out().Printf("Updated notes on slide %s", slideID)
	return nil
}
