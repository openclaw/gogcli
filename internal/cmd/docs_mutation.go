package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docsmarkdown"
)

// docsBatchUpdateRequestCap is the Docs API hard limit on the number of
// requests a single documents.batchUpdate may carry. When the consolidated
// body + table + formatting request list exceeds it we chunk into multiple
// sequential batchUpdate calls, preserving the request order so cell-index
// arithmetic stays consistent. See #699.
const docsBatchUpdateRequestCap = 500

const (
	docsContentFormatPlain    = "plain"
	docsContentFormatMarkdown = "markdown"
)

type docsLoadedTarget struct {
	full   *docs.Document
	target *docs.Document
	tabID  string
}

func loadDocsTargetDocument(ctx context.Context, svc *docs.Service, docID, tabID string) (*docsLoadedTarget, error) {
	getCall := svc.Documents.Get(docID).Context(ctx)
	if tabID != "" {
		getCall = getCall.IncludeTabsContent(true)
	}

	doc, err := getCall.Do()
	if err != nil {
		if isDocsNotFound(err) {
			return nil, fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return nil, err
	}
	if doc == nil {
		return nil, errors.New("doc not found")
	}
	if tabID == "" {
		return &docsLoadedTarget{full: doc, target: doc}, nil
	}

	tab, tabErr := findTab(flattenTabs(doc.Tabs), tabID)
	if tabErr != nil {
		return nil, tabErr
	}
	resolvedTabID := ""
	if tab.TabProperties != nil {
		resolvedTabID = strings.TrimSpace(tab.TabProperties.TabId)
	}
	if resolvedTabID == "" {
		return nil, fmt.Errorf("tab has no ID: %s", tabID)
	}
	if tab.DocumentTab == nil || tab.DocumentTab.Body == nil {
		return nil, fmt.Errorf("tab has no document body: %s", tabID)
	}

	return &docsLoadedTarget{
		full: doc,
		target: &docs.Document{
			DocumentId: doc.DocumentId,
			RevisionId: doc.RevisionId,
			Body:       tab.DocumentTab.Body,
		},
		tabID: resolvedTabID,
	}, nil
}

func runDocsReplaceAll(ctx context.Context, svc *docs.Service, docID, find, replaceText string, matchCase bool, tabID string) (string, int64, error) {
	req := &docs.ReplaceAllTextRequest{
		ContainsText: &docs.SubstringMatchCriteria{Text: find, MatchCase: matchCase},
		ReplaceText:  replaceText,
	}
	if tabID != "" {
		req.TabsCriteria = &docs.TabsCriteria{TabIds: []string{tabID}}
	}

	result, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{ReplaceAllText: req}},
	}).Context(ctx).Do()
	if err != nil {
		return "", 0, fmt.Errorf("find-replace: %w", err)
	}

	var replacements int64
	if len(result.Replies) > 0 && result.Replies[0].ReplaceAllText != nil {
		replacements = result.Replies[0].ReplaceAllText.OccurrencesChanged
	}
	return result.DocumentId, replacements, nil
}

func replaceDocsTextRange(ctx context.Context, svc *docs.Service, doc *docs.Document, startIdx, endIdx int64, replaceText, tabID string) error {
	requests := []*docs.Request{
		{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{StartIndex: startIdx, EndIndex: endIdx, TabId: tabID},
			},
		},
	}
	if replaceText != "" {
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: startIdx, TabId: tabID},
				Text:     replaceText,
			},
		})
	}

	_, err := svc.Documents.BatchUpdate(doc.DocumentId, &docs.BatchUpdateDocumentRequest{
		WriteControl: &docs.WriteControl{RequiredRevisionId: doc.RevisionId},
		Requests:     requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("replace: %w", err)
	}
	return nil
}

func replaceDocsMarkdownRange(ctx context.Context, svc *docs.Service, doc *docs.Document, startIdx, endIdx int64, replaceText string, tabID string) (requestCount int, inserted int, err error) {
	return replacePreparedDocsMarkdownRange(ctx, svc, doc, startIdx, endIdx, prepareMarkdown(replaceText), tabID)
}

func replacePreparedDocsMarkdownRange(
	ctx context.Context,
	svc *docs.Service,
	doc *docs.Document,
	startIdx int64,
	endIdx int64,
	markdown preparedMarkdown,
	tabID string,
) (requestCount int, inserted int, err error) {
	explicitHeadingAnchors := docsmarkdown.ExplicitHeadingAnchors(markdown.cleaned)
	elements := docsmarkdown.ParseMarkdown(markdown.cleaned)
	docsmarkdown.StripElementHeadingAnchors(elements)
	prefix := ""
	baseIndex := startIdx
	if markdownReplaceNeedsParagraphBoundary(doc, startIdx, tabID, elements) {
		prefix = "\n"
		baseIndex++
	}
	formattingRequests, textToInsert, tables := docsmarkdown.MarkdownToDocsRequests(elements, baseIndex, tabID)
	inlineReplacement := markdownRangeReplacementIsInline(markdown.cleaned, elements)
	if inlineReplacement {
		textToInsert = strings.TrimSuffix(textToInsert, "\n")
	}

	applyTabIDToFormattingRequests(formattingRequests, tabID)

	// Structural DeleteContentRange + body InsertText + per-element formatting
	// go in one batchUpdate. Tables are inserted afterwards via InsertNativeTable
	// (which does its own InsertTable + Get + batched cell-content per table —
	// see #699 follow-up: cross-table prediction was unreliable, server-readback
	// per table is correct).
	requests := make([]*docs.Request, 0, 2+len(formattingRequests))
	requests = append(requests, &docs.Request{
		DeleteContentRange: &docs.DeleteContentRangeRequest{
			Range: &docs.Range{StartIndex: startIdx, EndIndex: endIdx, TabId: tabID},
		},
	})
	if textToInsert != "" {
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: startIdx, TabId: tabID},
				Text:     prefix + textToInsert,
			},
		})
		requests = append(requests, resetDocsTextStyleRequest(baseIndex, baseIndex+utf16Len(textToInsert), tabID))
		if !inlineReplacement {
			resetEnd := baseIndex + utf16Len(textToInsert)
			if docRangeCoversParagraphText(doc, startIdx, endIdx, tabID) {
				resetEnd++
			}
			resetStart := markdownParagraphResetStart(elements, textToInsert, baseIndex)
			if resetStart < resetEnd {
				requests = append(requests, resetDocsParagraphRequests(resetStart, resetEnd, tabID)...)
			}
		}
		requests = append(requests, formattingRequests...)
	}

	requestCount, err = submitBatchedDocsRequests(ctx, svc, doc.DocumentId, requests, &docs.WriteControl{RequiredRevisionId: doc.RevisionId})
	if err != nil {
		return 0, 0, fmt.Errorf("replace (markdown): %w", err)
	}

	rewriteMaxIndex := baseIndex + utf16Len(textToInsert)
	if len(tables) > 0 {
		tableInserter := NewTableInserter(svc, doc.DocumentId)
		tableOffset := int64(0)
		for _, table := range tables {
			tableIndex := table.StartIndex + tableOffset
			tableEnd, tableErr := tableInserter.InsertNativeTable(ctx, tableIndex, table.Cells, tabID)
			if tableErr != nil {
				return requestCount, len(textToInsert), fmt.Errorf("insert native table: %w", tableErr)
			}
			tableOffset = nextTableInsertOffset(tableOffset, tableIndex, tableEnd)
		}
		rewriteMaxIndex += tableOffset
	}

	if len(markdown.images) > 0 {
		imgErr := insertImagesIntoDocs(ctx, svc, doc.DocumentId, markdown.images, tabID)
		cleanupDocsImagePlaceholders(ctx, svc, doc.DocumentId, markdown.images, tabID)
		if imgErr != nil {
			return requestCount, len(prefix) + len(textToInsert), fmt.Errorf("insert images: %w", imgErr)
		}
		rewriteMaxIndex = subtractMarkdownImagePlaceholderDrift(rewriteMaxIndex, baseIndex, markdown.images)
	}

	if markdownMayContainHeadingLinks(markdown.cleaned) {
		rewritten, rewriteErr := rewriteMarkdownHeadingLinksInRange(ctx, svc, doc.DocumentId, tabID, explicitHeadingAnchors, baseIndex, rewriteMaxIndex)
		if rewriteErr != nil {
			return requestCount, len(prefix) + len(textToInsert), fmt.Errorf("rewrite heading links: %w", rewriteErr)
		}
		requestCount += rewritten
	}

	return requestCount, len(prefix) + len(textToInsert), nil
}

func markdownRangeReplacementIsInline(markdown string, elements []docsmarkdown.MarkdownElement) bool {
	return !strings.HasSuffix(markdown, "\n") &&
		len(elements) == 1 &&
		elements[0].Type == docsmarkdown.MDParagraph
}

func markdownParagraphResetStart(elements []docsmarkdown.MarkdownElement, text string, baseIndex int64) int64 {
	if len(elements) == 0 || elements[0].Type != docsmarkdown.MDParagraph {
		return baseIndex
	}
	if newline := strings.IndexByte(text, '\n'); newline >= 0 {
		return baseIndex + utf16Len(text[:newline+1])
	}
	return baseIndex + utf16Len(text)
}

func resetDocsParagraphRequests(startIdx, endIdx int64, tabID string) []*docs.Request {
	rng := &docs.Range{StartIndex: startIdx, EndIndex: endIdx, TabId: tabID}
	return []*docs.Request{
		{
			DeleteParagraphBullets: &docs.DeleteParagraphBulletsRequest{
				Range: rng,
			},
		},
		{
			UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
				Range: &docs.Range{StartIndex: startIdx, EndIndex: endIdx, TabId: tabID},
				ParagraphStyle: &docs.ParagraphStyle{
					NamedStyleType:  docsNamedStyleNormalText,
					IndentStart:     &docs.Dimension{Magnitude: 0, Unit: "PT"},
					IndentFirstLine: &docs.Dimension{Magnitude: 0, Unit: "PT"},
				},
				Fields: "namedStyleType,indentStart,indentFirstLine",
			},
		},
	}
}

func markdownReplaceNeedsParagraphBoundary(doc *docs.Document, startIdx int64, tabID string, elements []docsmarkdown.MarkdownElement) bool {
	return markdownAppendNeedsParagraphBoundary(elements) && !docRangeStartsParagraph(doc, startIdx, tabID)
}

type docsMarkdownInsertResult struct {
	RequestCount int
	Inserted     int
	ContentStart int64
	ContentEnd   int64
}

func insertPreparedDocsMarkdownAt(
	ctx context.Context,
	svc *docs.Service,
	docID string,
	insertIdx int64,
	markdown preparedMarkdown,
	tabID string,
	stripHeadingAnchors bool,
) (docsMarkdownInsertResult, error) {
	return insertPreparedDocsMarkdownAtWithWriteControl(ctx, svc, docID, insertIdx, markdown, tabID, stripHeadingAnchors, nil)
}

func insertPreparedDocsMarkdownAtWithWriteControl(
	ctx context.Context,
	svc *docs.Service,
	docID string,
	insertIdx int64,
	markdown preparedMarkdown,
	tabID string,
	stripHeadingAnchors bool,
	writeControl *docs.WriteControl,
) (docsMarkdownInsertResult, error) {
	elements := docsmarkdown.ParseMarkdown(markdown.cleaned)
	if stripHeadingAnchors {
		docsmarkdown.StripElementHeadingAnchors(elements)
	}
	prefix := ""
	baseIndex := insertIdx
	if insertIdx > 1 && markdownAppendNeedsParagraphBoundary(elements) {
		prefix = "\n"
		baseIndex++
	}

	result := docsMarkdownInsertResult{
		ContentStart: baseIndex,
		ContentEnd:   insertIdx,
	}
	formattingRequests, textToInsert, tables := docsmarkdown.MarkdownToDocsRequests(elements, baseIndex, tabID)
	if textToInsert == "" {
		return result, nil
	}
	result.Inserted = len(prefix) + len(textToInsert)
	result.ContentEnd = insertIdx + utf16Len(prefix+textToInsert)

	applyTabIDToFormattingRequests(formattingRequests, tabID)

	// Body InsertText + per-element formatting in one batchUpdate. Tables
	// follow via InsertNativeTable (one InsertTable + one cell batch per
	// table — see #699 follow-up).
	requests := make([]*docs.Request, 0, 1+len(formattingRequests))
	requests = append(requests, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: insertIdx, TabId: tabID},
			Text:     prefix + textToInsert,
		},
	})
	requests = append(requests, formattingRequests...)

	requestCount, err := submitBatchedDocsRequests(ctx, svc, docID, requests, writeControl)
	if err != nil {
		return docsMarkdownInsertResult{}, fmt.Errorf("append (markdown): %w", err)
	}
	result.RequestCount = requestCount

	if len(tables) > 0 {
		tableInserter := NewTableInserter(svc, docID)
		tableOffset := int64(0)
		for _, table := range tables {
			tableIndex := table.StartIndex + tableOffset
			tableEnd, tableErr := tableInserter.InsertNativeTable(ctx, tableIndex, table.Cells, tabID)
			if tableErr != nil {
				return result, fmt.Errorf("insert native table: %w", tableErr)
			}
			tableOffset = nextTableInsertOffset(tableOffset, tableIndex, tableEnd)
		}
		result.ContentEnd += tableOffset
	}

	if len(markdown.images) > 0 {
		imgErr := insertImagesIntoDocs(ctx, svc, docID, markdown.images, tabID)
		cleanupDocsImagePlaceholders(ctx, svc, docID, markdown.images, tabID)
		if imgErr != nil {
			return result, fmt.Errorf("insert images: %w", imgErr)
		}
		result.ContentEnd = subtractMarkdownImagePlaceholderDrift(result.ContentEnd, insertIdx, markdown.images)
	}

	return result, nil
}

func subtractMarkdownImagePlaceholderDrift(index int64, floor int64, images []markdownImage) int64 {
	for _, img := range images {
		drift := utf16Len(img.placeholder()) - 1
		if drift > 0 {
			index -= drift
		}
	}
	if index < floor {
		return floor
	}
	return index
}

// applyTabIDToFormattingRequests propagates tabID to every request whose
// range needs to be tab-scoped. Centralised so both the append and replace
// markdown paths stay in sync — previously each duplicated the same eight
// nil-guarded assignments inline.
func applyTabIDToFormattingRequests(requests []*docs.Request, tabID string) {
	if tabID == "" {
		return
	}
	for _, req := range requests {
		if req == nil {
			continue
		}
		if req.UpdateTextStyle != nil && req.UpdateTextStyle.Range != nil {
			req.UpdateTextStyle.Range.TabId = tabID
		}
		if req.UpdateParagraphStyle != nil && req.UpdateParagraphStyle.Range != nil {
			req.UpdateParagraphStyle.Range.TabId = tabID
		}
		if req.CreateParagraphBullets != nil && req.CreateParagraphBullets.Range != nil {
			req.CreateParagraphBullets.Range.TabId = tabID
		}
		if req.DeleteParagraphBullets != nil && req.DeleteParagraphBullets.Range != nil {
			req.DeleteParagraphBullets.Range.TabId = tabID
		}
	}
}

// submitBatchedDocsRequests sends the supplied request list as one or more
// documents.batchUpdate calls, splitting at docsBatchUpdateRequestCap-sized
// chunks when the consolidated request count exceeds the Docs API per-batch
// hard limit. Each chunk preserves the source order so cell-index
// arithmetic remains consistent across the split. Returns the total number
// of requests submitted (matches len(requests) on success); chunk events are
// announced on stderr so callers can correlate wire traffic with logs. When
// revision control is supplied, each response's updated control is chained
// into the next chunk so stale structural indexes fail closed.
func submitBatchedDocsRequests(ctx context.Context, svc *docs.Service, docID string, requests []*docs.Request, writeControl *docs.WriteControl) (int, error) {
	count, _, err := submitBatchedDocsRequestsWithRevision(ctx, svc, docID, requests, writeControl)
	return count, err
}

func submitBatchedDocsRequestsWithRevision(
	ctx context.Context,
	svc *docs.Service,
	docID string,
	requests []*docs.Request,
	writeControl *docs.WriteControl,
) (int, string, error) {
	if len(requests) == 0 {
		return 0, docsRequiredRevisionID(writeControl), nil
	}
	if len(requests) <= docsBatchUpdateRequestCap {
		resp, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
			WriteControl: writeControl,
			Requests:     requests,
		}).Context(ctx).Do()
		if err != nil {
			return 0, "", err
		}
		return len(requests), docsBatchResponseRevisionID(resp), nil
	}

	totalChunks := (len(requests) + docsBatchUpdateRequestCap - 1) / docsBatchUpdateRequestCap
	revisionID := docsRequiredRevisionID(writeControl)
	for i := 0; i < len(requests); i += docsBatchUpdateRequestCap {
		end := i + docsBatchUpdateRequestCap
		if end > len(requests) {
			end = len(requests)
		}
		chunkIdx := i/docsBatchUpdateRequestCap + 1
		fmt.Fprintf(stderrWriter(ctx), "gog: docs batchUpdate split %d/%d (%d requests; Docs API per-call cap is %d)\n",
			chunkIdx, totalChunks, end-i, docsBatchUpdateRequestCap)
		resp, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
			WriteControl: writeControl,
			Requests:     requests[i:end],
		}).Context(ctx).Do()
		if err != nil {
			return i, revisionID, err
		}
		if end < len(requests) && writeControl != nil {
			if resp == nil || resp.WriteControl == nil || resp.WriteControl.RequiredRevisionId == "" {
				return end, revisionID, fmt.Errorf("docs batchUpdate split %d/%d did not return required revision control", chunkIdx, totalChunks)
			}
			revisionID = resp.WriteControl.RequiredRevisionId
			writeControl = &docs.WriteControl{RequiredRevisionId: revisionID}
		} else if responseRevisionID := docsBatchResponseRevisionID(resp); responseRevisionID != "" {
			revisionID = responseRevisionID
		}
	}
	return len(requests), revisionID, nil
}

func docsRequiredRevisionID(writeControl *docs.WriteControl) string {
	if writeControl == nil {
		return ""
	}
	return writeControl.RequiredRevisionId
}

func docsBatchResponseRevisionID(resp *docs.BatchUpdateDocumentResponse) string {
	if resp == nil || resp.WriteControl == nil {
		return ""
	}
	return resp.WriteControl.RequiredRevisionId
}

func markdownAppendNeedsParagraphBoundary(elements []docsmarkdown.MarkdownElement) bool {
	if len(elements) == 0 {
		return false
	}
	switch elements[0].Type {
	case docsmarkdown.MDEmptyLine, docsmarkdown.MDParagraph:
		return false
	default:
		return true
	}
}

func docRangeStartsParagraph(doc *docs.Document, startIdx int64, tabID string) bool {
	if doc == nil {
		return false
	}
	if tabID != "" {
		tab, err := findTab(flattenTabs(doc.Tabs), tabID)
		if err != nil || tab.DocumentTab == nil {
			return false
		}
		return bodyHasParagraphStart(tab.DocumentTab.Body, startIdx)
	}
	return bodyHasParagraphStart(doc.Body, startIdx)
}

func docRangeCoversParagraphText(doc *docs.Document, startIdx, endIdx int64, tabID string) bool {
	if doc == nil {
		return false
	}
	if tabID != "" {
		tab, err := findTab(flattenTabs(doc.Tabs), tabID)
		if err != nil || tab.DocumentTab == nil {
			return false
		}
		return elementsContainParagraphTextRange(tab.DocumentTab.Body.Content, startIdx, endIdx)
	}
	if doc.Body == nil {
		return false
	}
	return elementsContainParagraphTextRange(doc.Body.Content, startIdx, endIdx)
}

func elementsContainParagraphTextRange(elements []*docs.StructuralElement, startIdx, endIdx int64) bool {
	for _, el := range elements {
		if el == nil {
			continue
		}
		if el.Paragraph != nil && paragraphTextStart(el) == startIdx && el.EndIndex == endIdx+1 {
			return true
		}
		if el.Table != nil {
			for _, row := range el.Table.TableRows {
				for _, cell := range row.TableCells {
					if elementsContainParagraphTextRange(cell.Content, startIdx, endIdx) {
						return true
					}
				}
			}
		}
	}
	return false
}

func bodyHasParagraphStart(body *docs.Body, startIdx int64) bool {
	if body == nil {
		return false
	}
	return elementsHaveParagraphStart(body.Content, startIdx)
}

func elementsHaveParagraphStart(elements []*docs.StructuralElement, startIdx int64) bool {
	for _, el := range elements {
		if el == nil {
			continue
		}
		if el.Paragraph != nil && paragraphTextStart(el) == startIdx {
			return true
		}
		if el.Table != nil {
			for _, row := range el.Table.TableRows {
				for _, cell := range row.TableCells {
					if elementsHaveParagraphStart(cell.Content, startIdx) {
						return true
					}
				}
			}
		}
	}
	return false
}

func paragraphTextStart(el *docs.StructuralElement) int64 {
	for _, pe := range el.Paragraph.Elements {
		if pe != nil && pe.TextRun != nil {
			return pe.StartIndex
		}
	}
	return el.StartIndex
}

func cleanupDocsImagePlaceholders(ctx context.Context, svc *docs.Service, docID string, images []markdownImage, tabID string) {
	reqs := make([]*docs.Request, 0, len(images))
	for _, img := range images {
		req := &docs.Request{
			ReplaceAllText: &docs.ReplaceAllTextRequest{
				ContainsText: &docs.SubstringMatchCriteria{
					Text:      img.placeholder(),
					MatchCase: true,
				},
				ReplaceText: "",
			},
		}
		if tabID != "" {
			req.ReplaceAllText.TabsCriteria = &docs.TabsCriteria{TabIds: []string{tabID}}
		}
		reqs = append(reqs, req)
	}
	_, _ = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: reqs,
	}).Context(ctx).Do()
}
