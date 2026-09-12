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

type DocsUpdateCmd struct {
	DocID        string `arg:"" name:"docId" help:"Doc ID"`
	Text         string `name:"text" help:"Text to insert"`
	File         string `name:"file" help:"Text file path ('-' for stdin)"`
	Index        int64  `name:"index" help:"Insert index (default: end of document)"`
	ReplaceRange string `name:"replace-range" help:"Replace UTF-16 Docs API range START:END instead of inserting"`
	At           string `name:"at" help:"Anchor by literal text and replace that matched range"`
	Occurrence   *int   `name:"occurrence" help:"Use the Nth --at match (1-based; required when --at is ambiguous)"`
	MatchCase    bool   `name:"match-case" help:"Use case-sensitive --at matching"`
	Markdown     bool   `name:"markdown" help:"Convert markdown to Google Docs formatting"`
	Pageless     bool   `name:"pageless" help:"Set document to pageless mode"`
	Tab          string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	Segment      string `name:"segment" help:"Target an exact header, footer, or footnote segment ID"`
	TabID        string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
	Batch        string `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
}

func (c *DocsUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	text, provided, err := resolveTextInput(ctx, c.Text, c.File, kctx)
	if err != nil {
		return err
	}
	if !provided {
		return usage("required: --text or --file")
	}
	if text == "" {
		return usage("empty text")
	}
	placement, err := docsedit.PlanUpdatePlacement(docsedit.UpdatePlacementOptions{
		Index:         c.Index,
		IndexProvided: flagProvided(kctx, "index"),
		AllowZero:     strings.TrimSpace(c.Segment) != "",
		ReplaceRange:  c.ReplaceRange,
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
	replacing := placement.Kind == docsedit.PlacementRange
	replaceRange := placement.Range
	replaceStart, replaceEnd := replaceRange.Start, replaceRange.End

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab
	if strings.TrimSpace(c.Segment) != "" && c.Markdown {
		return usage("--segment supports plain-text updates only; markdown can contain structures that are invalid in segments")
	}
	if c.Batch != "" && (c.Markdown || c.Pageless) {
		return usage("--batch supports plain text updates without --pageless")
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.update", c.dryRunPayload(id, len(text), replacing, replaceStart, replaceEnd, at)); dryRunErr != nil {
		return dryRunErr
	}
	if batchErr := validateDocsBatchTarget(ctx, flags, c.Batch, id); batchErr != nil {
		return batchErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	batchRevision, err := captureDocsBatchRevision(ctx, svc, c.Batch, id)
	if err != nil {
		return err
	}

	resolvedPlacement, err := resolveDocsPlacementTarget(ctx, svc, id, c.Tab, c.Segment, placement)
	if err != nil {
		return err
	}
	insertIndex := resolvedPlacement.Index
	c.Tab = resolvedPlacement.TabID
	c.Segment = resolvedPlacement.SegmentID
	if resolvedPlacement.Range != nil {
		replaceStart = resolvedPlacement.Range.Start
		replaceEnd = resolvedPlacement.Range.End
		replacing = true
	}

	requestCount := 0
	written := len(text)
	var resp *docs.BatchUpdateDocumentResponse

	if c.Markdown {
		var inserted int
		markdown := prepareMarkdown(text)
		explicitHeadingAnchors := docsmarkdown.ExplicitHeadingAnchors(markdown.cleaned)
		if replacing {
			loadedDoc := resolvedPlacement.Document
			if loadedDoc == nil {
				loaded, loadErr := loadDocsTargetDocument(ctx, svc, id, c.Tab)
				if loadErr != nil {
					return loadErr
				}
				c.Tab = loaded.tabID
				loadedDoc = loaded.full
			}
			replacedRequests, replacedText, replaceErr := replacePreparedDocsMarkdownRange(
				ctx,
				svc,
				loadedDoc,
				replaceStart,
				replaceEnd,
				markdown,
				c.Tab,
			)
			if replaceErr != nil {
				err = replaceErr
			} else {
				inserted = replacedText
				requestCount = replacedRequests
			}
		} else {
			var insertResult docsMarkdownInsertResult
			insertResult, err = insertPreparedDocsMarkdownAt(ctx, svc, id, insertIndex, markdown, c.Tab, true)
			requestCount = insertResult.RequestCount
			inserted = insertResult.Inserted
			if err == nil && markdownMayContainHeadingLinks(markdown.cleaned) {
				var rewritten int
				rewritten, err = rewriteMarkdownHeadingLinksInRange(
					ctx,
					svc,
					id,
					c.Tab,
					explicitHeadingAnchors,
					insertResult.ContentStart,
					insertResult.ContentEnd,
				)
				requestCount += rewritten
			}
		}
		if err != nil {
			if isDocsNotFound(err) {
				return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
			}
			return err
		}
		written = inserted
		resp = &docs.BatchUpdateDocumentResponse{DocumentId: id}
	} else {
		var targetRange *docsedit.Range
		if replacing {
			targetRange = &docsedit.Range{Start: replaceStart, End: replaceEnd}
		}
		reqs := docsedit.BuildUpdateRequests(text, insertIndex, c.Tab, targetRange)
		applyDocsRequestTarget(reqs, docsTargetFromPlacement(resolvedPlacement.ResolvedPlacement))
		requestCount = len(reqs)
		batchReq := &docs.BatchUpdateDocumentRequest{Requests: reqs}
		batchReq.WriteControl = docsRequiredRevisionWriteControl(resolvedPlacement.RequiredRevisionID)
		if queued, queueErr := queueDocsBatchRequests(ctx, flags, c.Batch, id, "docs.update", batchRevision, reqs, false); queued || queueErr != nil {
			return queueErr
		}
		resp, err = svc.Documents.BatchUpdate(id, batchReq).Context(ctx).Do()
		if err != nil {
			if isDocsNotFound(err) {
				return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
			}
			return err
		}
	}
	if c.Pageless {
		if err := setDocumentPageless(ctx, svc, id); err != nil {
			return fmt.Errorf("set pageless mode: %w", err)
		}
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": resp.DocumentId,
			"requests":   requestCount,
			"index":      insertIndex,
		}
		if replacing {
			payload["replaced"] = true
			payload["replaceRange"] = map[string]int64{"start": replaceStart, "end": replaceEnd}
		}
		if c.Markdown {
			payload["written"] = written
			payload["markdown"] = true
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		if c.Segment != "" {
			payload["segmentId"] = c.Segment
			payload["segmentType"] = resolvedPlacement.SegmentKind
		}
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("id\t%s", resp.DocumentId)
	u.Out().Linef("requests\t%d", requestCount)
	u.Out().Linef("index\t%d", insertIndex)
	if replacing {
		u.Out().Linef("replaced\ttrue")
		u.Out().Linef("range\t%d:%d", replaceStart, replaceEnd)
	}
	if c.Markdown {
		u.Out().Linef("written\t%d", written)
		u.Out().Linef("markdown\ttrue")
	}
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	if c.Segment != "" {
		u.Out().Linef("segmentId\t%s", c.Segment)
		u.Out().Linef("segmentType\t%s", resolvedPlacement.SegmentKind)
	}
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Linef("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

func (c *DocsUpdateCmd) dryRunPayload(docID string, written int, replacing bool, replaceStart, replaceEnd int64, at string) map[string]any {
	var index any = docsAtIndexEnd
	switch {
	case replacing:
		index = replaceStart
	case at != "":
		index = docsAtIndexAnchorStart
	case c.Index > 0:
		index = c.Index
	}
	payload := map[string]any{
		"document_id": docID,
		"written":     written,
		"index":       index,
		"markdown":    c.Markdown,
		"pageless":    c.Pageless,
		"tab":         c.Tab,
		"segment":     c.Segment,
		"batch":       c.Batch,
	}
	if replacing {
		payload["replaceRange"] = map[string]int64{"start": replaceStart, "end": replaceEnd}
	}
	addDocsAtAnchorDryRunPayload(payload, docsAtAnchorFlags{At: at, Occurrence: c.Occurrence, MatchCase: c.MatchCase})
	return payload
}
