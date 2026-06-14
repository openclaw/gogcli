package cmd

import (
	"context"
	"fmt"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docssed"
	"github.com/steipete/gogcli/internal/ui"
)

// fetchDoc creates a Docs service and fetches the document. Used by command implementations
// that need the full document structure (delete, append, insert).
func fetchDoc(ctx context.Context, account, id string) (*docs.Service, *docs.Document, error) {
	docsSvc, err := docsService(ctx, account)
	if err != nil {
		return nil, nil, fmt.Errorf("create docs service: %w", err)
	}
	doc, err := getDoc(ctx, docsSvc, id)
	if err != nil {
		return nil, nil, fmt.Errorf("get document: %w", err)
	}
	return docsSvc, doc, nil
}

// runDeleteCommand executes a d/pattern/ command, deleting all lines containing the pattern.
func (c *DocsSedCmd) runDeleteCommand(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	planner, err := docssed.NewParagraphPlanner(semanticExpressionFromSedExpr(expr))
	if err != nil {
		return err
	}

	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	if doc.Body == nil {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: "0 (empty document)"})
	}

	projection := docssed.ProjectDocument(doc)
	if projection.Legacy == nil {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: "0 (no matches)"})
	}
	mutations := planner.PlanDelete(*projection.Legacy)
	requests := buildAddressMutationRequests(mutations, "")

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: "0 (no matches)"})
	}

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (delete): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: fmt.Sprintf("%d lines", len(mutations))})
}

// runAppendCommand executes an a/pattern/text/ command, inserting text after each matching line.
func (c *DocsSedCmd) runAppendCommand(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	return c.runInsertAroundMatch(ctx, u, account, id, expr, false)
}

// runInsertCommand executes an i/pattern/text/ command, inserting text before each matching line.
func (c *DocsSedCmd) runInsertCommand(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	return c.runInsertAroundMatch(ctx, u, account, id, expr, true)
}

// runInsertAroundMatch implements both append-after and insert-before matching lines.
func (c *DocsSedCmd) runInsertAroundMatch(ctx context.Context, u *ui.UI, account, id string, expr sedExpr, before bool) error {
	planner, err := docssed.NewParagraphPlanner(semanticExpressionFromSedExpr(expr))
	if err != nil {
		return err
	}

	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}
	resultKey := "appended"
	if before {
		resultKey = "inserted"
	}

	if doc.Body == nil {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: resultKey, Value: "0 (empty document)"})
	}

	projection := docssed.ProjectDocument(doc)
	if projection.Legacy == nil {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: resultKey, Value: "0 (no matches)"})
	}
	mutations := planner.PlanInsert(*projection.Legacy, expr.replacement, before)
	requests := buildAddressMutationRequests(mutations, "")

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: resultKey, Value: "0 (no matches)"})
	}

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (insert): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: resultKey, Value: fmt.Sprintf("%d lines", len(mutations))})
}

// runTransliterate executes a y/source/dest/ command, replacing each character in source
// with the corresponding character in dest throughout the document.
func (c *DocsSedCmd) runTransliterate(ctx context.Context, u *ui.UI, account, id string, expr sedExpr) error {
	docsSvc, _, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	sourceRunes := []rune(expr.pattern)
	destRunes := []rune(expr.replacement)

	// Use native FindReplace for each character pair
	var requests []*docs.Request
	for i, src := range sourceRunes {
		requests = append(requests, &docs.Request{
			ReplaceAllText: &docs.ReplaceAllTextRequest{
				ContainsText: &docs.SubstringMatchCriteria{
					Text:      string(src),
					MatchCase: true,
				},
				ReplaceText: string(destRunes[i]),
			},
		})
	}

	resp, err := batchUpdate(ctx, docsSvc, id, requests)
	if err != nil {
		return fmt.Errorf("batch update (transliterate): %w", err)
	}
	var replaced int
	if resp != nil {
		for _, reply := range resp.Replies {
			if reply.ReplaceAllText != nil {
				replaced += int(reply.ReplaceAllText.OccurrencesChanged)
			}
		}
	}

	return sedOutputOK(ctx, u, id,
		sedOutputKV{Key: "transliterated", Value: fmt.Sprintf("%d chars across %d pairs", replaced, len(sourceRunes))},
	)
}

// --- Addressed command implementations ---

func addressElements(paragraphs []docParagraph) []docssed.AddressElement {
	elements := make([]docssed.AddressElement, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		kind := docssed.AddressElementKind(paragraph.ElemType)
		elements = append(elements, docssed.AddressElement{
			Number:     paragraph.Num,
			Kind:       kind,
			Text:       paragraph.Text,
			StartIndex: paragraph.StartIndex,
			EndIndex:   paragraph.EndIndex,
		})
	}
	return elements
}

func buildAddressMutationRequests(mutations []docssed.AddressMutation, tabID string) []*docs.Request {
	requests := make([]*docs.Request, 0, len(mutations)*2)
	for index := len(mutations) - 1; index >= 0; index-- {
		mutation := mutations[index]
		if mutation.EndIndex > mutation.StartIndex {
			requests = append(requests, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: mutation.StartIndex,
						EndIndex:   mutation.EndIndex,
						TabId:      tabID,
					},
				},
			})
		}
		if mutation.InsertText != "" {
			requests = append(requests, &docs.Request{
				InsertText: &docs.InsertTextRequest{
					Location: &docs.Location{Index: mutation.StartIndex, TabId: tabID},
					Text:     mutation.InsertText,
				},
			})
		}
	}
	return requests
}

// runAddressedDelete deletes paragraphs by address (number or range).
func (c *DocsSedCmd) runAddressedDelete(ctx context.Context, u *ui.UI, account, id, tabID string, expr sedExpr) error {
	docsSvc, err := docsService(ctx, account)
	if err != nil {
		return err
	}

	pm, err := fetchAndBuildMap(ctx, docsSvc, id, tabID)
	if err != nil {
		return err
	}

	elements := addressElements(pm.Paragraphs)
	targets, err := docssed.ResolveAddress(expr.addr, elements)
	if err != nil {
		return err
	}
	mutations := docssed.PlanAddressedDelete(elements, targets)
	requests := buildAddressMutationRequests(mutations, pm.TabID)

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: "0"})
	}

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (addressed delete): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: fmt.Sprintf("%d paragraphs", len(targets))})
}

// runAddressedAppend inserts text after the addressed paragraph(s).
func (c *DocsSedCmd) runAddressedAppend(ctx context.Context, u *ui.UI, account, id, tabID string, expr sedExpr) error {
	docsSvc, err := docsService(ctx, account)
	if err != nil {
		return err
	}

	pm, err := fetchAndBuildMap(ctx, docsSvc, id, tabID)
	if err != nil {
		return err
	}

	elements := addressElements(pm.Paragraphs)
	targets, err := docssed.ResolveAddress(expr.addr, elements)
	if err != nil {
		return err
	}
	mutations := docssed.PlanAddressedAppend(targets, expr.replacement)
	requests := buildAddressMutationRequests(mutations, pm.TabID)

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (addressed append): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: "appended", Value: fmt.Sprintf("%d paragraphs", len(targets))})
}

// runAddressedInsert inserts text before the addressed paragraph(s).
func (c *DocsSedCmd) runAddressedInsert(ctx context.Context, u *ui.UI, account, id, tabID string, expr sedExpr) error {
	docsSvc, err := docsService(ctx, account)
	if err != nil {
		return err
	}

	pm, err := fetchAndBuildMap(ctx, docsSvc, id, tabID)
	if err != nil {
		return err
	}

	elements := addressElements(pm.Paragraphs)
	targets, err := docssed.ResolveAddress(expr.addr, elements)
	if err != nil {
		return err
	}
	mutations := docssed.PlanAddressedInsert(targets, expr.replacement)
	requests := buildAddressMutationRequests(mutations, pm.TabID)

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (addressed insert): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: "inserted", Value: fmt.Sprintf("%d paragraphs", len(targets))})
}

// runAddressedSubstitute applies a substitution only within the addressed paragraph(s).
func (c *DocsSedCmd) runAddressedSubstitute(ctx context.Context, u *ui.UI, account, id, tabID string, expr sedExpr) error {
	planner, err := docssed.NewMatchPlanner(semanticExpressionFromSedExpr(expr))
	if err != nil {
		return err
	}

	docsSvc, err := docsService(ctx, account)
	if err != nil {
		return err
	}

	pm, err := fetchAndBuildMap(ctx, docsSvc, id, tabID)
	if err != nil {
		return err
	}

	elements := addressElements(pm.Paragraphs)
	targets, err := docssed.ResolveAddress(expr.addr, elements)
	if err != nil {
		return err
	}

	paragraphs := make([]docssed.DocumentParagraph, 0, len(targets))
	for _, target := range targets {
		if target.Kind != docssed.AddressParagraph {
			continue
		}
		paragraphs = append(paragraphs, docssed.DocumentParagraph{
			Text:       target.Text,
			StartIndex: target.StartIndex,
			EndIndex:   target.EndIndex,
		})
	}
	actions := planner.PlanParagraphs(paragraphs)

	// Apply in reverse document order so earlier Docs indices remain stable.
	requests := make([]*docs.Request, 0, len(actions)*2)
	for index := len(actions) - 1; index >= 0; index-- {
		action := actions[index]
		requests = append(requests, &docs.Request{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{
					StartIndex: action.StartIndex,
					EndIndex:   action.EndIndex,
					TabId:      pm.TabID,
				},
			},
		})
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: action.StartIndex, TabId: pm.TabID},
				Text:     action.Replacement.ExpandedText,
			},
		})
	}

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: "replaced", Value: 0})
	}

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (addressed substitute): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: "replaced", Value: len(actions)})
}
