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

type DocsBodyPlacementFlags struct {
	Index      *int64 `name:"index" help:"Character index (1 = beginning); omit for end-of-doc"`
	At         string `name:"at" help:"Anchor by literal text and use the start of the matched range"`
	Occurrence *int   `name:"occurrence" help:"Use the Nth --at match (1-based; required when --at is ambiguous)"`
	MatchCase  bool   `name:"match-case" help:"Use case-sensitive --at matching"`
	AtEnd      bool   `name:"at-end" help:"Target end-of-doc/tab (mutually exclusive with --index and --at)"`
	Tab        string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type DocsFootnoteCmd struct {
	DocID     string                 `arg:"" name:"docId" help:"Doc ID"`
	Text      string                 `name:"text" help:"Footnote text"`
	File      string                 `name:"file" help:"Read footnote text from a file ('-' for stdin)"`
	Placement DocsBodyPlacementFlags `embed:""`
}

type DocsSectionBreakCmd struct {
	DocID     string                 `arg:"" name:"docId" help:"Doc ID"`
	Type      string                 `name:"type" help:"Section type: next-page or continuous" default:"next-page"`
	Batch     string                 `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
	Placement DocsBodyPlacementFlags `embed:""`
}

type DocsHorizontalRuleCmd struct {
	DocID     string                 `arg:"" name:"docId" help:"Doc ID"`
	Batch     string                 `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
	Placement DocsBodyPlacementFlags `embed:""`
}

type DocsSectionColumnsCmd struct {
	DocID     string                 `arg:"" name:"docId" help:"Doc ID"`
	Count     int                    `name:"count" required:"" help:"Number of columns (1-3; 1 resets to one column)"`
	Separator string                 `name:"separator" help:"Column separator: none or between" default:"none"`
	Batch     string                 `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
	Placement DocsBodyPlacementFlags `embed:""`
}

type docsBodyMutation struct {
	action    string
	docID     string
	batch     string
	placement DocsBodyPlacementFlags
	payload   map[string]any
	build     func(docsResolvedPlacement) ([]*docs.Request, error)
}

func (p DocsBodyPlacementFlags) plan(kctx *kong.Context) (docsedit.Placement, error) {
	placement, err := docsedit.PlanEndInsertPlacement(docsedit.EndInsertPlacementOptions{
		Index: p.Index,
		AtEnd: p.AtEnd,
		Anchor: docsedit.AnchorOptions{
			Text:       p.At,
			Provided:   flagProvided(kctx, "at"),
			Occurrence: p.Occurrence,
			MatchCase:  p.MatchCase,
		},
	})
	if err != nil {
		return docsedit.Placement{}, usage(err.Error())
	}
	return placement, nil
}

func (p DocsBodyPlacementFlags) dryRunPayload(placement docsedit.Placement) map[string]any {
	payload := map[string]any{"tab": p.Tab}
	switch placement.Kind {
	case docsedit.PlacementAnchor:
		payload["atIndex"] = docsAtIndexAnchorStart
		addDocsAtAnchorDryRunPayload(payload, docsAtAnchorFlags{
			At: p.At, Occurrence: p.Occurrence, MatchCase: p.MatchCase,
		})
	case docsedit.PlacementIndex:
		payload["atIndex"] = placement.Index
	default:
		payload["atIndex"] = docsAtIndexEnd
	}
	return payload
}

func runDocsBodyMutation(ctx context.Context, kctx *kong.Context, flags *RootFlags, mutation docsBodyMutation) error {
	docID := normalizeGoogleID(strings.TrimSpace(mutation.docID))
	if docID == "" {
		return usage("empty docId")
	}
	placement, err := mutation.placement.plan(kctx)
	if err != nil {
		return err
	}
	payload := mutation.placement.dryRunPayload(placement)
	payload["documentId"] = docID
	payload["batch"] = mutation.batch
	for key, value := range mutation.payload {
		payload[key] = value
	}
	if dryRunErr := dryRunExit(ctx, flags, mutation.action, payload); dryRunErr != nil {
		return dryRunErr
	}
	if batchErr := validateDocsBatchTarget(ctx, flags, mutation.batch, docID); batchErr != nil {
		return batchErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	batchRevision, err := captureDocsBatchRevision(ctx, svc, mutation.batch, docID)
	if err != nil {
		return err
	}
	resolved, err := resolveDocsPlacement(ctx, svc, docID, mutation.placement.Tab, placement)
	if err != nil {
		return err
	}
	if resolved.InTable {
		return usage(fmt.Sprintf("%s cannot target text inside a table", mutation.action))
	}
	requests, err := mutation.build(resolved)
	if err != nil {
		return err
	}
	applyDocsRequestTarget(requests, docsTargetFromPlacement(resolved.ResolvedPlacement))
	if queued, queueErr := queueDocsBatchRequests(ctx, flags, mutation.batch, docID, mutation.action, batchRevision, requests, false); queued || queueErr != nil {
		return queueErr
	}
	response, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests:     requests,
		WriteControl: docsRequiredRevisionWriteControl(resolved.RequiredRevisionID),
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("%s: %w", mutation.action, err)
	}
	return writeDocsBodyMutationResult(ctx, response, resolved, len(requests), mutation.payload)
}

func writeDocsBodyMutationResult(ctx context.Context, response *docs.BatchUpdateDocumentResponse, resolved docsResolvedPlacement, requestCount int, payload map[string]any) error {
	documentID := ""
	if response != nil {
		documentID = response.DocumentId
	}
	result := map[string]any{
		"documentId": documentID,
		"atIndex":    resolved.Index,
		"requests":   requestCount,
	}
	if resolved.TabID != "" {
		result["tabId"] = resolved.TabID
	}
	for key, value := range payload {
		result[key] = value
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("documentId\t%s", documentID)
	u.Out().Linef("atIndex\t%d", resolved.Index)
	u.Out().Linef("requests\t%d", requestCount)
	if resolved.TabID != "" {
		u.Out().Linef("tabId\t%s", resolved.TabID)
	}
	return nil
}

func (c *DocsSectionBreakCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	sectionType, err := normalizeDocsSectionType(c.Type)
	if err != nil {
		return err
	}
	return runDocsBodyMutation(ctx, kctx, flags, docsBodyMutation{
		action: "docs.insert-section-break", docID: c.DocID, batch: c.Batch, placement: c.Placement,
		payload: map[string]any{"sectionType": sectionType},
		build: func(resolved docsResolvedPlacement) ([]*docs.Request, error) {
			return []*docs.Request{{InsertSectionBreak: &docs.InsertSectionBreakRequest{
				Location: &docs.Location{Index: resolved.Index}, SectionType: sectionType,
			}}}, nil
		},
	})
}

func normalizeDocsSectionType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "next-page", "next_page", "nextpage":
		return "NEXT_PAGE", nil
	case "continuous":
		return "CONTINUOUS", nil
	default:
		return "", usage("--type must be next-page or continuous")
	}
}

func (c *DocsHorizontalRuleCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	return runDocsBodyMutation(ctx, kctx, flags, docsBodyMutation{
		action: "docs.insert-horizontal-rule", docID: c.DocID, batch: c.Batch, placement: c.Placement,
		payload: map[string]any{"kind": "paragraph-border"},
		build: func(resolved docsResolvedPlacement) ([]*docs.Request, error) {
			return []*docs.Request{
				{InsertText: &docs.InsertTextRequest{Location: &docs.Location{Index: resolved.Index}, Text: "\n"}},
				buildHruleBorderRequest(resolved.Index, resolved.Index+1),
			}, nil
		},
	})
}

func (c *DocsSectionColumnsCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	if c.Count < 1 || c.Count > 3 {
		return usage("--count must be between 1 and 3")
	}
	separator, err := normalizeDocsColumnSeparator(c.Separator)
	if err != nil {
		return err
	}
	return runDocsBodyMutation(ctx, kctx, flags, docsBodyMutation{
		action: "docs.section-columns", docID: c.DocID, batch: c.Batch, placement: c.Placement,
		payload: map[string]any{"count": c.Count, "separator": separator},
		build: func(resolved docsResolvedPlacement) ([]*docs.Request, error) {
			columns := make([]*docs.SectionColumnProperties, c.Count)
			for index := range columns {
				columns[index] = &docs.SectionColumnProperties{PaddingEnd: &docs.Dimension{Magnitude: 36, Unit: "PT"}}
			}
			return []*docs.Request{{UpdateSectionStyle: &docs.UpdateSectionStyleRequest{
				Range:        &docs.Range{StartIndex: resolved.Index, EndIndex: resolved.Index + 1},
				SectionStyle: &docs.SectionStyle{ColumnProperties: columns, ColumnSeparatorStyle: separator},
				Fields:       "columnProperties,columnSeparatorStyle",
			}}}, nil
		},
	})
}

func normalizeDocsColumnSeparator(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "":
		return "NONE", nil
	case "between", "between-each-column", "between_each_column":
		return "BETWEEN_EACH_COLUMN", nil
	default:
		return "", usage("--separator must be none or between")
	}
}

func (c *DocsFootnoteCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if docID == "" {
		return usage("empty docId")
	}
	text, provided, err := resolveTextInput(ctx, c.Text, c.File, kctx)
	if err != nil {
		return err
	}
	if !provided || text == "" {
		return usage("required: non-empty --text or --file")
	}
	placement, err := c.Placement.plan(kctx)
	if err != nil {
		return err
	}
	payload := c.Placement.dryRunPayload(placement)
	payload["documentId"] = docID
	payload["textBytes"] = len(text)
	if dryRunErr := dryRunExit(ctx, flags, "docs.insert-footnote", payload); dryRunErr != nil {
		return dryRunErr
	}
	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	resolved, err := resolveDocsPlacement(ctx, svc, docID, c.Placement.Tab, placement)
	if err != nil {
		return err
	}
	if resolved.InTable {
		return usage("docs.insert-footnote cannot target text inside a table")
	}
	createResponse, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{CreateFootnote: &docs.CreateFootnoteRequest{
			Location: &docs.Location{Index: resolved.Index, TabId: resolved.TabID},
		}}},
		WriteControl: docsRequiredRevisionWriteControl(resolved.RequiredRevisionID),
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create footnote: %w", err)
	}
	footnoteID := docsCreatedFootnoteID(createResponse)
	if footnoteID == "" {
		return fmt.Errorf("create footnote response missing footnote ID")
	}
	_, err = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{Requests: []*docs.Request{{
		InsertText: &docs.InsertTextRequest{
			EndOfSegmentLocation: &docs.EndOfSegmentLocation{SegmentId: footnoteID, TabId: resolved.TabID},
			Text:                 text,
		},
	}}}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("footnote %s was created but could not be populated: %w", footnoteID, err)
	}
	result := map[string]any{
		"documentId": docID, "footnoteId": footnoteID, "segmentId": footnoteID,
		"segmentType": docsSegmentKindFootnote, "atIndex": resolved.Index, "requests": 2,
	}
	if resolved.TabID != "" {
		result["tabId"] = resolved.TabID
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("footnoteId\t%s", footnoteID)
	u.Out().Linef("atIndex\t%d", resolved.Index)
	if resolved.TabID != "" {
		u.Out().Linef("tabId\t%s", resolved.TabID)
	}
	return nil
}

func docsCreatedFootnoteID(response *docs.BatchUpdateDocumentResponse) string {
	if response == nil {
		return ""
	}
	for _, reply := range response.Replies {
		if reply != nil && reply.CreateFootnote != nil && reply.CreateFootnote.FootnoteId != "" {
			return reply.CreateFootnote.FootnoteId
		}
	}
	return ""
}
