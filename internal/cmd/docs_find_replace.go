package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/docsedit"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

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
