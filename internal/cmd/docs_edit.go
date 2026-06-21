package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/docsedit"
	"github.com/steipete/gogcli/internal/docsmarkdown"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// resolveTabArg returns the effective tab value from --tab or the deprecated
// --tab-id flag. It rejects supplying both and emits a deprecation warning
// when --tab-id is used.
func resolveTabArg(ctx context.Context, tab, tabID string) (string, error) {
	tab = strings.TrimSpace(tab)
	tabID = strings.TrimSpace(tabID)
	if tab != "" && tabID != "" {
		return "", usage("--tab and --tab-id are mutually exclusive (--tab-id is deprecated; use --tab)")
	}
	if tabID != "" {
		u := ui.FromContext(ctx)
		u.Err().Linef("Warning: --tab-id is deprecated; use --tab instead")
		return tabID, nil
	}
	return tab, nil
}

type DocsWriteCmd struct {
	DocID        string          `arg:"" name:"docId" help:"Doc ID"`
	Text         string          `name:"text" help:"Text to write"`
	File         string          `name:"file" help:"Text file path ('-' for stdin)"`
	Replace      bool            `name:"replace" help:"Replace all content explicitly (required with --markdown unless --append is set)"`
	Markdown     bool            `name:"markdown" help:"Convert markdown to Google Docs formatting (requires --replace or --append)"`
	Append       bool            `name:"append" help:"Append instead of replacing the document body"`
	CheckOrphans bool            `name:"check-orphans" help:"Block markdown replacement when open comment quotes would disappear"`
	Pageless     bool            `name:"pageless" help:"Set document to pageless mode"`
	Layout       DocsLayoutFlags `embed:""`
	Tab          string          `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID        string          `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
	Batch        string          `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
	Format       DocsFormatFlags `embed:""`
}

func (c *DocsWriteCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	text, err := c.resolveWriteText(ctx, kctx)
	if err != nil {
		return err
	}
	if c.Append && c.Replace {
		return usage("--append cannot be combined with --replace")
	}
	if c.CheckOrphans && (!c.Markdown || !c.Replace || c.Append) {
		return usage("--check-orphans requires --replace --markdown")
	}

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab

	if err := c.validateDocumentStyle(); err != nil {
		return err
	}
	if c.Batch != "" && (c.Markdown || c.Pageless || c.Layout.any()) {
		return usage("--batch supports plain text writes without --pageless or layout flags")
	}

	if c.Markdown {
		if c.Format.any() {
			return usage("formatting flags are only supported for plain-text docs write; use markdown syntax or run docs format after writing")
		}
		return c.writeMarkdown(ctx, flags, id, text)
	}

	return c.writePlainText(ctx, flags, id, text)
}

func (c *DocsWriteCmd) validateDocumentStyle() error {
	if !c.Pageless && !c.Layout.any() {
		return nil
	}
	mode := ""
	if c.Pageless {
		mode = docsDocumentModePageless
	}
	_, err := buildUpdateDocumentStyleRequest(docsDocumentStyleOptions{
		Mode:            mode,
		DocsLayoutFlags: c.Layout,
	})
	return err
}

func (c *DocsWriteCmd) resolveWriteText(ctx context.Context, kctx *kong.Context) (string, error) {
	text, provided, err := resolveTextInput(ctx, c.Text, c.File, kctx)
	if err != nil {
		return "", err
	}
	if !provided {
		return "", usage("required: --text or --file")
	}
	if text == "" {
		return "", usage("empty text")
	}
	return text, nil
}

func (c *DocsWriteCmd) writePlainText(ctx context.Context, flags *RootFlags, docID, text string) error {
	if c.Append && c.Format.createsBullets() && c.Format.hasParagraphStyle() {
		return usage("docs write --append cannot combine bullet creation with paragraph formatting; append first, then use docs format")
	}
	if c.Format.any() {
		if _, err := c.Format.buildRequests(1, 1+utf16Len(text), c.Tab); err != nil {
			return err
		}
	}

	dryRunPayload := map[string]any{
		"document_id": docID,
		"written":     len(text),
		"append":      c.Append,
		"replace":     !c.Append,
		"markdown":    false,
		"pageless":    c.Pageless,
		"tab":         c.Tab,
		"batch":       c.Batch,
	}
	for k, v := range c.Layout.dryRunPayload() {
		dryRunPayload[k] = v
	}
	if err := dryRunExit(ctx, flags, "docs.write", dryRunPayload); err != nil {
		return err
	}
	if err := validateDocsBatchTarget(ctx, flags, c.Batch, docID); err != nil {
		return err
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	batchRevision, err := captureDocsBatchRevision(ctx, svc, c.Batch, docID)
	if err != nil {
		return err
	}

	endIndex, tabID, err := docsTargetEndIndexAndTabID(ctx, svc, docID, c.Tab)
	if err != nil {
		return err
	}
	c.Tab = tabID
	insertIndex := int64(1)
	if c.Append {
		insertIndex = docsedit.AppendIndex(endIndex)
	}

	reqs, err := docsedit.BuildWriteRequests(docsedit.WriteOptions{
		EndIndex:    endIndex,
		InsertIndex: insertIndex,
		Text:        text,
		TabID:       c.Tab,
		Append:      c.Append,
		Format:      c.Format.options(),
	})
	if err != nil {
		return usage(err.Error())
	}
	if queued, queueErr := queueDocsBatchRequests(ctx, flags, c.Batch, docID, "docs.write", batchRevision, reqs, !c.Append); queued || queueErr != nil {
		return queueErr
	}
	resp, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{Requests: reqs}).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}
	if err := c.applyDocumentStyle(ctx, svc, docID); err != nil {
		return err
	}

	return c.writePlainTextResult(ctx, resp, len(reqs), insertIndex)
}

func (c *DocsWriteCmd) applyDocumentStyle(ctx context.Context, svc *docs.Service, docID string) error {
	if !c.Pageless && !c.Layout.any() {
		return nil
	}
	mode := ""
	if c.Pageless {
		mode = docsDocumentModePageless
	}
	if err := setDocumentStyle(ctx, svc, docID, docsDocumentStyleOptions{
		Mode:            mode,
		DocsLayoutFlags: c.Layout,
	}); err != nil {
		return fmt.Errorf("set document style: %w", err)
	}
	return nil
}

func (c *DocsWriteCmd) writePlainTextResult(ctx context.Context, resp *docs.BatchUpdateDocumentResponse, requestCount int, insertIndex int64) error {
	u := ui.FromContext(ctx)
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": resp.DocumentId,
			"requests":   requestCount,
			"append":     c.Append,
			"index":      insertIndex,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		for k, v := range c.Layout.dryRunPayload() {
			payload[k] = v
		}
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("id\t%s", resp.DocumentId)
	u.Out().Linef("requests\t%d", requestCount)
	u.Out().Linef("append\t%t", c.Append)
	u.Out().Linef("index\t%d", insertIndex)
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Linef("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

func (c *DocsWriteCmd) writeMarkdown(ctx context.Context, flags *RootFlags, docID, content string) error {
	markdown := prepareMarkdown(content)
	plan, err := docsedit.PlanMarkdownWrite(docsedit.MarkdownWriteOptions{
		Markdown:           markdown.cleaned,
		ImageCount:         len(markdown.images),
		Append:             c.Append,
		Replace:            c.Replace,
		Tab:                c.Tab,
		CheckOrphans:       c.CheckOrphans,
		ApplyDocumentStyle: c.Pageless || c.Layout.any(),
	})
	if err != nil {
		return usage(err.Error())
	}

	switch plan.Mode {
	case docsedit.MarkdownWriteDriveReplace:
		return c.replaceMarkdownWithDrive(ctx, flags, docID, markdown, plan)
	case docsedit.MarkdownWriteLocalAppend:
		return c.appendMarkdown(ctx, flags, docID, markdown, plan)
	case docsedit.MarkdownWriteLocalReplace:
		return c.replaceMarkdownLocally(ctx, flags, docID, markdown, plan)
	default:
		return fmt.Errorf("unsupported markdown write mode: %d", plan.Mode)
	}
}

func (c *DocsWriteCmd) replaceMarkdownWithDrive(
	ctx context.Context,
	flags *RootFlags,
	docID string,
	markdown preparedMarkdown,
	plan docsedit.MarkdownWritePlan,
) error {
	u := ui.FromContext(ctx)
	dryRunPayload := map[string]any{
		"document_id":   docID,
		"written":       len(markdown.source),
		"append":        false,
		"replace":       true,
		"markdown":      true,
		"pageless":      c.Pageless,
		"images":        plan.ImageCount,
		"check_orphans": plan.CheckOrphans,
	}
	for k, v := range c.Layout.dryRunPayload() {
		dryRunPayload[k] = v
	}
	if err := dryRunExit(ctx, flags, "docs.write", dryRunPayload); err != nil {
		return err
	}

	account, driveSvc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	var docsSvc *docs.Service
	if plan.CheckOrphans {
		docsSvc, err = docsService(ctx, account)
		if err != nil {
			return err
		}
		orphans, tabID, orphanErr := findDocsWriteMarkdownOrphans(
			ctx,
			driveSvc,
			docsSvc,
			docID,
			markdown,
			plan.Tab,
			plan.OrphanScopeWholeDocument,
		)
		if orphanErr != nil {
			return orphanErr
		}
		if resultErr := writeDocsWriteOrphanResult(ctx, docID, tabID, orphans); resultErr != nil {
			return resultErr
		}
	}

	updated, err := driveSvc.Files.Update(docID, &drive.File{}).
		Media(strings.NewReader(plan.Markdown), gapi.ContentType(mimeTextMarkdown)).
		SupportsAllDrives(true).
		Fields("id,name,webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("writing markdown to document: %w", err)
	}

	if plan.RequiresDocumentsService && docsSvc == nil {
		var svcErr error
		docsSvc, svcErr = docsService(ctx, account)
		if svcErr != nil {
			return svcErr
		}
	}
	rewrittenHeadingLinks := 0
	if plan.RewriteHeadingLinks {
		count, rewriteErr := rewriteMarkdownHeadingLinks(ctx, docsSvc, docID, plan.Tab, plan.ExplicitHeadingAnchors)
		if rewriteErr != nil {
			return fmt.Errorf("rewrite heading links: %w", rewriteErr)
		}
		rewrittenHeadingLinks = count
	}
	if plan.InsertImages {
		if err := insertImagesIntoDocs(ctx, docsSvc, docID, markdown.images, plan.Tab); err != nil {
			cleanupDocsImagePlaceholders(ctx, docsSvc, docID, markdown.images, plan.Tab)
			return fmt.Errorf("insert images: %w", err)
		}
	}
	if plan.ApplyDocumentStyle {
		if err := c.applyDocumentStyle(ctx, docsSvc, docID); err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": updated.Id,
			"written":    len(markdown.source),
			"replaced":   true,
			"markdown":   true,
		}
		if c.Pageless {
			payload["pageless"] = true
		}
		if rewrittenHeadingLinks > 0 {
			payload["headingLinks"] = rewrittenHeadingLinks
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("documentId\t%s", updated.Id)
	u.Out().Linef("written\t%d", len(markdown.source))
	u.Out().Linef("mode\treplaced (markdown converted)")
	if c.Pageless {
		u.Out().Linef("pageless\ttrue")
	}
	if rewrittenHeadingLinks > 0 {
		u.Out().Linef("headingLinks\t%d", rewrittenHeadingLinks)
	}
	if updated.WebViewLink != "" {
		u.Out().Linef("link\t%s", updated.WebViewLink)
	}
	return nil
}

func (c *DocsWriteCmd) appendMarkdown(
	ctx context.Context,
	flags *RootFlags,
	docID string,
	markdown preparedMarkdown,
	plan docsedit.MarkdownWritePlan,
) error {
	dryRunPayload := map[string]any{
		"document_id": docID,
		"written":     len(plan.Markdown),
		"append":      true,
		"replace":     false,
		"markdown":    true,
		"pageless":    c.Pageless,
		"tab":         plan.Tab,
		"images":      plan.ImageCount,
	}
	for k, v := range c.Layout.dryRunPayload() {
		dryRunPayload[k] = v
	}
	if err := dryRunExit(ctx, flags, "docs.write", dryRunPayload); err != nil {
		return err
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	endIndex, tabID, err := docsTargetEndIndexAndTabID(ctx, svc, docID, c.Tab)
	if err != nil {
		return err
	}
	c.Tab = tabID
	insertIndex := docsedit.AppendIndex(endIndex)
	insertResult, err := insertPreparedDocsMarkdownAt(ctx, svc, docID, insertIndex, markdown, c.Tab, true)
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}
	if plan.ApplyDocumentStyle {
		if err := c.applyDocumentStyle(ctx, svc, docID); err != nil {
			return err
		}
	}
	rewrittenHeadingLinks := 0
	if plan.RewriteHeadingLinks {
		count, rewriteErr := rewriteMarkdownHeadingLinksFromIndex(
			ctx,
			svc,
			docID,
			c.Tab,
			plan.ExplicitHeadingAnchors,
			insertResult.ContentStart,
		)
		if rewriteErr != nil {
			return fmt.Errorf("rewrite heading links: %w", rewriteErr)
		}
		rewrittenHeadingLinks = count
		insertResult.RequestCount += count
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
			"written":    insertResult.Inserted,
			"requests":   insertResult.RequestCount,
			"append":     true,
			"index":      insertIndex,
			"markdown":   true,
		}
		if c.Pageless {
			payload["pageless"] = true
		}
		if rewrittenHeadingLinks > 0 {
			payload["headingLinks"] = rewrittenHeadingLinks
		}
		for k, v := range c.Layout.dryRunPayload() {
			payload[k] = v
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("written\t%d", insertResult.Inserted)
	u.Out().Linef("requests\t%d", insertResult.RequestCount)
	u.Out().Linef("mode\tappended (markdown converted)")
	u.Out().Linef("index\t%d", insertIndex)
	if rewrittenHeadingLinks > 0 {
		u.Out().Linef("headingLinks\t%d", rewrittenHeadingLinks)
	}
	if c.Pageless {
		u.Out().Linef("pageless\ttrue")
	}
	return nil
}

// replaceMarkdownLocally renders Markdown through Docs batchUpdate after
// clearing the selected body. This preserves tab targeting and table-cell
// line breaks that Drive's whole-document converter cannot represent.
func (c *DocsWriteCmd) replaceMarkdownLocally(
	ctx context.Context,
	flags *RootFlags,
	docID string,
	markdown preparedMarkdown,
	plan docsedit.MarkdownWritePlan,
) error {
	dryRunPayload := map[string]any{
		"document_id":   docID,
		"written":       len(plan.Markdown),
		"append":        false,
		"replace":       true,
		"markdown":      true,
		"pageless":      c.Pageless,
		"tab":           plan.Tab,
		"images":        plan.ImageCount,
		"check_orphans": plan.CheckOrphans,
	}
	for k, v := range c.Layout.dryRunPayload() {
		dryRunPayload[k] = v
	}
	if err := dryRunExit(ctx, flags, "docs.write", dryRunPayload); err != nil {
		return err
	}

	var svc *docs.Service
	var err error
	if plan.CheckOrphans {
		account, driveSvc, driveErr := requireDriveService(ctx, flags)
		if driveErr != nil {
			return driveErr
		}
		svc, err = docsService(ctx, account)
		if err != nil {
			return err
		}
		orphans, resolvedTabID, orphanErr := findDocsWriteMarkdownOrphans(
			ctx,
			driveSvc,
			svc,
			docID,
			markdown,
			plan.Tab,
			plan.OrphanScopeWholeDocument,
		)
		if orphanErr != nil {
			return orphanErr
		}
		if resultErr := writeDocsWriteOrphanResult(ctx, docID, resolvedTabID, orphans); resultErr != nil {
			return resultErr
		}
		c.Tab = resolvedTabID
	} else {
		svc, err = requireDocsService(ctx, flags)
	}
	if err != nil {
		return err
	}

	endIndex, tabID, err := docsTargetEndIndexAndTabID(ctx, svc, docID, c.Tab)
	if err != nil {
		return err
	}
	c.Tab = tabID

	// Wipe existing tab body (everything between the implicit start index 1
	// and the last segment endIndex - 1). Skipped when the tab is already
	// empty (endIndex <= 2 means a single newline segment).
	deleteEnd := endIndex - 1
	if deleteEnd > 1 {
		if _, derr := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
			Requests: []*docs.Request{{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{StartIndex: 1, EndIndex: deleteEnd, TabId: tabID},
				},
			}},
		}).Context(ctx).Do(); derr != nil {
			if isDocsNotFound(derr) {
				return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
			}
			return fmt.Errorf("clear tab content: %w", derr)
		}
	}

	insertResult, err := insertPreparedDocsMarkdownAt(ctx, svc, docID, 1, markdown, tabID, true)
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}
	if plan.ApplyDocumentStyle {
		if err := c.applyDocumentStyle(ctx, svc, docID); err != nil {
			return err
		}
	}
	rewrittenHeadingLinks := 0
	if plan.RewriteHeadingLinks {
		count, rewriteErr := rewriteMarkdownHeadingLinks(ctx, svc, docID, tabID, plan.ExplicitHeadingAnchors)
		if rewriteErr != nil {
			return fmt.Errorf("rewrite heading links: %w", rewriteErr)
		}
		rewrittenHeadingLinks = count
		insertResult.RequestCount += count
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
			"written":    insertResult.Inserted,
			"requests":   insertResult.RequestCount,
			"replaced":   true,
			"markdown":   true,
			"tabId":      tabID,
		}
		if c.Pageless {
			payload["pageless"] = true
		}
		if rewrittenHeadingLinks > 0 {
			payload["headingLinks"] = rewrittenHeadingLinks
		}
		for k, v := range c.Layout.dryRunPayload() {
			payload[k] = v
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("written\t%d", insertResult.Inserted)
	u.Out().Linef("requests\t%d", insertResult.RequestCount)
	u.Out().Linef("mode\treplaced tab (markdown converted)")
	u.Out().Linef("tabId\t%s", tabID)
	if rewrittenHeadingLinks > 0 {
		u.Out().Linef("headingLinks\t%d", rewrittenHeadingLinks)
	}
	if c.Pageless {
		u.Out().Linef("pageless\ttrue")
	}
	return nil
}

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

type DocsDeleteCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Start      *int64 `name:"start" help:"Start index (>= 1; required unless --at is set)"`
	End        *int64 `name:"end" help:"End index (> start; required unless --at is set)"`
	At         string `name:"at" help:"Anchor by literal text and delete that matched range"`
	Occurrence *int   `name:"occurrence" help:"Use the Nth --at match (1-based; required when --at is ambiguous)"`
	MatchCase  bool   `name:"match-case" help:"Use case-sensitive --at matching"`
	Tab        string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	Segment    string `name:"segment" help:"Target an exact header, footer, or footnote segment ID"`
	TabID      string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
	Batch      string `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
}

func (c *DocsDeleteCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	placement, err := docsedit.PlanRangePlacement(docsedit.RangePlacementOptions{
		Start:     c.Start,
		End:       c.End,
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
	dryRunPayload := map[string]any{
		"document_id": docID,
		"start_index": docsDeleteDryRunStart(c.Start),
		"end_index":   docsDeleteDryRunEnd(c.End),
		"deleted":     docsDeleteDryRunDeleted(c.Start, c.End, at),
		"tab":         c.Tab,
		"segment":     c.Segment,
		"batch":       c.Batch,
	}
	addDocsAtAnchorDryRunPayload(dryRunPayload, docsAtAnchorFlags{At: at, Occurrence: c.Occurrence, MatchCase: c.MatchCase})
	if dryRunErr := dryRunExit(ctx, flags, "docs.delete", dryRunPayload); dryRunErr != nil {
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
	start := resolvedPlacement.Range.Start
	end := resolvedPlacement.Range.End
	c.Tab = resolvedPlacement.TabID
	c.Segment = resolvedPlacement.SegmentID

	batchReq := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{docsedit.BuildDeleteRequest(docsedit.Range{Start: start, End: end}, c.Tab)},
	}
	applyDocsRequestTarget(batchReq.Requests, docsTargetFromPlacement(resolvedPlacement.ResolvedPlacement))
	batchReq.WriteControl = docsRequiredRevisionWriteControl(resolvedPlacement.RequiredRevisionID)
	if queued, queueErr := queueDocsBatchRequests(ctx, flags, c.Batch, docID, "docs.delete", batchRevision, batchReq.Requests, false); queued || queueErr != nil {
		return queueErr
	}
	result, err := svc.Documents.BatchUpdate(docID, batchReq).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("deleting content: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": result.DocumentId,
			"deleted":    end - start,
			"startIndex": start,
			"endIndex":   end,
		}
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
	u.Out().Linef("deleted\t%d characters", end-start)
	u.Out().Linef("range\t%d-%d", start, end)
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	if c.Segment != "" {
		u.Out().Linef("segmentId\t%s", c.Segment)
		u.Out().Linef("segmentType\t%s", resolvedPlacement.SegmentKind)
	}
	return nil
}

func docsDeleteDryRunStart(start *int64) any {
	if start == nil {
		return nil
	}
	return *start
}

func docsDeleteDryRunEnd(end *int64) any {
	if end == nil {
		return nil
	}
	return *end
}

func docsDeleteDryRunDeleted(start, end *int64, at string) any {
	if at != "" {
		return "at:range"
	}
	if start == nil || end == nil {
		return nil
	}
	return *end - *start
}

type DocsClearCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
}

func (c *DocsClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	if err := dryRunExit(ctx, flags, "docs.clear", map[string]any{
		"document_id": docID,
	}); err != nil {
		return err
	}
	return (&DocsSedCmd{DocID: docID, Expression: `s/^$//`}).Run(ctx, flags)
}

type DocsFindReplaceCmd struct {
	DocID       string `arg:"" name:"docId" help:"Doc ID"`
	Find        string `arg:"" name:"find" help:"Text to find"`
	ReplaceText string `arg:"" optional:"" name:"replace" help:"Replacement text (omit when using --content-file)"`
	ContentFile string `name:"content-file" help:"Read replacement from a file instead of the positional argument."`
	MatchCase   bool   `name:"match-case" help:"Case-sensitive matching"`
	Format      string `name:"format" help:"Replacement format: plain|markdown. Markdown converts formatting, tables, and inline images from public HTTPS URLs." default:"plain" enum:"plain,markdown"`
	First       bool   `name:"first" help:"Replace only the first occurrence instead of all."`
	Tab         string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID       string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

type DocsEditCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Find       string `arg:"" name:"find" help:"Text to find"`
	ReplaceStr string `arg:"" name:"replace" help:"Replacement text"`
	MatchCase  bool   `name:"match-case" help:"Case-sensitive matching"`
}

func (c *DocsEditCmd) Run(ctx context.Context, flags *RootFlags) error {
	return (&DocsFindReplaceCmd{
		DocID:       c.DocID,
		Find:        c.Find,
		ReplaceText: c.ReplaceStr,
		MatchCase:   c.MatchCase,
	}).Run(ctx, flags)
}

func (c *DocsFindReplaceCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	if c.Find == "" {
		return usage("find text cannot be empty")
	}

	replaceText, err := c.resolveReplaceText()
	if err != nil {
		return err
	}

	format := strings.ToLower(strings.TrimSpace(c.Format))
	if format == "" {
		format = docsContentFormatPlain
	}

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab

	if dryRunErr := dryRunExit(ctx, flags, "docs.find-replace", map[string]any{
		"document_id": docID,
		"find":        c.Find,
		"replace":     replaceText,
		"format":      format,
		"first":       c.First,
		"match_case":  c.MatchCase,
		"tab":         c.Tab,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	if c.Tab != "" {
		tabID, tabErr := resolveDocsTabID(ctx, svc, docID, c.Tab)
		if tabErr != nil {
			return tabErr
		}
		c.Tab = tabID
	}

	if flags != nil && flags.DryRun {
		return c.runDryRun(ctx, u, svc, docID, replaceText, format)
	}

	if !c.First && format == docsContentFormatPlain {
		return c.runReplaceAll(ctx, u, svc, docID, replaceText)
	}

	loaded, err := loadDocsTargetDocument(ctx, svc, docID, c.Tab)
	if err != nil {
		return err
	}
	c.Tab = loaded.tabID
	doc := loaded.full
	targetDoc := loaded.target

	if c.First {
		matches := docsedit.FindTextRanges(targetDoc, c.Find, docsedit.SearchOptions{
			MatchCase:            c.MatchCase,
			PreserveHTMLEntities: true,
			RequireTextSegment:   true,
		})
		if len(matches) == 0 {
			return c.printFirstResult(ctx, u, docID, replaceText, 0, 0)
		}
		match := matches[0]
		if format == docsContentFormatMarkdown {
			err = c.runMarkdown(ctx, svc, doc, match.StartIndex, match.EndIndex, replaceText)
		} else {
			err = c.runPlain(ctx, svc, doc, match.StartIndex, match.EndIndex, replaceText)
		}
		if err != nil {
			return err
		}
		return c.printFirstResult(ctx, u, docID, replaceText, 1, len(matches))
	}

	matches := docsedit.FindTextRanges(targetDoc, c.Find, docsedit.SearchOptions{
		MatchCase:            c.MatchCase,
		PreserveHTMLEntities: true,
		RequireTextSegment:   true,
	})
	for i := len(matches) - 1; i >= 0; i-- {
		if err = c.runMarkdown(ctx, svc, doc, matches[i].StartIndex, matches[i].EndIndex, replaceText); err != nil {
			return err
		}
		if i == 0 {
			continue
		}
		loaded, err = loadDocsTargetDocument(ctx, svc, docID, c.Tab)
		if err != nil {
			return fmt.Errorf("re-reading document: %w", err)
		}
		doc = loaded.full
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId":   docID,
			"find":         c.Find,
			"replace":      replaceText,
			"replacements": len(matches),
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("find\t%s", c.Find)
	u.Out().Linef("replace\t%s", replaceText)
	u.Out().Linef("replacements\t%d", len(matches))
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	return nil
}

func (c *DocsFindReplaceCmd) runDryRun(ctx context.Context, u *ui.UI, svc *docs.Service, docID, replaceText, format string) error {
	loaded, err := loadDocsTargetDocument(ctx, svc, docID, c.Tab)
	if err != nil {
		return err
	}
	c.Tab = loaded.tabID

	matches := docsedit.FindTextRanges(loaded.target, c.Find, docsedit.SearchOptions{
		MatchCase:            c.MatchCase,
		PreserveHTMLEntities: true,
		RequireTextSegment:   true,
	})
	replacements := len(matches)
	if c.First && replacements > 1 {
		replacements = 1
	}
	remaining := len(matches) - replacements

	payload := map[string]any{
		"documentId":   docID,
		"find":         c.Find,
		"replace":      replaceText,
		"format":       format,
		"first":        c.First,
		"replacements": replacements,
		"remaining":    remaining,
	}
	if c.Tab != "" {
		payload["tabId"] = c.Tab
	}
	if err := dryRunExit(ctx, &RootFlags{DryRun: true}, "docs.find-replace", payload); err != nil {
		return err
	}
	if !outfmt.IsJSON(ctx) {
		u.Out().Linef("matches\t%d", len(matches))
	}
	return nil
}

func (c *DocsFindReplaceCmd) runReplaceAll(ctx context.Context, u *ui.UI, svc *docs.Service, docID, replaceText string) error {
	documentID, replacements, err := runDocsReplaceAll(ctx, svc, docID, c.Find, replaceText, c.MatchCase, c.Tab)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId":   documentID,
			"find":         c.Find,
			"replace":      replaceText,
			"replacements": replacements,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("documentId\t%s", documentID)
	u.Out().Linef("find\t%s", c.Find)
	u.Out().Linef("replace\t%s", replaceText)
	u.Out().Linef("replacements\t%d", replacements)
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	return nil
}

func (c *DocsFindReplaceCmd) runPlain(ctx context.Context, svc *docs.Service, doc *docs.Document, startIdx, endIdx int64, replaceText string) error {
	return replaceDocsTextRange(ctx, svc, doc, startIdx, endIdx, replaceText, c.Tab)
}

func (c *DocsFindReplaceCmd) runMarkdown(ctx context.Context, svc *docs.Service, doc *docs.Document, startIdx, endIdx int64, replaceText string) error {
	_, _, err := replaceDocsMarkdownRange(ctx, svc, doc, startIdx, endIdx, replaceText, c.Tab)
	return err
}

func (c *DocsFindReplaceCmd) printFirstResult(ctx context.Context, u *ui.UI, docID, replaceText string, replacements, total int) error {
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId":   docID,
			"find":         c.Find,
			"replacements": replacements,
			"remaining":    total - replacements,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("find\t%s", c.Find)
	u.Out().Linef("replace\t%s", replaceText)
	u.Out().Linef("replacements\t%d", replacements)
	if remaining := total - replacements; remaining > 0 {
		u.Out().Linef("remaining\t%d", remaining)
	}
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	return nil
}

func (c *DocsFindReplaceCmd) resolveReplaceText() (string, error) {
	if c.ContentFile != "" && c.ReplaceText != "" {
		return "", usage("cannot use both replace argument and --content-file")
	}
	if c.ContentFile == "" {
		return c.ReplaceText, nil
	}
	data, err := os.ReadFile(c.ContentFile)
	if err != nil {
		return "", fmt.Errorf("read content file: %w", err)
	}
	return string(data), nil
}
