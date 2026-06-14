package cmd

import (
	"context"
	"fmt"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docssed"
	"github.com/steipete/gogcli/internal/ui"
)

func (c *DocsSedCmd) runManual(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	docsSvc, err := docsService(ctx, account)
	if err != nil {
		return fmt.Errorf("create docs service: %w", err)
	}

	count, bulletReqs, err := c.runManualInner(ctx, docsSvc, id, expr)
	if err != nil {
		return fmt.Errorf("manual replace: %w", err)
	}

	// Apply deferred bullet requests via re-fetch to get current positions
	if len(bulletReqs) > 0 {
		if err := docssed.NewServiceExecutor(docsSvc).ApplyDeferredBullets(ctx, id); err != nil {
			return fmt.Errorf("apply bullets: %w", err)
		}
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{"replaced", count})
}

func findDocActions(doc *docs.Document, planner *docssed.MatchPlanner) []docssed.MatchAction {
	projection := docssed.ProjectDocument(doc)
	if projection.Legacy == nil {
		return nil
	}
	return planner.PlanSegment(*projection.Legacy)
}

// processFootnotes handles footnote matches, each needing a two-phase create+populate approach.
func processFootnotes(ctx context.Context, docsSvc *docs.Service, id string, footnotes []docssed.FootnoteMutation) error {
	for i := len(footnotes) - 1; i >= 0; i-- {
		footnote := footnotes[i]
		fnReqs := []*docs.Request{
			{DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{StartIndex: footnote.StartIndex, EndIndex: footnote.EndIndex},
			}},
			{CreateFootnote: &docs.CreateFootnoteRequest{
				Location: &docs.Location{Index: footnote.StartIndex},
			}},
		}
		resp, err := batchUpdate(ctx, docsSvc, id, fnReqs)
		if err != nil {
			return fmt.Errorf("create footnote: %w", err)
		}
		// Find the footnote ID from the response and insert text into it
		if resp != nil {
			for _, reply := range resp.Replies {
				if reply.CreateFootnote != nil && reply.CreateFootnote.FootnoteId != "" {
					fnID := reply.CreateFootnote.FootnoteId
					fnTextReqs := []*docs.Request{
						{InsertText: &docs.InsertTextRequest{
							Location: &docs.Location{
								Index:     1, // footnote body starts at index 1
								SegmentId: fnID,
							},
							Text: footnote.Text,
						}},
					}
					if _, err := batchUpdate(ctx, docsSvc, id, fnTextReqs); err != nil {
						return fmt.Errorf("populate footnote: %w", err)
					}
					break
				}
			}
		}
	}
	return nil
}

// runManualInner is like runManual but reuses an existing docsSvc and returns count
// plus deferred bullet requests that need a post-mutation document fetch.
func (c *DocsSedCmd) runManualInner(ctx context.Context, docsSvc *docs.Service, id string, expr sedExpr) (int, []*docs.Request, error) {
	planner, err := docssed.NewMatchPlanner(semanticExpressionFromSedExpr(expr))
	if err != nil {
		return 0, nil, err
	}

	doc, err := getDoc(ctx, docsSvc, id)
	if err != nil {
		return 0, nil, fmt.Errorf("get document: %w", err)
	}

	plan := docssed.PlanTextMutations(findDocActions(doc, planner))
	if plan.MatchCount == 0 {
		return 0, nil, nil
	}

	var requests []*docs.Request

	// Process image matches individually — Google Docs API cannot handle
	// DeleteContentRange + InsertInlineImage in the same batch request
	// (it fails to fetch the image URL when combined with other operations).
	for i := len(plan.Images) - 1; i >= 0; i-- {
		image := plan.Images[i]
		// First: delete the matched text
		deleteReqs := []*docs.Request{{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{StartIndex: image.StartIndex, EndIndex: image.EndIndex},
			},
		}}
		if _, err2 := batchUpdate(ctx, docsSvc, id, deleteReqs); err2 != nil {
			return 0, nil, fmt.Errorf("delete before image insert: %w", err2)
		}
		// Then: insert image in a separate API call
		imgReq := &docs.InsertInlineImageRequest{
			Uri:        image.Image.URL,
			Location:   &docs.Location{Index: image.StartIndex},
			ObjectSize: buildImageSizeSpec(image.Image),
		}
		if _, err2 := batchUpdate(ctx, docsSvc, id, []*docs.Request{{InsertInlineImage: imgReq}}); err2 != nil {
			return 0, nil, fmt.Errorf(
				"image insert (url=%s idx=%d): %w",
				image.Image.URL,
				image.StartIndex,
				err2,
			)
		}
	}

	for i := len(plan.TextEdits) - 1; i >= 0; i-- {
		edit := plan.TextEdits[i]
		requests = append(requests, &docs.Request{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{StartIndex: edit.StartIndex, EndIndex: edit.EndIndex},
			},
		})

		switch {
		case edit.HorizontalRule:
			// Horizontal rule: insert a newline, then style it with a bottom border
			requests = append(requests, &docs.Request{
				InsertText: &docs.InsertTextRequest{
					Location: &docs.Location{Index: edit.StartIndex},
					Text:     "\n",
				},
			})
			requests = append(requests, buildHruleBorderRequest(edit.StartIndex, edit.StartIndex+1))
		default:
			if edit.InsertText != "" {
				requests = append(requests, &docs.Request{
					InsertText: &docs.InsertTextRequest{
						Location: &docs.Location{Index: edit.StartIndex},
						Text:     edit.InsertText,
					},
				})
			}
		}
	}

	// Add text-level formatting (bold, italic, code, super/sub, etc.)
	for _, formatting := range plan.Formatting {
		if formatting.Brace != nil {
			// SEDMAT v3.5 brace syntax path
			requests = append(
				requests,
				buildBraceTextStyleRequests(formatting.Brace, formatting.StartIndex, formatting.EndIndex)...,
			)
			// Handle inline scoping spans
			requests = append(requests, buildBraceInlineRequests(formatting.BraceSpans, formatting.StartIndex)...)
		} else {
			requests = append(
				requests,
				buildTextStyleRequests(formatting.Formats, formatting.StartIndex, formatting.EndIndex)...,
			)
		}
	}

	// Split paragraph-level requests into bullet requests (deferred) and
	// non-bullet requests (headings, blockquotes — applied immediately).
	// Bullets are deferred so the caller can merge consecutive same-preset
	// bullets into a single CreateParagraphBullets call, which is required
	// for Google Docs to interpret leading \t as nesting levels.
	var paraRequests []*docs.Request
	var deferredBullets []*docs.Request
	for _, formatting := range plan.Formatting {
		paraEnd := formatting.EndIndex + 1
		// Use brace paragraph formatting if available
		if formatting.Brace != nil && hasBraceParagraphFormat(formatting.Brace) {
			paraRequests = append(
				paraRequests,
				buildBraceParagraphStyleRequests(formatting.Brace, formatting.StartIndex, paraEnd)...,
			)
		} else {
			for _, req := range buildParagraphStyleRequests(
				formatting.Formats,
				formatting.StartIndex,
				paraEnd,
			) {
				if req.CreateParagraphBullets != nil && formatting.LeadingTab {
					// Nested bullets (have \t) are deferred so the caller can merge
					// them with adjacent L0 bullets for proper nesting.
					deferredBullets = append(deferredBullets, req)
				} else {
					paraRequests = append(paraRequests, req)
				}
			}
		}
	}

	// Phase 1: inserts, deletes, text formatting
	if _, err2 := batchUpdate(ctx, docsSvc, id, requests); err2 != nil {
		return 0, nil, fmt.Errorf("update document: %w", err2)
	}

	// Phase 2: non-bullet paragraph styles (headings, blockquotes)
	if _, err2 := batchUpdate(ctx, docsSvc, id, paraRequests); err2 != nil {
		return 0, nil, fmt.Errorf("apply paragraph styles: %w", err2)
	}

	// Handle footnotes — each needs create + populate, processed individually in reverse
	if err = processFootnotes(ctx, docsSvc, id, plan.Footnotes); err != nil {
		return 0, nil, err
	}

	// Phase 3: insert page/section/column break if {+=X} or {break=X} is set.
	if err = applyBreakPhase(ctx, docsSvc, id, expr, plan.Formatting); err != nil {
		return 0, nil, err
	}

	// Phase 4: Apply structural features (columns, checkboxes, bookmarks, smart chips).
	// Requires re-fetching the document since text indices shifted in Phase 1.
	if expr.brace != nil && hasBraceStructuralFeatures(expr.brace) {
		freshDoc, err := getDoc(ctx, docsSvc, id)
		if err != nil {
			return 0, nil, fmt.Errorf("get doc for structural: %w", err)
		}

		// Collect all structural requests
		var allStructuralReqs []*docs.Request

		for _, formatting := range plan.Formatting {
			if formatting.Brace == nil {
				continue
			}

			// Get section boundaries for columns
			sectionStart, sectionEnd := buildSectionRangeForMatch(
				freshDoc,
				formatting.StructuralStartIndex,
				formatting.StructuralEndIndex,
			)

			// Build structural requests
			colReqs, bulletReqs, anchorReqs, chipReqs := buildStructuralRequests(
				formatting.Brace,
				formatting.StructuralStartIndex,
				formatting.StructuralEndIndex,
				sectionStart,
				sectionEnd,
			)

			allStructuralReqs = append(allStructuralReqs, colReqs...)
			allStructuralReqs = append(allStructuralReqs, anchorReqs...)
			allStructuralReqs = append(allStructuralReqs, chipReqs...)

			// Add checkbox bullets to deferred bullets
			deferredBullets = append(deferredBullets, bulletReqs...)
		}

		if _, err := batchUpdate(ctx, docsSvc, id, allStructuralReqs); err != nil {
			return 0, nil, fmt.Errorf("apply structural features: %w", err)
		}
	}

	return plan.MatchCount, deferredBullets, nil
}

// applyBreakPhase inserts page/section/column breaks after all text modifications.
func applyBreakPhase(
	ctx context.Context,
	docsSvc *docs.Service,
	id string,
	expr sedExpr,
	formatting []docssed.FormatIntent,
) error {
	if expr.brace == nil || !expr.brace.HasBreak || len(formatting) == 0 {
		return nil
	}

	freshDoc, err := getDoc(ctx, docsSvc, id)
	if err != nil {
		return fmt.Errorf("get doc for break: %w", err)
	}

	lastEnd := formatting[len(formatting)-1].StructuralEndIndex
	breakIdx := lastEnd + 1
	if freshDoc.Body != nil && len(freshDoc.Body.Content) > 0 {
		bodyEnd := freshDoc.Body.Content[len(freshDoc.Body.Content)-1].EndIndex
		if breakIdx >= bodyEnd {
			breakIdx = bodyEnd - 1
		}
	}

	breakReqs := buildBraceBreakRequests(expr.brace, breakIdx)
	if _, err := batchUpdate(ctx, docsSvc, id, breakReqs); err != nil {
		return fmt.Errorf("insert break: %w", err)
	}
	return nil
}
