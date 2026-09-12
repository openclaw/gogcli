package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/docsedit"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

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
