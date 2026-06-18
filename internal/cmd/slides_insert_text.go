package cmd

import (
	"context"
	"fmt"
	"io"
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
	Row            *int64 `name:"row" help:"0-based table row index for cell-targeted text; requires --col"`
	Col            *int64 `name:"col" help:"0-based table column index for cell-targeted text; requires --row"`
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
		data, err := io.ReadAll(commandIO(ctx).In)
		if err != nil {
			return fmt.Errorf("read text from stdin: %w", err)
		}
		text = string(data)
	}
	if text == "" && !c.Replace {
		return usage("empty text")
	}
	if c.InsertionIndex < 0 {
		return usage("insertion-index must be >= 0")
	}
	var cellLocation *slides.TableCellLocation
	if c.Row != nil || c.Col != nil {
		if c.Row == nil || c.Col == nil {
			return usage("--row and --col must be provided together")
		}
		if *c.Row < 0 {
			return usage("--row must be >= 0")
		}
		if *c.Col < 0 {
			return usage("--col must be >= 0")
		}
		cellLocation = slidesTableCellLocation(*c.Row, *c.Col)
	}

	// Build the batchUpdate request body.
	var requests []*slides.Request
	if c.Replace {
		requests = buildSlidesClearAndInsertTextRequestsAt(objectID, text, cellLocation)
	} else {
		requests = append(requests, &slides.Request{
			InsertText: &slides.InsertTextRequest{
				CellLocation:   cellLocation,
				ObjectId:       objectID,
				Text:           text,
				InsertionIndex: c.InsertionIndex,
			},
		})
	}

	body := &slides.BatchUpdatePresentationRequest{Requests: requests}

	if err := dryRunExit(ctx, flags, "slides.insert-text", map[string]any{
		"presentation_id": presentationID,
		"object_id":       objectID,
		"text_length":     len(text),
		"insertion_index": c.InsertionIndex,
		"replace":         c.Replace,
		"row":             c.Row,
		"col":             c.Col,
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

	if c.Replace && cellLocation != nil {
		pres, getErr := slidesSvc.Presentations.Get(presentationID).Context(ctx).Do()
		if getErr != nil {
			return fmt.Errorf("get presentation: %w", getErr)
		}
		found, hasExistingText := slidesTableCellTextState(pres, objectID, *c.Row, *c.Col)
		if !found {
			return fmt.Errorf("table cell %s[%d,%d] not found", objectID, *c.Row, *c.Col)
		}
		body.Requests = buildSlidesReplaceTextRequestsAt(objectID, text, hasExistingText, cellLocation)
		if pres.RevisionId != "" {
			body.WriteControl = &slides.WriteControl{RequiredRevisionId: pres.RevisionId}
		}
		if len(body.Requests) == 0 {
			return writeSlidesInsertTextResult(ctx, u, presentationID, pres.RevisionId, 0)
		}
	}

	resp, err := slidesSvc.Presentations.BatchUpdate(presentationID, body).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("insert text: %w", err)
	}

	revisionID := ""
	if resp != nil && resp.WriteControl != nil {
		revisionID = resp.WriteControl.RequiredRevisionId
	}
	replies := 0
	if resp != nil {
		replies = len(resp.Replies)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), resp)
	}
	return writeSlidesInsertTextResult(ctx, u, presentationID, revisionID, replies)
}

func writeSlidesInsertTextResult(ctx context.Context, u *ui.UI, presentationID, revisionID string, replies int) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"presentationId": presentationID,
			"replies":        []any{},
			"writeControl":   map[string]any{"requiredRevisionId": revisionID},
		})
	}
	u.Out().Linef("ok | revisionId=%s | replies=%d", revisionID, replies)
	return nil
}
