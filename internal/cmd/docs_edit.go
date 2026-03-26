package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsWriteCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	Text     string `name:"text" help:"Text to write"`
	File     string `name:"file" help:"Text file path ('-' for stdin)"`
	Append   bool   `name:"append" help:"Append instead of replacing the document body"`
	Pageless bool   `name:"pageless" help:"Set document to pageless mode"`
	TabID    string `name:"tab-id" help:"Target a specific tab by ID (see docs list-tabs)"`

	// Formatting flags (applied after content write)
	FontFamily    string  `name:"font-family" help:"Font family (e.g. Arial, Georgia, Times New Roman)"`
	FontSize      float64 `name:"font-size" help:"Font size in points (e.g. 12, 14, 16)"`
	TextColor     string  `name:"text-color" help:"Text color as hex (#RRGGBB)"`
	BgColor       string  `name:"bg-color" help:"Background highlight color as hex (#RRGGBB)"`
	Alignment     string  `name:"alignment" help:"Paragraph alignment: left|center|right|justified"`
	Underline     bool    `name:"underline" help:"Apply underline to written text"`
	Strikethrough bool    `name:"strikethrough" help:"Apply strikethrough to written text"`
	LineSpacing   float64 `name:"line-spacing" help:"Line spacing percentage (e.g. 150 = 1.5x)"`
}

func (c *DocsWriteCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
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

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	endIndex, err := docsTargetEndIndex(ctx, svc, id, c.TabID)
	if err != nil {
		return err
	}
	insertIndex := int64(1)
	if c.Append {
		insertIndex = docsAppendIndex(endIndex)
	}

	var reqs []*docs.Request
	if !c.Append {
		deleteEnd := endIndex - 1
		if deleteEnd > 1 {
			reqs = append(reqs, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{StartIndex: 1, EndIndex: deleteEnd, TabId: c.TabID},
				},
			})
		}
	}
	reqs = append(reqs, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: insertIndex, TabId: c.TabID},
			Text:     text,
		},
	})

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

	// Apply formatting if any flags set
	fmtOpts := c.formattingOpts()
	if fmtOpts.hasAny() {
		docEnd, endErr := docsTargetEndIndex(ctx, svc, id, c.TabID)
		if endErr != nil {
			return fmt.Errorf("re-fetch document for formatting: %w", endErr)
		}
		fmtEnd := docEnd - 1
		if fmtEnd > 1 {
			fmtReqs := buildFormattingRequests(1, fmtEnd, fmtOpts)
			if len(fmtReqs) > 0 {
				_, err = svc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{Requests: fmtReqs}).Context(ctx).Do()
				if err != nil {
					return fmt.Errorf("apply formatting: %w", err)
				}
			}
		}
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": resp.DocumentId,
			"requests":   len(reqs),
			"append":     c.Append,
			"index":      insertIndex,
		}
		if c.TabID != "" {
			payload["tabId"] = c.TabID
		}
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("requests\t%d", len(reqs))
	u.Out().Printf("append\t%t", c.Append)
	u.Out().Printf("index\t%d", insertIndex)
	if c.TabID != "" {
		u.Out().Printf("tabId\t%s", c.TabID)
	}
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

func (c *DocsWriteCmd) formattingOpts() FormattingOpts {
	opts := FormattingOpts{
		FontFamily: c.FontFamily, FontSize: c.FontSize,
		TextColor: c.TextColor, BgColor: c.BgColor,
		Alignment: c.Alignment, LineSpacing: c.LineSpacing,
	}
	if c.Underline {
		v := true
		opts.Underline = &v
	}
	if c.Strikethrough {
		v := true
		opts.Strikethrough = &v
	}
	return opts
}

type DocsUpdateCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	Text     string `name:"text" help:"Text to insert"`
	File     string `name:"file" help:"Text file path ('-' for stdin)"`
	Index    int64  `name:"index" help:"Insert index (default: end of document)"`
	Pageless bool   `name:"pageless" help:"Set document to pageless mode"`
	TabID    string `name:"tab-id" help:"Target a specific tab by ID (see docs list-tabs)"`
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

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	insertIndex := c.Index
	if insertIndex <= 0 {
		endIndex, endErr := docsTargetEndIndex(ctx, svc, id, c.TabID)
		if endErr != nil {
			return endErr
		}
		insertIndex = docsAppendIndex(endIndex)
	}

	reqs := []*docs.Request{{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: insertIndex, TabId: c.TabID},
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
		if c.TabID != "" {
			payload["tabId"] = c.TabID
		}
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("requests\t%d", len(reqs))
	u.Out().Printf("index\t%d", insertIndex)
	if c.TabID != "" {
		u.Out().Printf("tabId\t%s", c.TabID)
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
	TabID   string `name:"tab-id" help:"Target a specific tab by ID (see docs list-tabs)"`
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

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	result, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			InsertText: &docs.InsertTextRequest{
				Text: content,
				Location: &docs.Location{
					Index: c.Index,
					TabId: c.TabID,
				},
			},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("inserting text: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{"documentId": result.DocumentId, "inserted": len(content), "atIndex": c.Index}
		if c.TabID != "" {
			payload["tabId"] = c.TabID
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("documentId\t%s", result.DocumentId)
	u.Out().Printf("inserted\t%d bytes", len(content))
	u.Out().Printf("atIndex\t%d", c.Index)
	if c.TabID != "" {
		u.Out().Printf("tabId\t%s", c.TabID)
	}
	return nil
}

type DocsDeleteCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Start int64  `name:"start" required:"" help:"Start index (>= 1)"`
	End   int64  `name:"end" required:"" help:"End index (> start)"`
	TabID string `name:"tab-id" help:"Target a specific tab by ID (see docs list-tabs)"`
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

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	result, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{StartIndex: c.Start, EndIndex: c.End, TabId: c.TabID},
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
		if c.TabID != "" {
			payload["tabId"] = c.TabID
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("documentId\t%s", result.DocumentId)
	u.Out().Printf("deleted\t%d characters", c.End-c.Start)
	u.Out().Printf("range\t%d-%d", c.Start, c.End)
	if c.TabID != "" {
		u.Out().Printf("tabId\t%s", c.TabID)
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
	Format      string `name:"format" help:"Replacement format: plain|markdown. Markdown converts formatting, tables, and inline images; local images must be under --content-file's directory (or use remote URLs)." default:"plain" enum:"plain,markdown"`
	First       bool   `name:"first" help:"Replace only the first occurrence instead of all."`
	TabID       string `name:"tab-id" help:"Target a specific tab by ID (see docs list-tabs)"`
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
	if c.TabID != "" && format == docsContentFormatMarkdown {
		return usage("--tab-id is not yet supported with --format markdown")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	if !c.First && format == docsContentFormatPlain {
		return c.runReplaceAll(ctx, u, svc, docID, replaceText)
	}

	loaded, err := loadDocsTargetDocument(ctx, svc, docID, c.TabID)
	if err != nil {
		return err
	}
	doc := loaded.full
	targetDoc := loaded.target

	if c.First {
		startIdx, endIdx, total := findTextInDoc(targetDoc, c.Find, c.MatchCase)
		if total == 0 {
			return c.printFirstResult(ctx, u, docID, replaceText, 0, 0)
		}
		if format == docsContentFormatMarkdown {
			err = c.runMarkdown(ctx, svc, account, doc, startIdx, endIdx, replaceText)
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
		if err = c.runMarkdown(ctx, svc, account, doc, matches[i].startIndex, matches[i].endIndex, replaceText); err != nil {
			return err
		}
		if i == 0 {
			continue
		}
		loaded, err = loadDocsTargetDocument(ctx, svc, docID, c.TabID)
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
		if c.TabID != "" {
			payload["tabId"] = c.TabID
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("documentId\t%s", docID)
	u.Out().Printf("find\t%s", c.Find)
	u.Out().Printf("replace\t%s", replaceText)
	u.Out().Printf("replacements\t%d", len(matches))
	if c.TabID != "" {
		u.Out().Printf("tabId\t%s", c.TabID)
	}
	return nil
}

func (c *DocsFindReplaceCmd) runReplaceAll(ctx context.Context, u *ui.UI, svc *docs.Service, docID, replaceText string) error {
	documentID, replacements, err := runDocsReplaceAll(ctx, svc, docID, c.Find, replaceText, c.MatchCase, c.TabID)
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
		if c.TabID != "" {
			payload["tabId"] = c.TabID
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("documentId\t%s", documentID)
	u.Out().Printf("find\t%s", c.Find)
	u.Out().Printf("replace\t%s", replaceText)
	u.Out().Printf("replacements\t%d", replacements)
	if c.TabID != "" {
		u.Out().Printf("tabId\t%s", c.TabID)
	}
	return nil
}

func (c *DocsFindReplaceCmd) runPlain(ctx context.Context, svc *docs.Service, doc *docs.Document, startIdx, endIdx int64, replaceText string) error {
	return replaceDocsTextRange(ctx, svc, doc, startIdx, endIdx, replaceText, c.TabID)
}

func (c *DocsFindReplaceCmd) runMarkdown(ctx context.Context, svc *docs.Service, account string, doc *docs.Document, startIdx, endIdx int64, replaceText string) error {
	basePath := c.ContentFile
	if basePath == "" {
		basePath = "."
	}
	return replaceDocsMarkdownRange(ctx, svc, account, doc, startIdx, endIdx, replaceText, basePath)
}

func (c *DocsFindReplaceCmd) printFirstResult(ctx context.Context, u *ui.UI, docID, replaceText string, replacements, total int) error {
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId":   docID,
			"find":         c.Find,
			"replacements": replacements,
			"remaining":    total - replacements,
		}
		if c.TabID != "" {
			payload["tabId"] = c.TabID
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
	if c.TabID != "" {
		u.Out().Printf("tabId\t%s", c.TabID)
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

// DocsFormatCmd applies formatting to existing text in a Google Doc.
type DocsFormatCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Match string `name:"match" help:"Text to find and format (first occurrence unless --match-all)"`
	All   bool   `name:"match-all" help:"Format all occurrences of --match text"`

	FontFamily    string  `name:"font-family" help:"Font family (e.g. Arial, Georgia)"`
	FontSize      float64 `name:"font-size" help:"Font size in points"`
	TextColor     string  `name:"text-color" help:"Text color as hex (#RRGGBB)"`
	BgColor       string  `name:"bg-color" help:"Background highlight color as hex (#RRGGBB)"`
	Bold          bool    `name:"bold" help:"Apply bold"`
	Italic        bool    `name:"italic" help:"Apply italic"`
	Underline     bool    `name:"underline" help:"Apply underline"`
	Strikethrough bool    `name:"strikethrough" help:"Apply strikethrough"`
	Alignment     string  `name:"alignment" help:"Paragraph alignment: left|center|right|justified"`
	LineSpacing   float64 `name:"line-spacing" help:"Line spacing percentage (e.g. 150 = 1.5x)"`
}

func (c *DocsFormatCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	opts := c.formattingOpts()
	if !opts.hasAny() {
		return usage("at least one formatting flag is required")
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	doc, err := svc.Documents.Get(id).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}

	type fmtRange struct{ start, end int64 }
	var ranges []fmtRange

	if c.Match != "" {
		if c.All {
			matches := findTextMatches(doc, c.Match, true)
			for _, m := range matches {
				ranges = append(ranges, fmtRange{m.startIndex, m.endIndex})
			}
		} else {
			start, end, total := findTextInDoc(doc, c.Match, true)
			if total == 0 {
				return fmt.Errorf("text %q not found in document", c.Match)
			}
			ranges = append(ranges, fmtRange{start, end})
		}
		if len(ranges) == 0 {
			return fmt.Errorf("text %q not found in document", c.Match)
		}
	} else {
		// Apply to entire document
		if doc.Body != nil && len(doc.Body.Content) > 0 {
			last := doc.Body.Content[len(doc.Body.Content)-1]
			if last != nil && last.EndIndex > 2 {
				ranges = append(ranges, fmtRange{1, last.EndIndex - 1})
			}
		}
		if len(ranges) == 0 {
			return fmt.Errorf("document is empty")
		}
	}

	var allReqs []*docs.Request
	for _, r := range ranges {
		allReqs = append(allReqs, buildFormattingRequests(r.start, r.end, opts)...)
	}

	_, err = svc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{Requests: allReqs}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("apply formatting: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"success":    true,
			"documentId": id,
			"ranges":     len(ranges),
		})
	}

	u.Out().Printf("Formatted %d range(s) in document %s", len(ranges), id)
	return nil
}

func (c *DocsFormatCmd) formattingOpts() FormattingOpts {
	opts := FormattingOpts{
		FontFamily: c.FontFamily, FontSize: c.FontSize,
		TextColor: c.TextColor, BgColor: c.BgColor,
		Alignment: c.Alignment, LineSpacing: c.LineSpacing,
	}
	if c.Bold {
		v := true
		opts.Bold = &v
	}
	if c.Italic {
		v := true
		opts.Italic = &v
	}
	if c.Underline {
		v := true
		opts.Underline = &v
	}
	if c.Strikethrough {
		v := true
		opts.Strikethrough = &v
	}
	return opts
}
