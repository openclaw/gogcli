package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

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
	DocID    string          `arg:"" name:"docId" help:"Doc ID"`
	Text     string          `name:"text" help:"Text to write"`
	File     string          `name:"file" help:"Text file path ('-' for stdin)"`
	Replace  bool            `name:"replace" help:"Replace all content explicitly (required with --markdown unless --append is set)"`
	Markdown bool            `name:"markdown" help:"Convert markdown to Google Docs formatting (requires --replace or --append)"`
	Append   bool            `name:"append" help:"Append instead of replacing the document body"`
	Pageless bool            `name:"pageless" help:"Set document to pageless mode"`
	Layout   DocsLayoutFlags `embed:""`
	Tab      string          `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID    string          `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
	Format   DocsFormatFlags `embed:""`
}

func (c *DocsWriteCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	text, err := c.resolveWriteText(kctx)
	if err != nil {
		return err
	}
	if c.Append && c.Replace {
		return usage("--append cannot be combined with --replace")
	}

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab

	if err := c.validateDocumentStyle(); err != nil {
		return err
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

func (c *DocsWriteCmd) resolveWriteText(kctx *kong.Context) (string, error) {
	text, provided, err := resolveTextInput(c.Text, c.File, kctx, "text", "file")
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
	insertIndex := int64(1)
	if c.Append {
		insertIndex = docsAppendIndex(endIndex)
	}

	reqs, err := c.buildPlainWriteRequests(endIndex, insertIndex, text)
	if err != nil {
		return err
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

func (c *DocsWriteCmd) buildPlainWriteRequests(endIndex, insertIndex int64, text string) ([]*docs.Request, error) {
	reqs := make([]*docs.Request, 0, 2)
	if !c.Append {
		deleteEnd := endIndex - 1
		if deleteEnd > 1 {
			reqs = append(reqs, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{StartIndex: 1, EndIndex: deleteEnd, TabId: c.Tab},
				},
			})
		}
	}
	reqs = append(reqs, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: insertIndex, TabId: c.Tab},
			Text:     text,
		},
	})
	if c.Format.any() {
		formatReqs, err := c.Format.buildRequests(insertIndex, insertIndex+utf16Len(text), c.Tab)
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, formatReqs...)
	}
	return reqs, nil
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
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
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
	u := ui.FromContext(ctx)

	if c.Append {
		return c.appendMarkdown(ctx, flags, docID, content)
	}
	if !c.Replace {
		return usage("--markdown requires --replace or --append")
	}
	// Drive's markdown converter operates on entire documents, so we cannot use
	// the Drive Files.Update path when --tab is set. Instead, render markdown
	// locally and apply it to the specified tab via Docs batchUpdate.
	if c.Tab != "" {
		return c.replaceMarkdownInTab(ctx, flags, docID, content)
	}

	cleaned, images := extractMarkdownImages(content)
	if markdownHasTableCellBreaks(cleaned) {
		return c.replaceMarkdownInTab(ctx, flags, docID, content)
	}
	cleaned = normalizeMarkdownTablesForDriveImport(cleaned)
	explicitHeadingAnchors := markdownImportExplicitHeadingAnchors(cleaned)
	cleaned = stripMarkdownHeadingAnchors(cleaned)
	dryRunPayload := map[string]any{
		"document_id": docID,
		"written":     len(content),
		"append":      false,
		"replace":     true,
		"markdown":    true,
		"pageless":    c.Pageless,
		"images":      len(images),
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

	updated, err := driveSvc.Files.Update(docID, &drive.File{}).
		Media(strings.NewReader(cleaned), gapi.ContentType(mimeTextMarkdown)).
		SupportsAllDrives(true).
		Fields("id,name,webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("writing markdown to document: %w", err)
	}

	var docsSvc *docs.Service
	needsDocsSvc := len(images) > 0 || c.Pageless || c.Layout.any() || markdownMayContainHeadingLinks(cleaned)
	if needsDocsSvc {
		var svcErr error
		docsSvc, svcErr = newDocsService(ctx, account)
		if svcErr != nil {
			return svcErr
		}
	}
	rewrittenHeadingLinks := 0
	if markdownMayContainHeadingLinks(cleaned) {
		count, rewriteErr := rewriteMarkdownHeadingLinks(ctx, docsSvc, docID, "", explicitHeadingAnchors)
		if rewriteErr != nil {
			return fmt.Errorf("rewrite heading links: %w", rewriteErr)
		}
		rewrittenHeadingLinks = count
	}
	if len(images) > 0 {
		if err := insertImagesIntoDocs(ctx, docsSvc, docID, images, ""); err != nil {
			cleanupDocsImagePlaceholders(ctx, docsSvc, docID, images, "")
			return fmt.Errorf("insert images: %w", err)
		}
	}
	if c.Pageless || c.Layout.any() {
		if err := c.applyDocumentStyle(ctx, docsSvc, docID); err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": updated.Id,
			"written":    len(content),
			"replaced":   true,
			"markdown":   true,
		}
		if c.Pageless {
			payload["pageless"] = true
		}
		if rewrittenHeadingLinks > 0 {
			payload["headingLinks"] = rewrittenHeadingLinks
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Linef("documentId\t%s", updated.Id)
	u.Out().Linef("written\t%d", len(content))
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

func (c *DocsWriteCmd) appendMarkdown(ctx context.Context, flags *RootFlags, docID, content string) error {
	cleaned, images := extractMarkdownImages(content)
	explicitHeadingAnchors := markdownExplicitHeadingAnchors(cleaned)
	dryRunPayload := map[string]any{
		"document_id": docID,
		"written":     len(cleaned),
		"append":      true,
		"replace":     false,
		"markdown":    true,
		"pageless":    c.Pageless,
		"tab":         c.Tab,
		"images":      len(images),
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
	insertIndex := docsAppendIndex(endIndex)
	insertedMarkdownStart := insertIndex
	appendElements := ParseMarkdown(cleaned)
	if insertIndex > 1 && markdownAppendNeedsParagraphBoundary(appendElements) {
		insertedMarkdownStart++
	}

	requestCount, inserted, err := insertDocsMarkdownAtWithOptions(ctx, svc, docID, insertIndex, content, c.Tab, true)
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}
	if err := c.applyDocumentStyle(ctx, svc, docID); err != nil {
		return err
	}
	rewrittenHeadingLinks := 0
	if markdownMayContainHeadingLinks(cleaned) {
		count, rewriteErr := rewriteMarkdownHeadingLinksFromIndex(ctx, svc, docID, c.Tab, explicitHeadingAnchors, insertedMarkdownStart)
		if rewriteErr != nil {
			return fmt.Errorf("rewrite heading links: %w", rewriteErr)
		}
		rewrittenHeadingLinks = count
		requestCount += count
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
			"written":    inserted,
			"requests":   requestCount,
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
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("written\t%d", inserted)
	u.Out().Linef("requests\t%d", requestCount)
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

// replaceMarkdownInTab implements --replace --markdown --tab=<tab>. Drive's
// markdown converter is whole-document-only, so per-tab whole-tab re-render
// is achieved at the gogcli layer: render markdown locally with the same
// Docs API path used by --append --markdown, after wiping the tab's existing
// body content via DeleteContentRange. Other tabs are untouched.
func (c *DocsWriteCmd) replaceMarkdownInTab(ctx context.Context, flags *RootFlags, docID, content string) error {
	cleaned, images := extractMarkdownImages(content)
	explicitHeadingAnchors := markdownExplicitHeadingAnchors(cleaned)
	dryRunPayload := map[string]any{
		"document_id": docID,
		"written":     len(cleaned),
		"append":      false,
		"replace":     true,
		"markdown":    true,
		"pageless":    c.Pageless,
		"tab":         c.Tab,
		"images":      len(images),
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

	requestCount, inserted, err := insertDocsMarkdownAtWithOptions(ctx, svc, docID, 1, content, tabID, true)
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}
	if err := c.applyDocumentStyle(ctx, svc, docID); err != nil {
		return err
	}
	rewrittenHeadingLinks := 0
	if markdownMayContainHeadingLinks(cleaned) {
		count, rewriteErr := rewriteMarkdownHeadingLinks(ctx, svc, docID, tabID, explicitHeadingAnchors)
		if rewriteErr != nil {
			return fmt.Errorf("rewrite heading links: %w", rewriteErr)
		}
		rewrittenHeadingLinks = count
		requestCount += count
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
			"written":    inserted,
			"requests":   requestCount,
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
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("written\t%d", inserted)
	u.Out().Linef("requests\t%d", requestCount)
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
	TabID        string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	text, provided, err := resolveTextInput(c.Text, c.File, kctx, "text", "file")
	if err != nil {
		return err
	}
	if !provided {
		return usage("required: --text or --file")
	}
	if text == "" {
		return usage("empty text")
	}
	at, placementErr := c.validatePlacement(kctx)
	if placementErr != nil {
		return placementErr
	}

	replaceStart, replaceEnd, replacing, err := parseDocsReplaceRange(c.ReplaceRange)
	if err != nil {
		return err
	}

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab

	if dryRunErr := dryRunExit(ctx, flags, "docs.update", c.dryRunPayload(id, len(text), replacing, replaceStart, replaceEnd, at)); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	insertIndex := c.Index
	anchor, anchorReplacing, anchorErr := c.resolveAtAnchor(ctx, svc, id, at)
	if anchorErr != nil {
		return anchorErr
	}
	if anchorReplacing {
		replaceStart = anchor.Match.StartIndex
		replaceEnd = anchor.Match.EndIndex
		replacing = true
		insertIndex = replaceStart
		c.Tab = anchor.Match.TabID
	}
	switch {
	case replacing:
		insertIndex = replaceStart
		if c.Tab != "" {
			tabID, tabErr := resolveDocsTabID(ctx, svc, id, c.Tab)
			if tabErr != nil {
				return tabErr
			}
			c.Tab = tabID
		}
	case insertIndex <= 0:
		endIndex, tabID, endErr := docsTargetEndIndexAndTabID(ctx, svc, id, c.Tab)
		if endErr != nil {
			return endErr
		}
		c.Tab = tabID
		insertIndex = docsAppendIndex(endIndex)
	case c.Tab != "":
		tabID, tabErr := resolveDocsTabID(ctx, svc, id, c.Tab)
		if tabErr != nil {
			return tabErr
		}
		c.Tab = tabID
	}

	requestCount := 0
	written := len(text)
	var resp *docs.BatchUpdateDocumentResponse

	if c.Markdown {
		var inserted int
		cleaned, _ := extractMarkdownImages(text)
		explicitHeadingAnchors := markdownExplicitHeadingAnchors(cleaned)
		if replacing {
			loadedDoc := anchorDocumentForMarkdownReplace(anchor)
			if loadedDoc == nil {
				loaded, loadErr := loadDocsTargetDocument(ctx, svc, id, c.Tab)
				if loadErr != nil {
					return loadErr
				}
				c.Tab = loaded.tabID
				loadedDoc = loaded.full
			}
			replacedRequests, replacedText, replaceErr := replaceDocsMarkdownRange(ctx, svc, loadedDoc, replaceStart, replaceEnd, text, c.Tab)
			if replaceErr != nil {
				err = replaceErr
			} else {
				inserted = replacedText
				requestCount = replacedRequests
			}
		} else {
			insertedMarkdownStart := insertIndex
			insertElements := ParseMarkdown(cleaned)
			stripMarkdownElementHeadingAnchors(insertElements)
			if insertIndex > 1 && markdownAppendNeedsParagraphBoundary(insertElements) {
				insertedMarkdownStart++
			}
			var insertedMarkdownEnd int64
			requestCount, inserted, insertedMarkdownEnd, err = insertDocsMarkdownAtWithOptionsAndEnd(ctx, svc, id, insertIndex, text, c.Tab, true)
			if err == nil && markdownMayContainHeadingLinks(cleaned) {
				var rewritten int
				rewritten, err = rewriteMarkdownHeadingLinksInRange(ctx, svc, id, c.Tab, explicitHeadingAnchors, insertedMarkdownStart, insertedMarkdownEnd)
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
		reqs := make([]*docs.Request, 0, 2)
		if replacing {
			reqs = append(reqs, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{StartIndex: replaceStart, EndIndex: replaceEnd, TabId: c.Tab},
				},
			})
		}
		reqs = append(reqs, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: insertIndex, TabId: c.Tab},
				Text:     text,
			},
		})
		requestCount = len(reqs)
		batchReq := &docs.BatchUpdateDocumentRequest{Requests: reqs}
		if anchor != nil {
			batchReq.WriteControl = docsRequiredRevisionWriteControl(anchor.RevisionID)
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
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
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
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Linef("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

func (c *DocsUpdateCmd) validatePlacement(kctx *kong.Context) (string, error) {
	if flagProvided(kctx, "index") && c.Index <= 0 {
		return "", usage("invalid --index (must be >= 1)")
	}
	if flagProvided(kctx, "index") && strings.TrimSpace(c.ReplaceRange) != "" {
		return "", usage("--index cannot be combined with --replace-range")
	}
	if err := validateDocsAtAnchorFlags(docsAtAnchorFlags{At: c.At, AtProvided: flagProvided(kctx, "at"), Occurrence: c.Occurrence, MatchCase: c.MatchCase}); err != nil {
		return "", err
	}
	at := c.At
	if at != "" && (flagProvided(kctx, "index") || strings.TrimSpace(c.ReplaceRange) != "") {
		return "", usage("--at cannot be combined with --index or --replace-range")
	}
	return at, nil
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
	}
	if replacing {
		payload["replaceRange"] = map[string]int64{"start": replaceStart, "end": replaceEnd}
	}
	addDocsAtAnchorDryRunPayload(payload, docsAtAnchorFlags{At: at, Occurrence: c.Occurrence, MatchCase: c.MatchCase})
	return payload
}

func (c *DocsUpdateCmd) resolveAtAnchor(ctx context.Context, svc *docs.Service, docID, at string) (*docsResolvedAtAnchor, bool, error) {
	if at == "" {
		return nil, false, nil
	}
	match, err := resolveDocsAtAnchor(ctx, svc, docID, docsAtAnchorFlags{
		At:         at,
		Occurrence: c.Occurrence,
		MatchCase:  c.MatchCase,
		Tab:        c.Tab,
	})
	if err != nil {
		return nil, false, err
	}
	return &match, true, nil
}

func anchorDocumentForMarkdownReplace(anchor *docsResolvedAtAnchor) *docs.Document {
	if anchor == nil {
		return nil
	}
	return anchor.Document
}

func parseDocsReplaceRange(value string) (start int64, end int64, ok bool, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, false, nil
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return 0, 0, false, usage("invalid --replace-range (expected START:END)")
	}
	start, parseErr := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if parseErr != nil || start < 1 {
		return 0, 0, false, usage("invalid --replace-range start (must be >= 1)")
	}
	end, parseErr = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if parseErr != nil || end <= start {
		return 0, 0, false, usage("invalid --replace-range end (must be greater than start)")
	}
	return start, end, true, nil
}

type DocsInsertCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Content    string `arg:"" optional:"" name:"content" help:"Text to insert (or use --file / stdin)"`
	Index      *int64 `name:"index" help:"Character index to insert at (1 = beginning). Defaults to end-of-doc when omitted."`
	At         string `name:"at" help:"Anchor by literal text and insert at the start of the matched range"`
	Occurrence *int   `name:"occurrence" help:"Use the Nth --at match (1-based; required when --at is ambiguous)"`
	MatchCase  bool   `name:"match-case" help:"Use case-sensitive --at matching"`
	File       string `name:"file" short:"f" help:"Read content from file (use - for stdin)"`
	Tab        string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID      string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsInsertCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	content, err := resolveContentInput(c.Content, c.File)
	if err != nil {
		return err
	}
	if content == "" {
		return usage("no content provided (use argument, --file, or stdin)")
	}
	if c.Index != nil && *c.Index < 1 {
		return usage("--index must be >= 1 (index 0 is reserved)")
	}
	if anchorErr := validateDocsAtAnchorFlags(docsAtAnchorFlags{At: c.At, AtProvided: flagProvided(kctx, "at"), Occurrence: c.Occurrence, MatchCase: c.MatchCase}); anchorErr != nil {
		return anchorErr
	}
	at := c.At
	if at != "" && c.Index != nil {
		return usage("--at and --index are mutually exclusive")
	}

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab

	dryRunPayload := map[string]any{
		"documentId": docID,
		"inserted":   len(content),
		"tab":        c.Tab,
	}
	switch {
	case c.Index != nil:
		dryRunPayload["atIndex"] = *c.Index
	case at != "":
		dryRunPayload["atIndex"] = docsAtIndexAnchorStart
		addDocsAtAnchorDryRunPayload(dryRunPayload, docsAtAnchorFlags{At: at, Occurrence: c.Occurrence, MatchCase: c.MatchCase})
	default:
		dryRunPayload["atIndex"] = docsAtIndexEnd
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.insert", dryRunPayload); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	var insertIndex int64
	var anchor *docsResolvedAtAnchor
	switch {
	case at != "":
		match, anchorErr := resolveDocsAtAnchor(ctx, svc, docID, docsAtAnchorFlags{
			At:         at,
			Occurrence: c.Occurrence,
			MatchCase:  c.MatchCase,
			Tab:        c.Tab,
		})
		if anchorErr != nil {
			return anchorErr
		}
		anchor = &match
		insertIndex = match.Match.StartIndex
		c.Tab = match.Match.TabID
	case c.Index != nil:
		insertIndex = *c.Index
		if c.Tab != "" {
			tabID, tabErr := resolveDocsTabID(ctx, svc, docID, c.Tab)
			if tabErr != nil {
				return tabErr
			}
			c.Tab = tabID
		}
	default:
		endIndex, tabID, endErr := docsTargetEndIndexAndTabID(ctx, svc, docID, c.Tab)
		if endErr != nil {
			return endErr
		}
		c.Tab = tabID
		insertIndex = docsAppendIndex(endIndex)
	}

	batchReq := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			InsertText: &docs.InsertTextRequest{
				Text: content,
				Location: &docs.Location{
					Index: insertIndex,
					TabId: c.Tab,
				},
			},
		}},
	}
	if anchor != nil {
		batchReq.WriteControl = docsRequiredRevisionWriteControl(anchor.RevisionID)
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
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Linef("documentId\t%s", result.DocumentId)
	u.Out().Linef("inserted\t%d bytes", len(content))
	u.Out().Linef("atIndex\t%d", insertIndex)
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
	TabID      string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsDeleteCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	if anchorErr := validateDocsAtAnchorFlags(docsAtAnchorFlags{At: c.At, AtProvided: flagProvided(kctx, "at"), Occurrence: c.Occurrence, MatchCase: c.MatchCase}); anchorErr != nil {
		return anchorErr
	}
	at := c.At
	hasNumericRange := c.Start != nil || c.End != nil
	if at != "" && hasNumericRange {
		return usage("--at cannot be combined with --start or --end")
	}
	if at == "" && (c.Start == nil || c.End == nil) {
		return usage("provide --at or both --start and --end")
	}
	if c.Start != nil && *c.Start < 1 {
		return usage("--start must be >= 1")
	}
	if c.Start != nil && c.End != nil && *c.End <= *c.Start {
		return usage("--end must be greater than --start")
	}

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
	}
	addDocsAtAnchorDryRunPayload(dryRunPayload, docsAtAnchorFlags{At: at, Occurrence: c.Occurrence, MatchCase: c.MatchCase})
	if err := dryRunExit(ctx, flags, "docs.delete", dryRunPayload); err != nil {
		return err
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	var start, end int64
	var anchor *docsResolvedAtAnchor
	if at != "" {
		match, anchorErr := resolveDocsAtAnchor(ctx, svc, docID, docsAtAnchorFlags{
			At:         at,
			Occurrence: c.Occurrence,
			MatchCase:  c.MatchCase,
			Tab:        c.Tab,
		})
		if anchorErr != nil {
			return anchorErr
		}
		anchor = &match
		start = match.Match.StartIndex
		end = match.Match.EndIndex
		c.Tab = match.Match.TabID
	} else {
		start = *c.Start
		end = *c.End
	}
	if at == "" && c.Tab != "" {
		tabID, tabErr := resolveDocsTabID(ctx, svc, docID, c.Tab)
		if tabErr != nil {
			return tabErr
		}
		c.Tab = tabID
	}

	batchReq := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{StartIndex: start, EndIndex: end, TabId: c.Tab},
			},
		}},
	}
	if anchor != nil {
		batchReq.WriteControl = docsRequiredRevisionWriteControl(anchor.RevisionID)
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
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Linef("documentId\t%s", result.DocumentId)
	u.Out().Linef("deleted\t%d characters", end-start)
	u.Out().Linef("range\t%d-%d", start, end)
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
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
		startIdx, endIdx, total := findTextInDoc(targetDoc, c.Find, c.MatchCase)
		if total == 0 {
			return c.printFirstResult(ctx, u, docID, replaceText, 0, 0)
		}
		if format == docsContentFormatMarkdown {
			err = c.runMarkdown(ctx, svc, doc, startIdx, endIdx, replaceText)
		} else {
			err = c.runPlain(ctx, svc, doc, startIdx, endIdx, replaceText)
		}
		if err != nil {
			return err
		}
		return c.printFirstResult(ctx, u, docID, replaceText, 1, total)
	}

	matches := findTextMatches(targetDoc, c.Find, c.MatchCase)
	for i := len(matches) - 1; i >= 0; i-- {
		if err = c.runMarkdown(ctx, svc, doc, matches[i].startIndex, matches[i].endIndex, replaceText); err != nil {
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
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
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

	matches := findTextMatches(loaded.target, c.Find, c.MatchCase)
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
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
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
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
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
