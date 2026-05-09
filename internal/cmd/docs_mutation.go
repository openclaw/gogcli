package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"
)

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

func replaceDocsMarkdownRange(ctx context.Context, svc *docs.Service, doc *docs.Document, startIdx, endIdx int64, replaceText string, tabID string) error {
	cleaned, images := extractMarkdownImages(replaceText)
	elements := ParseMarkdown(cleaned)
	formattingRequests, textToInsert, tables := MarkdownToDocsRequests(elements, startIdx, tabID)

	for _, req := range formattingRequests {
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
				Text:     textToInsert,
			},
		})
		requests = append(requests, formattingRequests...)
	}

	_, err := svc.Documents.BatchUpdate(doc.DocumentId, &docs.BatchUpdateDocumentRequest{
		WriteControl: &docs.WriteControl{RequiredRevisionId: doc.RevisionId},
		Requests:     requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("replace (markdown): %w", err)
	}

	if len(tables) > 0 {
		tableInserter := NewTableInserter(svc, doc.DocumentId)
		tableOffset := int64(0)
		for _, table := range tables {
			tableIndex := table.StartIndex + tableOffset
			tableEnd, tableErr := tableInserter.InsertNativeTable(ctx, tableIndex, table.Cells, tabID)
			if tableErr != nil {
				return fmt.Errorf("insert native table: %w", tableErr)
			}
			if tableEnd > tableIndex {
				tableOffset += (tableEnd - tableIndex) - 1
			}
		}
	}

	if len(images) > 0 {
		imgErr := insertImagesIntoDocs(ctx, svc, doc.DocumentId, images, tabID)
		cleanupDocsImagePlaceholders(ctx, svc, doc.DocumentId, images, tabID)
		if imgErr != nil {
			return fmt.Errorf("insert images: %w", imgErr)
		}
	}

	return nil
}

func insertDocsMarkdownAt(ctx context.Context, svc *docs.Service, docID string, insertIdx int64, content string, tabID string) (requestCount int, inserted int, err error) {
	cleaned, images := extractMarkdownImages(content)
	elements := ParseMarkdown(cleaned)
	formattingRequests, textToInsert, tables := MarkdownToDocsRequests(elements, insertIdx, tabID)
	if textToInsert == "" {
		return 0, 0, nil
	}

	for _, req := range formattingRequests {
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

	requests := make([]*docs.Request, 0, 1+len(formattingRequests))
	requests = append(requests, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: insertIdx, TabId: tabID},
			Text:     textToInsert,
		},
	})
	requests = append(requests, formattingRequests...)

	_, err = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return 0, 0, fmt.Errorf("append (markdown): %w", err)
	}

	if len(tables) > 0 {
		tableInserter := NewTableInserter(svc, docID)
		tableOffset := int64(0)
		for _, table := range tables {
			tableIndex := table.StartIndex + tableOffset
			tableEnd, tableErr := tableInserter.InsertNativeTable(ctx, tableIndex, table.Cells, tabID)
			if tableErr != nil {
				return len(requests), len(textToInsert), fmt.Errorf("insert native table: %w", tableErr)
			}
			if tableEnd > tableIndex {
				tableOffset += (tableEnd - tableIndex) - 1
			}
		}
	}

	if len(images) > 0 {
		imgErr := insertImagesIntoDocs(ctx, svc, docID, images, tabID)
		cleanupDocsImagePlaceholders(ctx, svc, docID, images, tabID)
		if imgErr != nil {
			return len(requests), len(textToInsert), fmt.Errorf("insert images: %w", imgErr)
		}
	}

	return len(requests), len(textToInsert), nil
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

func findTextInDoc(doc *docs.Document, searchText string, matchCase bool) (int64, int64, int) {
	matches := findTextMatches(doc, searchText, matchCase)
	if len(matches) == 0 {
		return 0, 0, 0
	}
	return matches[0].startIndex, matches[0].endIndex, len(matches)
}

func findTextMatches(doc *docs.Document, searchText string, matchCase bool) []docRange {
	if doc == nil || doc.Body == nil {
		return nil
	}

	find := searchText
	if !matchCase {
		find = strings.ToLower(find)
	}

	var matches []docRange
	findTextInElements(doc.Body.Content, searchText, find, matchCase, &matches)
	return matches
}

func findTextInElements(elements []*docs.StructuralElement, searchText, find string, matchCase bool, matches *[]docRange) {
	for _, el := range elements {
		if el == nil {
			continue
		}
		switch {
		case el.Paragraph != nil:
			findTextInParagraph(el.Paragraph, searchText, find, matchCase, matches)
		case el.Table != nil:
			for _, row := range el.Table.TableRows {
				for _, cell := range row.TableCells {
					findTextInElements(cell.Content, searchText, find, matchCase, matches)
				}
			}
		}
	}
}

func findTextInParagraph(para *docs.Paragraph, searchText, find string, matchCase bool, matches *[]docRange) {
	var paraText strings.Builder
	var paraStart int64
	first := true
	for _, pe := range para.Elements {
		if pe.TextRun == nil {
			continue
		}
		if first {
			paraStart = pe.StartIndex
			first = false
		}
		paraText.WriteString(pe.TextRun.Content)
	}
	if paraText.Len() == 0 {
		return
	}

	text := paraText.String()
	compareText := text
	if !matchCase {
		compareText = strings.ToLower(text)
	}

	offset := 0
	for {
		idx := strings.Index(compareText[offset:], find)
		if idx < 0 {
			break
		}
		absIdx := offset + idx
		matchStart := paraStart + utf16Len(text[:absIdx])
		matchEnd := matchStart + utf16Len(searchText)
		*matches = append(*matches, docRange{startIndex: matchStart, endIndex: matchEnd})
		offset = absIdx + len(find)
	}
}
