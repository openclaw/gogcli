package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// SlidesReplaceTextCmd performs a find-and-replace across a presentation.
// It is a thin wrapper around presentations.batchUpdate with a single
// ReplaceAllTextRequest.
type SlidesReplaceTextCmd struct {
	PresentationID string   `arg:"" name:"presentationId" help:"Presentation ID"`
	Find           string   `arg:"" name:"find" help:"Substring to find"`
	Replacement    string   `arg:"" name:"replacement" help:"Replacement text"`
	MatchCase      bool     `name:"match-case" help:"Case-sensitive match (default: false)"`
	Pages          []string `name:"page" help:"Restrict replacement to specific slide object IDs (repeatable)"`
}

// Run executes the replace-text command.
func (c *SlidesReplaceTextCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	if c.Find == "" {
		return usage("empty find text")
	}

	// Build the batchUpdate request body.
	req := &slides.ReplaceAllTextRequest{
		ContainsText: &slides.SubstringMatchCriteria{
			Text:      c.Find,
			MatchCase: c.MatchCase,
		},
		ReplaceText: c.Replacement,
	}
	if len(c.Pages) > 0 {
		// Preserve order and trim whitespace on each page id.
		pages := make([]string, 0, len(c.Pages))
		for _, p := range c.Pages {
			if p = strings.TrimSpace(p); p != "" {
				pages = append(pages, p)
			}
		}
		req.PageObjectIds = pages
	}

	body := &slides.BatchUpdatePresentationRequest{
		Requests: []*slides.Request{{ReplaceAllText: req}},
	}

	// Dry-run: print the request body we would send and exit without calling the API.
	if flags != nil && flags.DryRun {
		return writeSlidesBatchUpdateDryRun(ctx, body)
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	slidesSvc, err := newSlidesService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := slidesSvc.Presentations.BatchUpdate(presentationID, body).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("replace text: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, resp)
	}

	var replaced int64
	if resp != nil {
		for _, r := range resp.Replies {
			if r != nil && r.ReplaceAllText != nil {
				replaced += r.ReplaceAllText.OccurrencesChanged
			}
		}
	}
	revisionID := ""
	if resp != nil && resp.WriteControl != nil {
		revisionID = resp.WriteControl.RequiredRevisionId
	}
	u.Out().Printf("ok | revisionId=%s | replaced=%d", revisionID, replaced)
	return nil
}
