package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/docsedit"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

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
	if tab == "" && (flagProvided(kctx, "tab") || flagProvided(kctx, "tab-id")) {
		return usage("--tab requires a non-empty tab title or ID")
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
	// Document-style fields are per-tab. Resolve --tab to a concrete tab ID so
	// pageless/layout lands on the targeted tab rather than silently hitting the
	// default tab. resolveDocsTabID is a no-op when the tab is already a concrete
	// ID (as in the plain-text path) and skipped entirely when no tab was given.
	tabID := ""
	if tab := strings.TrimSpace(c.Tab); tab != "" {
		resolved, err := resolveDocsTabID(ctx, svc, docID, tab)
		if err != nil {
			return fmt.Errorf("resolve tab %q: %w", tab, err)
		}
		tabID = resolved
	}
	if err := setDocumentStyle(ctx, svc, docID, docsDocumentStyleOptions{
		Mode:            mode,
		TabID:           tabID,
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
