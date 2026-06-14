package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/docsedit"
	"github.com/steipete/gogcli/internal/docsmarkdown"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type docsWriteOrphanComment struct {
	CommentID   string `json:"commentId"`
	Author      string `json:"author,omitempty"`
	AuthorEmail string `json:"authorEmail,omitempty"`
	Content     string `json:"content"`
	Quote       string `json:"quote"`
}

type docsWriteOrphanResult struct {
	DocumentID  string                   `json:"documentId"`
	TabID       string                   `json:"tabId,omitempty"`
	WouldOrphan []docsWriteOrphanComment `json:"wouldOrphan"`
}

func findDocsWriteMarkdownOrphans(
	ctx context.Context,
	driveSvc *drive.Service,
	docsSvc *docs.Service,
	docID string,
	markdown preparedMarkdown,
	tabQuery string,
	wholeDocument bool,
) ([]docsWriteOrphanComment, string, error) {
	doc, err := docsSvc.Documents.Get(docID).Context(ctx).IncludeTabsContent(true).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return nil, "", fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return nil, "", err
	}
	doc, err = requireRawResponse(doc, "doc not found")
	if err != nil {
		return nil, "", err
	}

	targetTabID, err := docsWriteOrphanTargetTabID(doc, tabQuery, wholeDocument)
	if err != nil {
		return nil, "", err
	}
	comments, _, err := listDriveComments(ctx, driveSvc, docID, driveCommentListOptions{
		includeQuoted: true,
		all:           true,
		max:           100,
		mode:          driveCommentListModeExpanded,
	})
	if err != nil {
		return nil, "", fmt.Errorf("list document comments: %w", err)
	}

	outgoing := docsWriteMarkdownDocument(markdown)
	locator := DocsCommentsLocateCmd{NormalizeWhitespace: true}
	var orphans []docsWriteOrphanComment
	for _, comment := range comments {
		quote := strings.TrimSpace(docsCommentQuote(comment))
		if quote == "" {
			continue
		}

		currentMatches := locator.findQuoteMatchesAcrossDocument(doc, quote)
		if len(currentMatches) == 0 || !docsWriteCommentTouchesTarget(currentMatches, targetTabID, wholeDocument) {
			continue
		}
		if len(locator.findQuoteMatchesInDoc(outgoing, quote, targetTabID)) > 0 {
			continue
		}

		orphan := docsWriteOrphanComment{
			CommentID: comment.Id,
			Content:   comment.Content,
			Quote:     docsCommentQuote(comment),
		}
		if comment.Author != nil {
			orphan.Author = comment.Author.DisplayName
			orphan.AuthorEmail = comment.Author.EmailAddress
		}
		orphans = append(orphans, orphan)
	}
	sort.SliceStable(orphans, func(i, j int) bool {
		left := strings.ToLower(orphans[i].Author + "\x00" + orphans[i].AuthorEmail + "\x00" + orphans[i].CommentID)
		right := strings.ToLower(orphans[j].Author + "\x00" + orphans[j].AuthorEmail + "\x00" + orphans[j].CommentID)
		return left < right
	})
	return orphans, targetTabID, nil
}

func docsWriteOrphanTargetTabID(doc *docs.Document, tabQuery string, wholeDocument bool) (string, error) {
	if wholeDocument {
		return "", nil
	}
	tabs := flattenTabs(doc.Tabs)
	if strings.TrimSpace(tabQuery) == "" {
		if len(tabs) == 0 {
			return "", nil
		}
		if tabs[0].TabProperties == nil || strings.TrimSpace(tabs[0].TabProperties.TabId) == "" {
			return "", fmt.Errorf("first tab has no ID")
		}
		return tabs[0].TabProperties.TabId, nil
	}
	tab, err := findTab(tabs, tabQuery)
	if err != nil {
		return "", err
	}
	if tab.TabProperties == nil || strings.TrimSpace(tab.TabProperties.TabId) == "" {
		return "", fmt.Errorf("tab has no ID: %s", tabQuery)
	}
	return tab.TabProperties.TabId, nil
}

func docsWriteCommentTouchesTarget(matches []docsedit.TextRange, targetTabID string, wholeDocument bool) bool {
	if wholeDocument {
		return len(matches) > 0
	}
	for _, match := range matches {
		if match.TabID == targetTabID {
			return true
		}
	}
	return false
}

func docsWriteMarkdownDocument(markdown preparedMarkdown) *docs.Document {
	cleaned := markdown.cleaned
	for _, image := range markdown.images {
		cleaned = strings.ReplaceAll(cleaned, image.placeholder(), "")
	}

	doc := &docs.Document{Body: &docs.Body{}}
	index := int64(1)
	elements := docsmarkdown.ParseMarkdown(cleaned)
	docsmarkdown.StripElementHeadingAnchors(elements)
	for _, element := range elements {
		if element.Type == docsmarkdown.MDTable {
			table, next := docsWriteMarkdownTable(element.TableCells, index)
			if table != nil {
				doc.Body.Content = append(doc.Body.Content, table)
				index = next
			}
			continue
		}
		_, text, _ := docsmarkdown.MarkdownToDocsRequests([]docsmarkdown.MarkdownElement{element}, index, "")
		if text == "" {
			continue
		}
		doc.Body.Content = append(doc.Body.Content, docsWriteMarkdownParagraph(index, text))
		index += utf16Len(text)
	}
	return doc
}

func docsWriteMarkdownTable(cells [][]string, index int64) (*docs.StructuralElement, int64) {
	if len(cells) == 0 {
		return nil, index
	}
	start := index
	table := &docs.Table{}
	for _, rowCells := range cells {
		row := &docs.TableRow{}
		for _, cellMarkdown := range rowCells {
			elements := docsmarkdown.ParseMarkdown(cellMarkdown)
			docsmarkdown.StripElementHeadingAnchors(elements)
			_, text, _ := docsmarkdown.MarkdownToDocsRequests(elements, index, "")
			if text == "" {
				text = "\n"
			}
			paragraph := docsWriteMarkdownParagraph(index, text)
			cell := &docs.TableCell{
				Content:    []*docs.StructuralElement{paragraph},
				StartIndex: index,
				EndIndex:   index + utf16Len(text),
			}
			index = cell.EndIndex
			row.TableCells = append(row.TableCells, cell)
		}
		table.TableRows = append(table.TableRows, row)
	}
	return &docs.StructuralElement{StartIndex: start, EndIndex: index, Table: table}, index
}

func docsWriteMarkdownParagraph(index int64, text string) *docs.StructuralElement {
	end := index + utf16Len(text)
	return &docs.StructuralElement{
		StartIndex: index,
		EndIndex:   end,
		Paragraph: &docs.Paragraph{
			Elements: []*docs.ParagraphElement{{
				StartIndex: index,
				EndIndex:   end,
				TextRun:    &docs.TextRun{Content: text},
			}},
		},
	}
}

func writeDocsWriteOrphanResult(ctx context.Context, docID, tabID string, orphans []docsWriteOrphanComment) error {
	if len(orphans) == 0 {
		return nil
	}
	result := docsWriteOrphanResult{
		DocumentID:  docID,
		TabID:       tabID,
		WouldOrphan: orphans,
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), result); err != nil {
			return err
		}
		return &ExitError{Code: exitCodeOrphaned, Err: nil}
	}

	u := ui.FromContext(ctx)
	u.Err().Linef("docs write blocked: %d open comment(s) would become orphaned", len(orphans))
	lastAuthor := ""
	for _, orphan := range orphans {
		author := strings.TrimSpace(orphan.Author)
		if author == "" {
			author = strings.TrimSpace(orphan.AuthorEmail)
		}
		if author == "" {
			author = "(unknown author)"
		}
		if author != lastAuthor {
			u.Err().Linef("%s", author)
			lastAuthor = author
		}
		u.Err().Linef("  %s\t%s\t%s",
			orphan.CommentID,
			truncateString(oneLineTSV(orphan.Content), 80),
			truncateString(oneLineTSV(orphan.Quote), 60),
		)
	}
	return &ExitError{Code: exitCodeOrphaned, Err: nil}
}
