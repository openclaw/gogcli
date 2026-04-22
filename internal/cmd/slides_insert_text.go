package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// SlidesInsertTextCmd inserts text into an existing text-capable page element.
// It is a thin wrapper around presentations.batchUpdate with an InsertTextRequest
// (optionally preceded by a DeleteText request when --replace is set).
type SlidesInsertTextCmd struct {
	PresentationID string `arg:"" name:"presentationId" help:"Presentation ID"`
	ObjectID       string `arg:"" name:"objectId" help:"Page element object ID (shape or table) to insert text into"`
	Text           string `arg:"" name:"text" help:"Text to insert (use '-' to read from stdin)"`
	InsertionIndex int64  `name:"insertion-index" help:"Zero-based index where text is inserted within the element's existing text" default:"0"`
	Replace        bool   `name:"replace" help:"Clear existing text in the element before inserting (emits DeleteText + InsertText in the same batch)"`
}

// Run executes the insert-text command.
func (c *SlidesInsertTextCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	presentationID := strings.TrimSpace(c.PresentationID)
	if presentationID == "" {
		return usage("empty presentationId")
	}
	objectID := strings.TrimSpace(c.ObjectID)
	if objectID == "" {
		return usage("empty objectId")
	}

	// Resolve text: '-' means read from stdin.
	text := c.Text
	if text == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read text from stdin: %w", err)
		}
		text = string(data)
	}

	// Build the batchUpdate request body.
	var requests []*slides.Request
	if c.Replace {
		requests = append(requests, &slides.Request{
			DeleteText: &slides.DeleteTextRequest{
				ObjectId: objectID,
				TextRange: &slides.Range{
					Type: "ALL",
				},
			},
		})
	}
	requests = append(requests, &slides.Request{
		InsertText: &slides.InsertTextRequest{
			ObjectId:       objectID,
			Text:           text,
			InsertionIndex: c.InsertionIndex,
		},
	})

	body := &slides.BatchUpdatePresentationRequest{Requests: requests}

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
		return fmt.Errorf("insert text: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, resp)
	}

	revisionID := ""
	if resp != nil && resp.WriteControl != nil {
		revisionID = resp.WriteControl.RequiredRevisionId
	}
	replies := 0
	if resp != nil {
		replies = len(resp.Replies)
	}
	u.Out().Printf("ok | revisionId=%s | replies=%d", revisionID, replies)
	return nil
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
