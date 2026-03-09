package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/ui"
)

// fetchDoc creates a Docs service and fetches the document. Used by command implementations
// that need the full document structure (delete, append, insert).
func fetchDoc(ctx context.Context, account, id string) (*docs.Service, *docs.Document, error) {
	docsSvc, err := newDocsService(ctx, account)
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
	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}

	re, err := expr.compilePattern()
	if err != nil {
		return fmt.Errorf("compile pattern: %w", err)
	}

	// Find paragraphs matching the pattern and collect their ranges for deletion
	var requests []*docs.Request
	deleted := 0

	// Walk in reverse so deletions don't shift indices
	if doc.Body == nil {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: "0 (empty document)"})
	}
	elems := doc.Body.Content
	for i := len(elems) - 1; i >= 0; i-- {
		elem := elems[i]
		if elem.Paragraph == nil {
			continue
		}
		text := extractParagraphText(elem.Paragraph)
		if re.MatchString(text) {
			start := elem.StartIndex
			end := elem.EndIndex
			// Don't delete before the document body start
			if start < 1 {
				start = 1
			}
			requests = append(requests, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: start,
						EndIndex:   end,
						SegmentId:  "",
					},
				},
			})
			deleted++
		}
	}

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: "0 (no matches)"})
	}

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (delete): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: "deleted", Value: fmt.Sprintf("%d lines", deleted)})
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
	docsSvc, doc, err := fetchDoc(ctx, account, id)
	if err != nil {
		return err
	}
	resultKey := "appended"
	if before {
		resultKey = "inserted"
	}

	re, err := expr.compilePattern()
	if err != nil {
		return fmt.Errorf("compile pattern: %w", err)
	}

	// Process replacement text: convert \n to real newlines
	insertText := strings.ReplaceAll(expr.replacement, "\\n", "\n")
	if !strings.HasSuffix(insertText, "\n") {
		insertText += "\n"
	}

	// Collect insertion points (in reverse order to preserve indices)
	var insertPoints []int64
	if doc.Body == nil {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: resultKey, Value: "0 (empty document)"})
	}
	for _, elem := range doc.Body.Content {
		if elem.Paragraph == nil {
			continue
		}
		text := extractParagraphText(elem.Paragraph)
		if re.MatchString(text) {
			if before {
				insertPoints = append(insertPoints, elem.StartIndex)
			} else {
				insertPoints = append(insertPoints, elem.EndIndex)
			}
		}
	}

	if len(insertPoints) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: resultKey, Value: "0 (no matches)"})
	}

	// Build requests in reverse document order
	var requests []*docs.Request
	for i := len(insertPoints) - 1; i >= 0; i-- {
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: insertPoints[i]},
				Text:     insertText,
			},
		})
	}

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (insert): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: resultKey, Value: fmt.Sprintf("%d lines", len(insertPoints))})
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

// extractParagraphText returns the plain text content of a paragraph.
func extractParagraphText(p *docs.Paragraph) string {
	// Fast path: single text run (most common case) avoids Builder allocation.
	if len(p.Elements) == 1 && p.Elements[0].TextRun != nil {
		return strings.TrimRight(p.Elements[0].TextRun.Content, "\n")
	}
	var sb strings.Builder
	for _, elem := range p.Elements {
		if elem.TextRun != nil {
			sb.WriteString(elem.TextRun.Content)
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// --- Addressed command implementations ---

// resolveAddress converts a sedAddress into a slice of target paragraphs from the map.
func resolveAddress(addr *sedAddress, pm *paragraphMap) ([]docParagraph, error) {
	if addr == nil {
		return nil, fmt.Errorf("nil address")
	}
	if len(pm.Paragraphs) == 0 {
		return nil, fmt.Errorf("document has no paragraphs")
	}

	last := len(pm.Paragraphs)

	start := addr.Start
	if start == -1 {
		start = last
	}
	if start < 1 || start > last {
		return nil, fmt.Errorf("address %d out of range (document has %d paragraphs)", start, last)
	}

	if !addr.HasRange {
		return []docParagraph{pm.Paragraphs[start-1]}, nil
	}

	end := addr.End
	if end == 0 {
		end = start
	}
	if end == -1 {
		end = last
	}
	if end < 1 || end > last {
		return nil, fmt.Errorf("address end %d out of range (document has %d paragraphs)", end, last)
	}

	return pm.Paragraphs[start-1 : end], nil
}

// runAddressedDelete deletes paragraphs by address (number or range).
func (c *DocsSedCmd) runAddressedDelete(ctx context.Context, u *ui.UI, account, id, tabID string, expr sedExpr) error {
	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	pm, err := fetchAndBuildMap(ctx, docsSvc, id, tabID)
	if err != nil {
		return err
	}

	targets, err := resolveAddress(expr.addr, pm)
	if err != nil {
		return err
	}

	// Build delete requests in reverse order to preserve indices
	var requests []*docs.Request
	for i := len(targets) - 1; i >= 0; i-- {
		para := targets[i]
		startIndex := para.StartIndex
		endIndex := para.EndIndex

		isLast := para.Num == len(pm.Paragraphs)
		if isLast && para.Num > 1 {
			// Last paragraph: delete from end of previous paragraph to our end-1
			prev := pm.Paragraphs[para.Num-2]
			startIndex = prev.EndIndex - 1
			endIndex = para.EndIndex - 1
		} else if isLast && para.Num == 1 {
			// Only paragraph: just clear the text
			if para.StartIndex >= para.EndIndex-1 {
				continue // empty paragraph, skip
			}
			endIndex = para.EndIndex - 1
		}

		requests = append(requests, &docs.Request{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{
					StartIndex: startIndex,
					EndIndex:   endIndex,
					TabId:      pm.TabID,
				},
			},
		})
	}

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
	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	pm, err := fetchAndBuildMap(ctx, docsSvc, id, tabID)
	if err != nil {
		return err
	}

	targets, err := resolveAddress(expr.addr, pm)
	if err != nil {
		return err
	}

	insertText := strings.ReplaceAll(expr.replacement, "\\n", "\n")
	if !strings.HasSuffix(insertText, "\n") {
		insertText = "\n" + insertText
	} else {
		insertText = "\n" + insertText[:len(insertText)-1]
	}

	// Insert in reverse order to preserve indices
	var requests []*docs.Request
	for i := len(targets) - 1; i >= 0; i-- {
		para := targets[i]
		// Insert before the trailing \n of the paragraph
		idx := para.EndIndex - 1
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: idx, TabId: pm.TabID},
				Text:     insertText,
			},
		})
	}

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (addressed append): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: "appended", Value: fmt.Sprintf("%d paragraphs", len(targets))})
}

// runAddressedInsert inserts text before the addressed paragraph(s).
func (c *DocsSedCmd) runAddressedInsert(ctx context.Context, u *ui.UI, account, id, tabID string, expr sedExpr) error {
	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	pm, err := fetchAndBuildMap(ctx, docsSvc, id, tabID)
	if err != nil {
		return err
	}

	targets, err := resolveAddress(expr.addr, pm)
	if err != nil {
		return err
	}

	insertText := strings.ReplaceAll(expr.replacement, "\\n", "\n")
	if !strings.HasSuffix(insertText, "\n") {
		insertText += "\n"
	}

	// Insert in reverse order to preserve indices
	var requests []*docs.Request
	for i := len(targets) - 1; i >= 0; i-- {
		para := targets[i]
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: para.StartIndex, TabId: pm.TabID},
				Text:     insertText,
			},
		})
	}

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (addressed insert): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: "inserted", Value: fmt.Sprintf("%d paragraphs", len(targets))})
}

// runAddressedSubstitute applies a substitution only within the addressed paragraph(s).
func (c *DocsSedCmd) runAddressedSubstitute(ctx context.Context, u *ui.UI, account, id, tabID string, expr sedExpr) error {
	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	pm, err := fetchAndBuildMap(ctx, docsSvc, id, tabID)
	if err != nil {
		return err
	}

	targets, err := resolveAddress(expr.addr, pm)
	if err != nil {
		return err
	}

	re, compileErr := expr.compilePattern()
	if compileErr != nil {
		return fmt.Errorf("compile pattern: %w", compileErr)
	}

	// For each target paragraph, find matches and apply substitutions.
	// Work in reverse order to preserve indices.
	var requests []*docs.Request
	replaced := 0

	for i := len(targets) - 1; i >= 0; i-- {
		para := targets[i]
		text := para.Text

		matches := re.FindAllStringIndex(text, -1)
		if len(matches) == 0 {
			continue
		}

		if !expr.global {
			matches = matches[:1]
		}

		// Process matches in reverse order within this paragraph
		for j := len(matches) - 1; j >= 0; j-- {
			m := matches[j]
			matchText := text[m[0]:m[1]]
			replText := re.ReplaceAllString(matchText, expr.replacement)
			// Unescape Go regex $$ to literal $
			replText = literalReplacement(replText)

			absStart := para.StartIndex + int64(m[0])
			absEnd := para.StartIndex + int64(m[1])

			requests = append(requests, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: absStart,
						EndIndex:   absEnd,
						TabId:      pm.TabID,
					},
				},
			})
			requests = append(requests, &docs.Request{
				InsertText: &docs.InsertTextRequest{
					Location: &docs.Location{Index: absStart, TabId: pm.TabID},
					Text:     replText,
				},
			})
			replaced++
		}
	}

	if len(requests) == 0 {
		return sedOutputOK(ctx, u, id, sedOutputKV{Key: "replaced", Value: 0})
	}

	if _, err := batchUpdate(ctx, docsSvc, id, requests); err != nil {
		return fmt.Errorf("batch update (addressed substitute): %w", err)
	}

	return sedOutputOK(ctx, u, id, sedOutputKV{Key: "replaced", Value: replaced})
}
