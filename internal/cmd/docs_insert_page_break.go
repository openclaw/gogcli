package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docsedit"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DocsInsertPageBreakCmd inserts a page break at a specific character index in
// a Google Doc (or at the end of the body/tab when --at-end is supplied, or
// --index is omitted). Surfaces the Docs API InsertPageBreakRequest directly,
// since markdown has no native page-break construct that the markdown writer
// could translate.
type DocsInsertPageBreakCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Index      *int64 `name:"index" help:"Character index to insert at (1 = beginning). Omit or use --at-end for end-of-doc."`
	At         string `name:"at" help:"Anchor by literal text and insert at the start of the matched range"`
	Occurrence *int   `name:"occurrence" help:"Use the Nth --at match (1-based; required when --at is ambiguous)"`
	MatchCase  bool   `name:"match-case" help:"Use case-sensitive --at matching"`
	AtEnd      bool   `name:"at-end" help:"Insert at end-of-doc/tab (mutually exclusive with --index)"`
	Tab        string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID      string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
	Batch      string `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
}

func (c *DocsInsertPageBreakCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	placement, err := docsedit.PlanEndInsertPlacement(docsedit.EndInsertPlacementOptions{
		Index: c.Index,
		AtEnd: c.AtEnd,
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

	dryRunPayload := map[string]any{
		"documentId": docID,
		"tab":        c.Tab,
		"batch":      c.Batch,
	}
	switch placement.Kind {
	case docsedit.PlacementAnchor:
		dryRunPayload["atIndex"] = docsAtIndexAnchorStart
		addDocsAtAnchorDryRunPayload(dryRunPayload, docsAtAnchorFlags{At: at, Occurrence: c.Occurrence, MatchCase: c.MatchCase})
	case docsedit.PlacementEnd:
		dryRunPayload["atIndex"] = docsAtIndexEnd
	case docsedit.PlacementIndex:
		dryRunPayload["atIndex"] = placement.Index
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.insert-page-break", dryRunPayload); dryRunErr != nil {
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

	resolvedPlacement, err := resolveDocsPlacement(ctx, svc, docID, c.Tab, placement)
	if err != nil {
		return err
	}
	if resolvedPlacement.InTable {
		return usage("--at matched text inside a table; page breaks cannot be inserted inside tables")
	}
	insertIndex := resolvedPlacement.Index
	c.Tab = resolvedPlacement.TabID

	batchReq := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			InsertPageBreak: &docs.InsertPageBreakRequest{
				Location: &docs.Location{
					Index: insertIndex,
					TabId: c.Tab,
				},
			},
		}},
		WriteControl: docsRequiredRevisionWriteControl(resolvedPlacement.RequiredRevisionID),
	}
	if queued, queueErr := queueDocsBatchRequests(ctx, flags, c.Batch, docID, "docs.insert-page-break", batchRevision, batchReq.Requests, false); queued || queueErr != nil {
		return queueErr
	}
	result, err := svc.Documents.BatchUpdate(docID, batchReq).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("inserting page break: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": result.DocumentId,
			"atIndex":    insertIndex,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("documentId\t%s", result.DocumentId)
	u.Out().Linef("atIndex\t%d", insertIndex)
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	return nil
}
