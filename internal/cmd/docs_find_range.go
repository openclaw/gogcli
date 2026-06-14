package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docsedit"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsFindRangeCmd struct {
	DocID               string `arg:"" name:"docId" help:"Doc ID"`
	Text                string `arg:"" name:"text" help:"Text to find"`
	MatchCase           bool   `name:"match-case" help:"Use case-sensitive matching"`
	All                 bool   `name:"all" help:"Return all matches"`
	Occurrence          *int   `name:"occurrence" help:"Return the Nth occurrence (1-based; default first)"`
	NormalizeWhitespace bool   `name:"normalize-whitespace" help:"Collapse whitespace while matching" default:"true" negatable:""`
	FailEmpty           bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no matches"`
	Tab                 string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID               string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

type docsFindRangeResult struct {
	Matches []docsedit.TextRange `json:"matches"`
}

func (c *DocsFindRangeCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	if c.Text == "" {
		return usage("empty text")
	}
	if c.All && c.Occurrence != nil {
		return usage("--all and --occurrence are mutually exclusive")
	}
	if c.Occurrence != nil && *c.Occurrence <= 0 {
		return usage("--occurrence must be > 0")
	}

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	doc, tabID, err := c.loadTargetDoc(ctx, svc, id)
	if err != nil {
		return err
	}
	matches := docsedit.FindTextRanges(doc, c.Text, docsedit.SearchOptions{
		MatchCase:           c.MatchCase,
		NormalizeWhitespace: c.NormalizeWhitespace,
		TabID:               tabID,
	})
	matches = c.selectMatches(matches)

	return c.writeResult(ctx, matches)
}

func (c *DocsFindRangeCmd) loadTargetDoc(ctx context.Context, svc *docs.Service, docID string) (*docs.Document, string, error) {
	getCall := svc.Documents.Get(docID).Context(ctx)
	if c.Tab != "" {
		getCall = getCall.IncludeTabsContent(true)
	}
	doc, err := getCall.Do()
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

	if c.Tab == "" {
		return doc, "", nil
	}
	tab, tabErr := findTab(flattenTabs(doc.Tabs), c.Tab)
	if tabErr != nil {
		return nil, "", tabErr
	}
	tabID := ""
	if tab.TabProperties != nil {
		tabID = tab.TabProperties.TabId
	}
	targetDoc := &docs.Document{}
	if tab.DocumentTab != nil {
		targetDoc.Body = tab.DocumentTab.Body
	}
	return targetDoc, tabID, nil
}

func (c *DocsFindRangeCmd) selectMatches(matches []docsedit.TextRange) []docsedit.TextRange {
	if c.All {
		return matches
	}
	occurrence := 1
	if c.Occurrence != nil {
		occurrence = *c.Occurrence
	}
	if occurrence > len(matches) {
		return nil
	}
	return matches[occurrence-1 : occurrence]
}

func (c *DocsFindRangeCmd) writeResult(ctx context.Context, matches []docsedit.TextRange) error {
	if matches == nil {
		matches = []docsedit.TextRange{}
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), docsFindRangeResult{Matches: matches}); err != nil {
			return err
		}
		return failEmptyIfNoDocsRange(c.FailEmpty, matches)
	}

	u := ui.FromContext(ctx)
	for _, match := range matches {
		u.Out().Linef("%d\t%d\t%d\t%s", match.StartIndex, match.EndIndex, match.ParagraphIndex, match.TabID)
	}
	return failEmptyIfNoDocsRange(c.FailEmpty, matches)
}

func failEmptyIfNoDocsRange(failEmpty bool, matches []docsedit.TextRange) error {
	if len(matches) == 0 {
		return failEmptyExit(failEmpty)
	}
	return nil
}
