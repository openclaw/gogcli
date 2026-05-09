package cmd

import (
	"context"
	"fmt"
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

	notes, updateNotes, err := resolveSlidesNotesInput(c.Notes, c.NotesFile)
	if err != nil {
		return err
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

	slide, _ := findSlidesPageByID(pres, slideID)
	if slide == nil {
		return fmt.Errorf("slide %q not found in presentation", slideID)
	}
	notesObjectID := findSpeakerNotesObjectID(slide)
	if notesObjectID == "" {
		return fmt.Errorf("could not find speaker notes placeholder on slide %s", slideID)
	}

	requests := buildSlidesClearAndInsertTextRequests(notesObjectID, notes)
	_, err = slidesSvc.Presentations.BatchUpdate(presentationID, &slides.BatchUpdatePresentationRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update speaker notes: %w", err)
	}

	u.Out().Printf("Updated notes on slide %s", slideID)
	return nil
}
