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
		u.Err().Printf("Warning: --tab-id is deprecated; use --tab instead")
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

	if c.Markdown {
		if c.Format.any() {
			return usage("formatting flags are only supported for plain-text docs write; use markdown syntax or run docs format after writing")
		}
		return c.writeMarkdown(ctx, flags, id, text)
	}

	return c.writePlainText(ctx, flags, id, text)
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
	if err := c.applyPageless(ctx, svc, docID); err != nil {
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

func (c *DocsWriteCmd) applyPageless(ctx context.Context, svc *docs.Service, docID string) error {
	if !c.Pageless {
		return nil
	}
	if err := setDocumentPageless(ctx, svc, docID); err != nil {
		return fmt.Errorf("set pageless mode: %w", err)
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
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("requests\t%d", requestCount)
	u.Out().Printf("append\t%t", c.Append)
	u.Out().Printf("index\t%d", insertIndex)
	if c.Tab != "" {
		u.Out().Printf("tabId\t%s", c.Tab)
	}
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
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
	// Tab support for markdown replace is limited because Drive's markdown
	// converter doesn't support tab-specific updates, so we skip tab support here.
	if c.Tab != "" {
		return usage("--markdown with --replace does not support --tab (Drive's markdown converter operates on entire documents)")
	}

	cleaned, images := extractMarkdownImages(content)

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
	if len(images) > 0 || c.Pageless {
		var svcErr error
		docsSvc, svcErr = newDocsService(ctx, account)
		if svcErr != nil {
			return svcErr
		}
	}
	if len(images) > 0 {
		if err := insertImagesIntoDocs(ctx, docsSvc, docID, images, ""); err != nil {
			cleanupDocsImagePlaceholders(ctx, docsSvc, docID, images, "")
			return fmt.Errorf("insert images: %w", err)
		}
	}
	if c.Pageless {
		if err := c.applyPageless(ctx, docsSvc, docID); err != nil {
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
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("documentId\t%s", updated.Id)
	u.Out().Printf("written\t%d", len(content))
	u.Out().Printf("mode\treplaced (markdown converted)")
	if c.Pageless {
		u.Out().Printf("pageless\ttrue")
	}
	if updated.WebViewLink != "" {
		u.Out().Printf("link\t%s", updated.WebViewLink)
	}
	return nil
}

func (c *DocsWriteCmd) appendMarkdown(ctx context.Context, flags *RootFlags, docID, content string) error {
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

	requestCount, inserted, err := insertDocsMarkdownAt(ctx, svc, docID, insertIndex, content, c.Tab)
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}
	if err := c.applyPageless(ctx, svc, docID); err != nil {
		return err
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
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("documentId\t%s", docID)
	u.Out().Printf("written\t%d", inserted)
	u.Out().Printf("requests\t%d", requestCount)
	u.Out().Printf("mode\tappended (markdown converted)")
	u.Out().Printf("index\t%d", insertIndex)
	if c.Pageless {
		u.Out().Printf("pageless\ttrue")
	}
	return nil
}

type DocsUpdateCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	Text     string `name:"text" help:"Text to insert"`
	File     string `name:"file" help:"Text file path ('-' for stdin)"`
	Index    int64  `name:"index" help:"Insert index (default: end of document)"`
	Pageless bool   `name:"pageless" help:"Set document to pageless mode"`
	Tab      string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID    string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
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
	if flagProvided(kctx, "index") && c.Index <= 0 {
		return usage("invalid --index (must be >= 1)")
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

	insertIndex := c.Index
	if insertIndex <= 0 {
		endIndex, tabID, endErr := docsTargetEndIndexAndTabID(ctx, svc, id, c.Tab)
		if endErr != nil {
			return endErr
		}
		c.Tab = tabID
		insertIndex = docsAppendIndex(endIndex)
	} else if c.Tab != "" {
		tabID, tabErr := resolveDocsTabID(ctx, svc, id, c.Tab)
		if tabErr != nil {
			return tabErr
		}
		c.Tab = tabID
	}

	reqs := []*docs.Request{{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: insertIndex, TabId: c.Tab},
			Text:     text,
		},
	}}

	resp, err := svc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{Requests: reqs}).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if c.Pageless {
		if err := setDocumentPageless(ctx, svc, id); err != nil {
			return fmt.Errorf("set pageless mode: %w", err)
		}
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": resp.DocumentId,
			"requests":   len(reqs),
			"index":      insertIndex,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("requests\t%d", len(reqs))
	u.Out().Printf("index\t%d", insertIndex)
	if c.Tab != "" {
		u.Out().Printf("tabId\t%s", c.Tab)
	}
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

type DocsInsertCmd struct {
	DocID   string `arg:"" name:"docId" help:"Doc ID"`
	Content string `arg:"" optional:"" name:"content" help:"Text to insert (or use --file / stdin)"`
	Index   int64  `name:"index" help:"Character index to insert at (1 = beginning)" default:"1"`
	File    string `name:"file" short:"f" help:"Read content from file (use - for stdin)"`
	Tab     string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID   string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
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
	if c.Index < 1 {
		return usage("--index must be >= 1 (index 0 is reserved)")
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
	if c.Tab != "" {
		tabID, tabErr := resolveDocsTabID(ctx, svc, docID, c.Tab)
		if tabErr != nil {
			return tabErr
		}
		c.Tab = tabID
	}

	result, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			InsertText: &docs.InsertTextRequest{
				Text: content,
				Location: &docs.Location{
					Index: c.Index,
					TabId: c.Tab,
				},
			},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("inserting text: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{"documentId": result.DocumentId, "inserted": len(content), "atIndex": c.Index}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("documentId\t%s", result.DocumentId)
	u.Out().Printf("inserted\t%d bytes", len(content))
	u.Out().Printf("atIndex\t%d", c.Index)
	if c.Tab != "" {
		u.Out().Printf("tabId\t%s", c.Tab)
	}
	return nil
}

type DocsDeleteCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Start int64  `name:"start" required:"" help:"Start index (>= 1)"`
	End   int64  `name:"end" required:"" help:"End index (> start)"`
	Tab   string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	if c.Start < 1 {
		return usage("--start must be >= 1")
	}
	if c.End <= c.Start {
		return usage("--end must be greater than --start")
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
	if c.Tab != "" {
		tabID, tabErr := resolveDocsTabID(ctx, svc, docID, c.Tab)
		if tabErr != nil {
			return tabErr
		}
		c.Tab = tabID
	}

	result, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{StartIndex: c.Start, EndIndex: c.End, TabId: c.Tab},
			},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("deleting content: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": result.DocumentId,
			"deleted":    c.End - c.Start,
			"startIndex": c.Start,
			"endIndex":   c.End,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("documentId\t%s", result.DocumentId)
	u.Out().Printf("deleted\t%d characters", c.End-c.Start)
	u.Out().Printf("range\t%d-%d", c.Start, c.End)
	if c.Tab != "" {
		u.Out().Printf("tabId\t%s", c.Tab)
	}
	return nil
}

type DocsClearCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
}

func (c *DocsClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
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

	u.Out().Printf("documentId\t%s", docID)
	u.Out().Printf("find\t%s", c.Find)
	u.Out().Printf("replace\t%s", replaceText)
	u.Out().Printf("replacements\t%d", len(matches))
	if c.Tab != "" {
		u.Out().Printf("tabId\t%s", c.Tab)
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
		u.Out().Printf("matches\t%d", len(matches))
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

	u.Out().Printf("documentId\t%s", documentID)
	u.Out().Printf("find\t%s", c.Find)
	u.Out().Printf("replace\t%s", replaceText)
	u.Out().Printf("replacements\t%d", replacements)
	if c.Tab != "" {
		u.Out().Printf("tabId\t%s", c.Tab)
	}
	return nil
}

func (c *DocsFindReplaceCmd) runPlain(ctx context.Context, svc *docs.Service, doc *docs.Document, startIdx, endIdx int64, replaceText string) error {
	return replaceDocsTextRange(ctx, svc, doc, startIdx, endIdx, replaceText, c.Tab)
}

func (c *DocsFindReplaceCmd) runMarkdown(ctx context.Context, svc *docs.Service, doc *docs.Document, startIdx, endIdx int64, replaceText string) error {
	return replaceDocsMarkdownRange(ctx, svc, doc, startIdx, endIdx, replaceText, c.Tab)
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

	u.Out().Printf("documentId\t%s", docID)
	u.Out().Printf("find\t%s", c.Find)
	u.Out().Printf("replace\t%s", replaceText)
	u.Out().Printf("replacements\t%d", replacements)
	if remaining := total - replacements; remaining > 0 {
		u.Out().Printf("remaining\t%d", remaining)
	}
	if c.Tab != "" {
		u.Out().Printf("tabId\t%s", c.Tab)
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
