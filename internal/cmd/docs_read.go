package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/alecthomas/kong"
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
	Chips    bool   `name:"chips" help:"Render Google Docs smart chips and text links inline in text output"`
}

func (c *DocsCatCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	tabProvided := flagProvided(kctx, "tab") || c.Tab != ""
	tab := strings.TrimSpace(c.Tab)
	if tabProvided && tab == "" {
		return usage("--tab cannot be empty")
	}
	c.Tab = tab
	if c.Tab != "" && c.AllTabs {
		return usage("--tab and --all-tabs cannot be used together")
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
			_, werr := stdoutWriter(ctx).Write(raw)
			return werr
		}
		buf.WriteByte('\n')
		_, rawErr = buf.WriteTo(stdoutWriter(ctx))
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), docsTextJSON(text, docsRenderedText(doc, c.MaxBytes, true)))
	}
	if c.Chips {
		_, err = io.WriteString(stdoutWriter(ctx), docsRenderedText(doc, c.MaxBytes, false).Text)
		return err
	}
	_, err = io.WriteString(stdoutWriter(ctx), text)
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
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"tab": tabJSON(tab, text, tabRenderedText(doc.DocumentId, tab, c.MaxBytes, true))})
		}
		if c.Chips {
			_, err = io.WriteString(stdoutWriter(ctx), tabRenderedText(doc.DocumentId, tab, c.MaxBytes, false).Text)
			return err
		}
		_, err = io.WriteString(stdoutWriter(ctx), text)
		return err
	}

	if outfmt.IsJSON(ctx) {
		var out []map[string]any
		for _, tab := range tabs {
			text := tabPlainText(tab, c.MaxBytes)
			out = append(out, tabJSON(tab, text, tabRenderedText(doc.DocumentId, tab, c.MaxBytes, true)))
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"tabs": out})
	}

	out := stdoutWriter(ctx)
	for i, tab := range tabs {
		title := tabTitle(tab)
		if i > 0 {
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(out, "=== Tab: %s ===\n", title); err != nil {
			return err
		}
		text := tabPlainText(tab, c.MaxBytes)
		if c.Chips {
			text = tabRenderedText(doc.DocumentId, tab, c.MaxBytes, false).Text
		}
		if _, err := io.WriteString(out, text); err != nil {
			return err
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			if _, err := fmt.Fprintln(out); err != nil {
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"tabs": out})
	}

	u.Out().Linef("ID\tTITLE\tINDEX")
	for _, tab := range tabs {
		if tab.TabProperties != nil {
			u.Out().Linef("%s\t%s\t%d", tab.TabProperties.TabId, tab.TabProperties.Title, tab.TabProperties.Index)
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), pm)
	}

	u.Out().Linef(" #  TYPE                CONTENT")
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
		u.Out().Linef("%2d  %-18s  %s%s", p.Num, p.Type, prefix, text)
	}
	return nil
}

func (c *DocsCatCmd) printNumbered(ctx context.Context, doc *docs.Document, tabID string) error {
	var (
		pm  *paragraphMap
		err error
	)
	if c.Chips {
		pm, err = buildParagraphMapWithChips(doc, tabID)
	} else {
		pm, err = buildParagraphMap(doc, tabID)
	}
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), pm)
	}

	for _, p := range pm.Paragraphs {
		text := p.Text
		if p.ElemType == "table" {
			text = fmt.Sprintf("[table %dx%d] %s", p.TableRows, p.TableCols, text)
		}
		if _, err := fmt.Fprintf(stdoutWriter(ctx), "[%d] %s\n", p.Num, text); err != nil {
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

type docsTextResult struct {
	Text  string
	Chips []docsSmartChip
	Links []docsTextLink
}

type docsSmartChip struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	StartIndex int64  `json:"startIndex,omitempty"`
	EndIndex   int64  `json:"endIndex,omitempty"`
	Name       string `json:"name,omitempty"`
	Email      string `json:"email,omitempty"`
	DateID     string `json:"dateId,omitempty"`
	Display    string `json:"displayText,omitempty"`
	Timestamp  string `json:"timestamp,omitempty"`
	Title      string `json:"title,omitempty"`
	URI        string `json:"uri,omitempty"`
	MimeType   string `json:"mimeType,omitempty"`
}

type docsTextLink struct {
	Text       string `json:"text"`
	URL        string `json:"url,omitempty"`
	BookmarkID string `json:"bookmarkId,omitempty"`
	HeadingID  string `json:"headingId,omitempty"`
	TabID      string `json:"tabId,omitempty"`
	StartIndex int64  `json:"startIndex,omitempty"`
	EndIndex   int64  `json:"endIndex,omitempty"`
}

func docsRenderedText(doc *docs.Document, maxBytes int64, collectMetadata bool) docsTextResult {
	if doc == nil || doc.Body == nil {
		return docsTextResult{}
	}

	var result docsTextResult
	var buf bytes.Buffer
	for _, el := range doc.Body.Content {
		if !appendDocsElementRenderedText(&buf, maxBytes, doc.DocumentId, el, collectMetadata, &result.Chips, &result.Links) {
			break
		}
	}
	result.Text = buf.String()
	return result
}

func tabRenderedText(docID string, tab *docs.Tab, maxBytes int64, collectMetadata bool) docsTextResult {
	if tab == nil || tab.DocumentTab == nil || tab.DocumentTab.Body == nil {
		return docsTextResult{}
	}

	var result docsTextResult
	var buf bytes.Buffer
	for _, el := range tab.DocumentTab.Body.Content {
		if !appendDocsElementRenderedText(&buf, maxBytes, docID, el, collectMetadata, &result.Chips, &result.Links) {
			break
		}
	}
	result.Text = buf.String()
	return result
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

func appendDocsElementRenderedText(buf *bytes.Buffer, maxBytes int64, docID string, el *docs.StructuralElement, collectMetadata bool, chips *[]docsSmartChip, links *[]docsTextLink) bool {
	if el == nil {
		return true
	}

	switch {
	case el.Paragraph != nil:
		for _, p := range el.Paragraph.Elements {
			if p.TextRun != nil {
				rendered, link, ok := renderDocsTextRunLink(docID, p)
				if ok {
					if rendered == p.TextRun.Content {
						if !appendLimited(buf, maxBytes, rendered) {
							return false
						}
					} else if !appendWholeLimited(buf, maxBytes, rendered) {
						return false
					}
					if collectMetadata {
						*links = append(*links, link)
					}
					continue
				}
				if !appendLimited(buf, maxBytes, p.TextRun.Content) {
					return false
				}
				continue
			}
			chip, ok := renderDocsSmartChip(p)
			if !ok {
				continue
			}
			// Smart chips are atomic document elements. Do not emit a partial
			// mention or Markdown link when the output byte limit lands inside one.
			if !appendWholeLimited(buf, maxBytes, chip.Text) {
				return false
			}
			if collectMetadata {
				*chips = append(*chips, chip)
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
					if !appendDocsElementRenderedText(buf, maxBytes, docID, content, collectMetadata, chips, links) {
						return false
					}
				}
			}
		}
	case el.TableOfContents != nil:
		for _, content := range el.TableOfContents.Content {
			if !appendDocsElementRenderedText(buf, maxBytes, docID, content, collectMetadata, chips, links) {
				return false
			}
		}
	}

	return true
}

func renderDocsTextRunLink(docID string, p *docs.ParagraphElement) (string, docsTextLink, bool) {
	if p == nil || p.TextRun == nil || p.TextRun.TextStyle == nil {
		return "", docsTextLink{}, false
	}
	target := docsParagraphRunLinkFrom(p.TextRun.TextStyle.Link)
	text := p.TextRun.Content
	if target == nil || text == "" {
		return text, docsTextLink{}, false
	}

	link := docsTextLink{
		Text:       text,
		URL:        strings.TrimSpace(target.URL),
		BookmarkID: strings.TrimSpace(target.BookmarkID),
		HeadingID:  strings.TrimSpace(target.HeadingID),
		TabID:      strings.TrimSpace(target.TabID),
		StartIndex: p.StartIndex,
		EndIndex:   p.EndIndex,
	}
	targetURL := docsTextLinkTargetURL(docID, link)

	rendered := text
	if targetURL != "" && strings.TrimSpace(text) != targetURL {
		rendered = fmt.Sprintf("[%s](%s)", escapeMarkdownLinkLabel(text), escapeMarkdownLinkDestination(targetURL))
	}
	return rendered, link, true
}

func docsTextLinkTargetURL(docID string, link docsTextLink) string {
	if link.URL != "" {
		return link.URL
	}
	base := docsWebViewLink(docID)
	if base == "" {
		return ""
	}
	if link.TabID != "" {
		base += "?tab=" + url.QueryEscape(link.TabID)
	}
	switch {
	case link.BookmarkID != "":
		return base + "#bookmark=" + url.QueryEscape(link.BookmarkID)
	case link.HeadingID != "":
		return base + "#heading=" + url.QueryEscape(link.HeadingID)
	case link.TabID != "":
		return base
	default:
		return ""
	}
}

func renderDocsSmartChip(p *docs.ParagraphElement) (docsSmartChip, bool) {
	if p == nil {
		return docsSmartChip{}, false
	}

	switch {
	case p.Person != nil:
		props := p.Person.PersonProperties
		name, email := "", ""
		if props != nil {
			name = strings.TrimSpace(props.Name)
			email = strings.TrimSpace(props.Email)
		}
		text := "@" + firstNonEmpty(name, email)
		if name != "" && email != "" {
			text = fmt.Sprintf("@%s <%s>", name, email)
		}
		if text == "@" {
			return docsSmartChip{}, false
		}
		return docsSmartChip{
			Type:       "person",
			Text:       text,
			StartIndex: p.StartIndex,
			EndIndex:   p.EndIndex,
			Name:       name,
			Email:      email,
		}, true
	case p.DateElement != nil:
		props := p.DateElement.DateElementProperties
		display, timestamp := "", ""
		if props != nil {
			display = strings.TrimSpace(props.DisplayText)
			timestamp = strings.TrimSpace(props.Timestamp)
		}
		text := firstNonEmpty(display, timestamp)
		if text == "" {
			return docsSmartChip{}, false
		}
		return docsSmartChip{
			Type:       "date",
			Text:       text,
			StartIndex: p.StartIndex,
			EndIndex:   p.EndIndex,
			DateID:     p.DateElement.DateId,
			Display:    display,
			Timestamp:  timestamp,
		}, true
	case p.RichLink != nil:
		props := p.RichLink.RichLinkProperties
		title, uri, mimeType := "", "", ""
		if props != nil {
			title = strings.TrimSpace(props.Title)
			uri = strings.TrimSpace(props.Uri)
			mimeType = strings.TrimSpace(props.MimeType)
		}
		text := firstNonEmpty(title, uri)
		if title != "" && uri != "" {
			text = fmt.Sprintf("[%s](%s)", escapeMarkdownLinkLabel(title), escapeMarkdownLinkDestination(uri))
		}
		if text == "" {
			return docsSmartChip{}, false
		}
		return docsSmartChip{
			Type:       "richLink",
			Text:       text,
			StartIndex: p.StartIndex,
			EndIndex:   p.EndIndex,
			Title:      title,
			URI:        uri,
			MimeType:   mimeType,
		}, true
	default:
		return docsSmartChip{}, false
	}
}

func escapeMarkdownLinkLabel(s string) string {
	return strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`).Replace(s)
}

func escapeMarkdownLinkDestination(s string) string {
	return strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(s)
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

func appendWholeLimited(buf *bytes.Buffer, maxBytes int64, s string) bool {
	if maxBytes > 0 && len(s) > int(maxBytes)-buf.Len() {
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

func docsTextJSON(text string, rendered docsTextResult) map[string]any {
	m := map[string]any{"text": text}
	if rendered.Text != text {
		m["renderedText"] = rendered.Text
	}
	if len(rendered.Chips) > 0 {
		m["chips"] = rendered.Chips
	}
	if len(rendered.Links) > 0 {
		m["links"] = rendered.Links
	}
	return m
}

func tabJSON(tab *docs.Tab, text string, rendered docsTextResult) map[string]any {
	m := docsTextJSON(text, rendered)
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
