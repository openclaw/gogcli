package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsCatCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	MaxBytes int64  `name:"max-bytes" help:"Max bytes to read (0 = unlimited)" default:"2000000"`
	Tab      string `name:"tab" help:"Tab title or ID to read (omit for default behavior)"`
	AllTabs  bool   `name:"all-tabs" help:"Show all tabs with headers"`
	Raw      bool   `name:"raw" help:"Output the raw Google Docs API JSON response without modifications"`
	Numbered bool   `name:"numbered" short:"N" help:"Prefix each paragraph with its number"`
}

func (c *DocsCatCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	if c.Raw {
		call := svc.Documents.Get(id).Context(ctx)
		if c.Tab != "" || c.AllTabs {
			call = call.IncludeTabsContent(true)
		}
		doc, rawErr := call.Do()
		if rawErr != nil {
			if isDocsNotFound(rawErr) {
				return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
			}
			return rawErr
		}
		raw, rawErr := doc.MarshalJSON()
		if rawErr != nil {
			return fmt.Errorf("marshalling raw response: %w", rawErr)
		}
		var buf bytes.Buffer
		if indentErr := json.Indent(&buf, raw, "", "  "); indentErr != nil {
			_, werr := os.Stdout.Write(raw)
			return werr
		}
		buf.WriteByte('\n')
		_, rawErr = buf.WriteTo(os.Stdout)
		return rawErr
	}

	if c.Tab != "" || c.AllTabs {
		return c.runWithTabs(ctx, svc, id)
	}

	doc, err := svc.Documents.Get(id).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if doc == nil {
		return errors.New("doc not found")
	}

	if c.Numbered {
		return c.printNumbered(ctx, doc, "")
	}

	text := docsPlainText(doc, c.MaxBytes)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"text": text})
	}
	_, err = io.WriteString(os.Stdout, text)
	return err
}

func (c *DocsCatCmd) runWithTabs(ctx context.Context, svc *docs.Service, id string) error {
	doc, err := svc.Documents.Get(id).IncludeTabsContent(true).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if doc == nil {
		return errors.New("doc not found")
	}

	tabs := flattenTabs(doc.Tabs)
	if c.Tab != "" {
		tab, tabErr := findTab(tabs, c.Tab)
		if tabErr != nil {
			return tabErr
		}
		if c.Numbered {
			return c.printNumbered(ctx, doc, c.Tab)
		}
		text := tabPlainText(tab, c.MaxBytes)
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"tab": tabJSON(tab, text)})
		}
		_, err = io.WriteString(os.Stdout, text)
		return err
	}

	if outfmt.IsJSON(ctx) {
		var out []map[string]any
		for _, tab := range tabs {
			text := tabPlainText(tab, c.MaxBytes)
			out = append(out, tabJSON(tab, text))
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"tabs": out})
	}

	for i, tab := range tabs {
		title := tabTitle(tab)
		if i > 0 {
			if _, err := fmt.Fprintln(os.Stdout); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(os.Stdout, "=== Tab: %s ===\n", title); err != nil {
			return err
		}
		text := tabPlainText(tab, c.MaxBytes)
		if _, err := io.WriteString(os.Stdout, text); err != nil {
			return err
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			if _, err := fmt.Fprintln(os.Stdout); err != nil {
				return err
			}
		}
	}
	return nil
}

type DocsListTabsCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
}

func (c *DocsListTabsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	doc, err := svc.Documents.Get(id).IncludeTabsContent(true).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if doc == nil {
		return errors.New("doc not found")
	}

	tabs := flattenTabs(doc.Tabs)
	if outfmt.IsJSON(ctx) {
		var out []map[string]any
		for _, tab := range tabs {
			out = append(out, tabInfoJSON(tab))
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"tabs": out})
	}

	u.Out().Printf("ID\tTITLE\tINDEX")
	for _, tab := range tabs {
		if tab.TabProperties != nil {
			u.Out().Printf("%s\t%s\t%d", tab.TabProperties.TabId, tab.TabProperties.Title, tab.TabProperties.Index)
		}
	}
	return nil
}

type DocsStructureCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Tab   string `name:"tab" help:"Tab title or ID (omit for default)"`
}

func (c *DocsStructureCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	getCall := svc.Documents.Get(id)
	if c.Tab != "" {
		getCall = getCall.IncludeTabsContent(true)
	}
	doc, err := getCall.Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if doc == nil {
		return errors.New("doc not found")
	}

	pm, err := buildParagraphMap(doc, c.Tab)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, pm)
	}

	u.Out().Printf(" #  TYPE                CONTENT")
	for _, p := range pm.Paragraphs {
		prefix := ""
		if p.IsBullet {
			prefix = strings.Repeat("  ", p.NestLevel) + "* "
		}
		text := p.Text
		if len(text) > 60 {
			text = text[:57] + "..."
		}
		if p.ElemType == "table" {
			text = fmt.Sprintf("[table %dx%d] %s", p.TableRows, p.TableCols, text)
		}
		u.Out().Printf("%2d  %-18s  %s%s", p.Num, p.Type, prefix, text)
	}
	return nil
}

func (c *DocsCatCmd) printNumbered(ctx context.Context, doc *docs.Document, tabID string) error {
	pm, err := buildParagraphMap(doc, tabID)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, pm)
	}

	for _, p := range pm.Paragraphs {
		text := p.Text
		if p.ElemType == "table" {
			text = fmt.Sprintf("[table %dx%d] %s", p.TableRows, p.TableCols, text)
		}
		if _, err := fmt.Fprintf(os.Stdout, "[%d] %s\n", p.Num, text); err != nil {
			return err
		}
	}
	return nil
}

func docsPlainText(doc *docs.Document, maxBytes int64) string {
	if doc == nil || doc.Body == nil {
		return ""
	}

	var buf bytes.Buffer
	for _, el := range doc.Body.Content {
		if !appendDocsElementText(&buf, maxBytes, el) {
			break
		}
	}
	return buf.String()
}

func appendDocsElementText(buf *bytes.Buffer, maxBytes int64, el *docs.StructuralElement) bool {
	if el == nil {
		return true
	}

	switch {
	case el.Paragraph != nil:
		for _, p := range el.Paragraph.Elements {
			if p.TextRun == nil {
				continue
			}
			if !appendLimited(buf, maxBytes, p.TextRun.Content) {
				return false
			}
		}
	case el.Table != nil:
		for rowIdx, row := range el.Table.TableRows {
			if rowIdx > 0 && !appendLimited(buf, maxBytes, "\n") {
				return false
			}
			for cellIdx, cell := range row.TableCells {
				if cellIdx > 0 && !appendLimited(buf, maxBytes, "\t") {
					return false
				}
				for _, content := range cell.Content {
					if !appendDocsElementText(buf, maxBytes, content) {
						return false
					}
				}
			}
		}
	case el.TableOfContents != nil:
		for _, content := range el.TableOfContents.Content {
			if !appendDocsElementText(buf, maxBytes, content) {
				return false
			}
		}
	}

	return true
}

func appendLimited(buf *bytes.Buffer, maxBytes int64, s string) bool {
	if maxBytes <= 0 {
		_, _ = buf.WriteString(s)
		return true
	}
	remaining := int(maxBytes) - buf.Len()
	if remaining <= 0 {
		return false
	}
	if len(s) > remaining {
		_, _ = buf.WriteString(s[:remaining])
		return false
	}
	_, _ = buf.WriteString(s)
	return true
}

func tabPlainText(tab *docs.Tab, maxBytes int64) string {
	if tab == nil || tab.DocumentTab == nil || tab.DocumentTab.Body == nil {
		return ""
	}
	var buf bytes.Buffer
	for _, el := range tab.DocumentTab.Body.Content {
		if !appendDocsElementText(&buf, maxBytes, el) {
			break
		}
	}
	return buf.String()
}

func tabJSON(tab *docs.Tab, text string) map[string]any {
	m := map[string]any{"text": text}
	if tab.TabProperties != nil {
		m["id"] = tab.TabProperties.TabId
		m["title"] = tab.TabProperties.Title
		m["index"] = tab.TabProperties.Index
	}
	return m
}

func tabInfoJSON(tab *docs.Tab) map[string]any {
	m := map[string]any{}
	if tab.TabProperties != nil {
		m["id"] = tab.TabProperties.TabId
		m["title"] = tab.TabProperties.Title
		m["index"] = tab.TabProperties.Index
		if tab.TabProperties.NestingLevel > 0 {
			m["nestingLevel"] = tab.TabProperties.NestingLevel
		}
		if tab.TabProperties.ParentTabId != "" {
			m["parentTabId"] = tab.TabProperties.ParentTabId
		}
	}
	return m
}
