package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/docsedit"
	"github.com/openclaw/gogcli/internal/docsmarkdown"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type DocsInsertCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Content    string `arg:"" optional:"" name:"content" help:"Text to insert (or use --file / stdin)"`
	Index      *int64 `name:"index" help:"Character index to insert at (1 = beginning). Defaults to end-of-doc when omitted."`
	At         string `name:"at" help:"Anchor by literal text and insert at the start of the matched range"`
	Occurrence *int   `name:"occurrence" help:"Use the Nth --at match (1-based; required when --at is ambiguous)"`
	MatchCase  bool   `name:"match-case" help:"Use case-sensitive --at matching"`
	File       string `name:"file" short:"f" help:"Read content from file (use - for stdin)"`
	Markdown   bool   `name:"markdown" help:"Convert markdown to Google Docs formatting before inserting"`
	Tab        string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	Segment    string `name:"segment" help:"Target an exact header, footer, or footnote segment ID"`
	TabID      string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
	Batch      string `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
}

func (c *DocsInsertCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	content, err := resolveContentInput(ctx, c.Content, c.File)
	if err != nil {
		return err
	}
	if content == "" {
		return usage("no content provided (use argument, --file, or stdin)")
	}
	placement, err := docsedit.PlanInsertPlacement(docsedit.InsertPlacementOptions{
		Index:     c.Index,
		AllowZero: strings.TrimSpace(c.Segment) != "",
		Anchor: docsedit.AnchorOptions{
			Text:       c.At,
			Provided:   flagProvided(kctx, "at"),
			Occurrence: c.Occurrence,
			MatchCase:  c.MatchCase,
		},
	})
	if err != nil {
		return usage(err.Error())
	}
	at := placement.Anchor.Text

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab
	if strings.TrimSpace(c.Segment) != "" && c.Markdown {
		return usage("--segment supports plain-text inserts only; markdown can contain structures that are invalid in segments")
	}
	if c.Markdown && c.Batch != "" {
		return usage("--markdown cannot be combined with --batch")
	}
	dryRunPayload := map[string]any{
		"documentId": docID,
		"inserted":   len(content),
		"markdown":   c.Markdown,
		"tab":        c.Tab,
		"segment":    c.Segment,
		"batch":      c.Batch,
	}
	switch placement.Kind {
	case docsedit.PlacementAnchor:
		dryRunPayload["atIndex"] = docsAtIndexAnchorStart
		addDocsAtAnchorDryRunPayload(dryRunPayload, docsAtAnchorFlags{At: at, Occurrence: c.Occurrence, MatchCase: c.MatchCase})
	case docsedit.PlacementIndex:
		dryRunPayload["atIndex"] = placement.Index
	default:
		dryRunPayload["atIndex"] = docsAtIndexEnd
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.insert", dryRunPayload); dryRunErr != nil {
		return dryRunErr
	}
	if batchErr := validateDocsBatchTarget(ctx, flags, c.Batch, docID); batchErr != nil {
		return batchErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	batchRevision, err := captureDocsBatchRevision(ctx, svc, c.Batch, docID)
	if err != nil {
		return err
	}

	resolvedPlacement, err := resolveDocsPlacementTarget(ctx, svc, docID, c.Tab, c.Segment, placement)
	if err != nil {
		return err
	}
	insertIndex := resolvedPlacement.Index
	c.Tab = resolvedPlacement.TabID
	c.Segment = resolvedPlacement.SegmentID

	if c.Markdown {
		return c.runMarkdown(ctx, svc, docID, insertIndex, resolvedPlacement.RequiredRevisionID, content)
	}

	batchReq := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{docsedit.BuildInsertRequest(content, insertIndex, c.Tab)},
	}
	applyDocsRequestTarget(batchReq.Requests, docsTargetFromPlacement(resolvedPlacement.ResolvedPlacement))
	batchReq.WriteControl = docsRequiredRevisionWriteControl(resolvedPlacement.RequiredRevisionID)
	if queued, queueErr := queueDocsBatchRequests(ctx, flags, c.Batch, docID, "docs.insert", batchRevision, batchReq.Requests, false); queued || queueErr != nil {
		return queueErr
	}
	result, err := svc.Documents.BatchUpdate(docID, batchReq).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("inserting text: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{"documentId": result.DocumentId, "inserted": len(content), "atIndex": insertIndex}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		if c.Segment != "" {
			payload["segmentId"] = c.Segment
			payload["segmentType"] = resolvedPlacement.SegmentKind
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("documentId\t%s", result.DocumentId)
	u.Out().Linef("inserted\t%d bytes", len(content))
	u.Out().Linef("atIndex\t%d", insertIndex)
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	if c.Segment != "" {
		u.Out().Linef("segmentId\t%s", c.Segment)
		u.Out().Linef("segmentType\t%s", resolvedPlacement.SegmentKind)
	}
	return nil
}

// runMarkdown converts the supplied content from markdown to Google Docs
// formatting and inserts it at insertIndex. It reuses the same converter +
// insertion helper that backs `docs write --markdown` and the non-replacing
// branch of `docs update --markdown`, so headings, fenced code blocks, lists,
// tables and images render identically regardless of which command placed them.
func (c *DocsInsertCmd) runMarkdown(
	ctx context.Context,
	svc *docs.Service,
	docID string,
	insertIndex int64,
	requiredRevisionID string,
	content string,
) error {
	markdown := prepareMarkdown(content)
	insertResult, err := insertPreparedDocsMarkdownAtWithWriteControl(
		ctx,
		svc,
		docID,
		insertIndex,
		markdown,
		c.Tab,
		true,
		docsRequiredRevisionWriteControl(requiredRevisionID),
	)
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}
	requestCount := insertResult.RequestCount
	if markdownMayContainHeadingLinks(markdown.cleaned) {
		explicitHeadingAnchors := docsmarkdown.ExplicitHeadingAnchors(markdown.cleaned)
		rewritten, rewriteErr := rewriteMarkdownHeadingLinksInRange(
			ctx,
			svc,
			docID,
			c.Tab,
			explicitHeadingAnchors,
			insertResult.ContentStart,
			insertResult.ContentEnd,
		)
		if rewriteErr != nil {
			return fmt.Errorf("rewrite heading links: %w", rewriteErr)
		}
		requestCount += rewritten
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
			"inserted":   insertResult.Inserted,
			"requests":   requestCount,
			"atIndex":    insertIndex,
			"markdown":   true,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("inserted\t%d", insertResult.Inserted)
	u.Out().Linef("requests\t%d", requestCount)
	u.Out().Linef("atIndex\t%d", insertIndex)
	u.Out().Linef("markdown\ttrue")
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	return nil
}
