package cmd

import (
	"context"
	"fmt"
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
	ObjectID       string   `name:"object" help:"Restrict replacement to a single shape text object ID"`
	All            bool     `name:"all" help:"Replace matching text across the entire presentation"`
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
	objectID := strings.TrimSpace(c.ObjectID)
	if objectID != "" {
		if len(c.Pages) > 0 || c.All {
			return usage("--object, --page, and --all are mutually exclusive")
		}
		return c.runObjectScoped(ctx, flags, u, presentationID, objectID)
	}
	if c.All && len(c.Pages) > 0 {
		return usage("--page and --all are mutually exclusive")
	}
	if !c.All && len(c.Pages) == 0 {
		return usage("explicit scope required: use --object, --page, or --all")
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
			p = strings.TrimSpace(p)
			if p == "" {
				return usage("empty page object ID")
			}
			pages = append(pages, p)
		}
		req.PageObjectIds = pages
	}

	body := &slides.BatchUpdatePresentationRequest{
		Requests: []*slides.Request{{ReplaceAllText: req}},
	}

	if err := dryRunExit(ctx, flags, "slides.replace-text", map[string]any{
		"presentation_id": presentationID,
		"find":            c.Find,
		"replacement":     c.Replacement,
		"match_case":      c.MatchCase,
		"pages":           req.PageObjectIds,
		"all":             c.All,
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

	resp, err := slidesSvc.Presentations.BatchUpdate(presentationID, body).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("replace text: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), resp)
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
	u.Out().Linef("ok | revisionId=%s | replaced=%d", revisionID, replaced)
	return nil
}

func (c *SlidesReplaceTextCmd) runObjectScoped(ctx context.Context, flags *RootFlags, u *ui.UI, presentationID, objectID string) error {
	if flags != nil && flags.DryRun {
		return dryRunExit(ctx, flags, "slides.replace-text", map[string]any{
			"presentation_id":            presentationID,
			"find":                       c.Find,
			"replacement":                c.Replacement,
			"match_case":                 c.MatchCase,
			"object_id":                  objectID,
			"object_scope":               true,
			"requires_presentation_read": true,
			"batch_update":               &slides.BatchUpdatePresentationRequest{Requests: []*slides.Request{}},
		})
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	slidesSvc, err := slidesService(ctx, account)
	if err != nil {
		return err
	}
	pres, err := slidesSvc.Presentations.Get(presentationID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get presentation: %w", err)
	}
	requests, replaced, err := buildSlidesObjectReplaceTextRequests(pres, objectID, c.Find, c.Replacement, c.MatchCase)
	if err != nil {
		return err
	}
	body := &slides.BatchUpdatePresentationRequest{Requests: requests}
	if strings.TrimSpace(pres.RevisionId) != "" {
		body.WriteControl = &slides.WriteControl{RequiredRevisionId: pres.RevisionId}
	}

	dryRunErr := dryRunExit(ctx, flags, "slides.replace-text", map[string]any{
		"presentation_id": presentationID,
		"find":            c.Find,
		"replacement":     c.Replacement,
		"match_case":      c.MatchCase,
		"object_id":       objectID,
		"replaced":        replaced,
		"batch_update":    body,
	})
	if dryRunErr != nil {
		return dryRunErr
	}

	resp, err := slidesSvc.Presentations.BatchUpdate(presentationID, body).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("replace text in object: %w", err)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), resp)
	}

	revisionID := ""
	if resp != nil && resp.WriteControl != nil {
		revisionID = resp.WriteControl.RequiredRevisionId
	}
	u.Out().Linef("ok | revisionId=%s | replaced=%d", revisionID, replaced)
	return nil
}

func buildSlidesObjectReplaceTextRequests(pres *slides.Presentation, objectID, find, replacement string, matchCase bool) ([]*slides.Request, int, error) {
	text, found, hasShapeText := slidesShapeTextContentByObjectID(pres, objectID)
	if !found {
		return nil, 0, usagef("object not found: %s", objectID)
	}
	if !hasShapeText {
		return nil, 0, usage("object-scoped replace-text currently supports shape text objects only")
	}

	matches := locateSlidesText(text, find, matchCase)
	if len(matches) == 0 {
		return nil, 0, usage("no matching text found in object")
	}

	requests := make([]*slides.Request, 0, len(matches)*2)
	for i := len(matches) - 1; i >= 0; i-- {
		match := matches[i]
		textRange, err := slidesFixedTextRange(match.StartIndex, match.EndIndex)
		if err != nil {
			return nil, 0, err
		}
		requests = append(requests, &slides.Request{
			DeleteText: &slides.DeleteTextRequest{
				ObjectId:  objectID,
				TextRange: textRange,
			},
		})
		if replacement != "" {
			requests = append(requests, &slides.Request{
				InsertText: &slides.InsertTextRequest{
					ObjectId:       objectID,
					InsertionIndex: match.StartIndex,
					Text:           replacement,
				},
			})
		}
	}
	return requests, len(matches), nil
}

func slidesShapeTextContentByObjectID(pres *slides.Presentation, objectID string) (*slides.TextContent, bool, bool) {
	if pres == nil {
		return nil, false, false
	}
	for _, page := range pres.Slides {
		if page == nil {
			continue
		}
		for _, el := range page.PageElements {
			text, found, hasShapeText := slidesShapeTextByPageElement(el, objectID)
			if found {
				return text, true, hasShapeText
			}
		}
	}
	return nil, false, false
}

func slidesShapeTextByPageElement(el *slides.PageElement, objectID string) (*slides.TextContent, bool, bool) {
	if el == nil {
		return nil, false, false
	}
	if el.ObjectId == objectID {
		if el.Shape == nil || el.Shape.Text == nil {
			return nil, true, false
		}
		return el.Shape.Text, true, true
	}
	if el.ElementGroup != nil {
		for _, child := range el.ElementGroup.Children {
			text, found, hasShapeText := slidesShapeTextByPageElement(child, objectID)
			if found {
				return text, true, hasShapeText
			}
		}
	}
	return nil, false, false
}
