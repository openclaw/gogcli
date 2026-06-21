package cmd

import (
	"context"
	"fmt"
	"html"
	"strings"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/docsedit"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsCommentsLocateCmd struct {
	DocID               string `arg:"" name:"docId" help:"Google Doc ID or URL"`
	CommentID           string `arg:"" name:"commentId" help:"Comment ID"`
	MatchCase           bool   `name:"match-case" help:"Use case-sensitive matching"`
	NormalizeWhitespace bool   `name:"normalize-whitespace" help:"Collapse whitespace while matching" default:"true" negatable:""`
	Tab                 string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID               string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

type docsCommentLocateResult struct {
	CommentID string               `json:"commentId"`
	Matches   []docsedit.TextRange `json:"matches"`
	Orphaned  bool                 `json:"orphaned"`
	Quote     string               `json:"quote"`
}

func (c *DocsCommentsLocateCmd) Run(ctx context.Context, flags *RootFlags) error {
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	commentID := strings.TrimSpace(c.CommentID)
	if docID == "" {
		return usage("empty docId")
	}
	if commentID == "" {
		return usage("empty commentId")
	}

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab

	_, driveSvc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	comment, err := getDriveComment(ctx, driveSvc, docID, commentID)
	if err != nil {
		return err
	}

	quote := docsCommentQuote(comment)
	var matches []docsedit.TextRange
	if strings.TrimSpace(quote) != "" {
		docsSvc, svcErr := requireDocsService(ctx, flags)
		if svcErr != nil {
			return svcErr
		}
		var findErr error
		matches, findErr = c.findQuoteMatches(ctx, docsSvc, docID, quote)
		if findErr != nil {
			return findErr
		}
	}

	return writeDocsCommentLocateResult(ctx, commentID, quote, matches)
}

func (c *DocsCommentsLocateCmd) findQuoteMatches(ctx context.Context, svc *docs.Service, docID string, quote string) ([]docsedit.TextRange, error) {
	if c.Tab != "" {
		findCmd := DocsFindRangeCmd{Tab: c.Tab}
		doc, target, err := findCmd.loadTargetDoc(ctx, svc, docID)
		if err != nil {
			return nil, err
		}
		return c.findQuoteMatchesInDoc(doc, quote, target.TabID), nil
	}

	doc, err := svc.Documents.Get(docID).Context(ctx).IncludeTabsContent(true).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return nil, fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return nil, err
	}
	doc, err = requireRawResponse(doc, "doc not found")
	if err != nil {
		return nil, err
	}

	return c.findQuoteMatchesAcrossDocument(doc, quote), nil
}

func (c *DocsCommentsLocateCmd) findQuoteMatchesAcrossDocument(doc *docs.Document, quote string) []docsedit.TextRange {
	var matches []docsedit.TextRange
	tabs := flattenTabs(doc.Tabs)
	if len(tabs) == 0 {
		return c.findQuoteMatchesInDoc(doc, quote, "")
	}
	for _, tab := range tabs {
		if tab == nil || tab.DocumentTab == nil {
			continue
		}
		tabID := ""
		if tab.TabProperties != nil {
			tabID = tab.TabProperties.TabId
		}
		tabDoc := &docs.Document{Body: tab.DocumentTab.Body}
		matches = append(matches, c.findQuoteMatchesInDoc(tabDoc, quote, tabID)...)
	}
	return matches
}

func (c *DocsCommentsLocateCmd) findQuoteMatchesInDoc(doc *docs.Document, quote string, tabID string) []docsedit.TextRange {
	opts := docsedit.SearchOptions{
		MatchCase:            c.MatchCase,
		NormalizeWhitespace:  c.NormalizeWhitespace,
		TabID:                tabID,
		PreserveHTMLEntities: true,
	}
	matches := docsedit.FindTextRanges(doc, quote, opts)
	if len(matches) > 0 {
		return matches
	}
	if html.UnescapeString(quote) != quote {
		opts.PreserveHTMLEntities = false
		return docsedit.FindTextRanges(doc, quote, opts)
	}
	return matches
}

func docsCommentQuote(comment *drive.Comment) string {
	if comment == nil || comment.QuotedFileContent == nil {
		return ""
	}
	return comment.QuotedFileContent.Value
}

func writeDocsCommentLocateResult(ctx context.Context, commentID, quote string, matches []docsedit.TextRange) error {
	if matches == nil {
		matches = []docsedit.TextRange{}
	}
	orphaned := len(matches) == 0
	result := docsCommentLocateResult{
		CommentID: commentID,
		Matches:   matches,
		Orphaned:  orphaned,
		Quote:     quote,
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}

	u := ui.FromContext(ctx)
	if orphaned {
		if strings.TrimSpace(quote) == "" {
			u.Err().Linef("comment %s has no quoted content", commentID)
		} else {
			u.Err().Linef("comment %s is orphaned", commentID)
		}
		return &ExitError{Code: exitCodeOrphaned, Err: nil}
	}

	for _, match := range matches {
		u.Out().Linef("%d\t%d\t%d\t%s", match.StartIndex, match.EndIndex, match.ParagraphIndex, match.TabID)
	}
	return nil
}
