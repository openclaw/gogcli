package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newDocsService = googleapi.NewDocs

type DocsCmd struct {
	Export      DocsExportCmd      `cmd:"" name:"export" aliases:"download,dl" help:"Export a Google Doc (pdf|docx|txt)"`
	Info        DocsInfoCmd        `cmd:"" name:"info" aliases:"get,show" help:"Get Google Doc metadata"`
	Create      DocsCreateCmd      `cmd:"" name:"create" aliases:"add,new" help:"Create a Google Doc"`
	Copy        DocsCopyCmd        `cmd:"" name:"copy" aliases:"cp,duplicate" help:"Copy a Google Doc"`
	Cat         DocsCatCmd         `cmd:"" name:"cat" aliases:"text,read" help:"Print a Google Doc as plain text"`
	Comments    DocsCommentsCmd    `cmd:"" name:"comments" help:"Manage comments on a Google Doc"`
	ListTabs    DocsListTabsCmd    `cmd:"" name:"list-tabs" help:"List all tabs in a Google Doc"`
	Write       DocsWriteCmd       `cmd:"" name:"write" help:"Write content to a Google Doc"`
	Insert      DocsInsertCmd      `cmd:"" name:"insert" help:"Insert text at a specific position"`
	Delete      DocsDeleteCmd      `cmd:"" name:"delete" help:"Delete text range from document"`
	FindReplace DocsFindReplaceCmd `cmd:"" name:"find-replace" help:"Find and replace text — supports plain text and markdown with images. Use --first for single occurrence."`
	Update      DocsUpdateCmd      `cmd:"" name:"update" help:"Update content in a Google Doc"`
	Edit        DocsEditCmd        `cmd:"" name:"edit" help:"Find and replace text in a Google Doc"`
	Sed         DocsSedCmd         `cmd:"" name:"sed" help:"Regex find/replace (sed-style: s/pattern/replacement/g)"`
	Clear       DocsClearCmd       `cmd:"" name:"clear" help:"Clear all content from a Google Doc"`
}
type DocsExportCmd struct {
	DocID  string         `arg:"" name:"docId" help:"Doc ID"`
	Output OutputPathFlag `embed:""`
	Format string         `name:"format" help:"Export format: pdf|docx|txt" default:"pdf"`
}

func (c *DocsExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	return exportViaDrive(ctx, flags, exportViaDriveOptions{
		ArgName:       "docId",
		ExpectedMime:  "application/vnd.google-apps.document",
		KindLabel:     "Google Doc",
		DefaultFormat: "pdf",
	}, c.DocID, c.Output.Path, c.Format)
}

type DocsInfoCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
}

func (c *DocsInfoCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	doc, err := svc.Documents.Get(id).
		Fields("documentId,title,revisionId").
		Context(ctx).
		Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if doc == nil {
		return errors.New("doc not found")
	}

	file := map[string]any{
		"id":       doc.DocumentId,
		"name":     doc.Title,
		"mimeType": driveMimeGoogleDoc,
	}
	if link := docsWebViewLink(doc.DocumentId); link != "" {
		file["webViewLink"] = link
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			strFile:    file,
			"document": doc,
		})
	}

	u.Out().Printf("id\t%s", doc.DocumentId)
	u.Out().Printf("name\t%s", doc.Title)
	u.Out().Printf("mime\t%s", driveMimeGoogleDoc)
	if link := docsWebViewLink(doc.DocumentId); link != "" {
		u.Out().Printf("link\t%s", link)
	}
	if doc.RevisionId != "" {
		u.Out().Printf("revision\t%s", doc.RevisionId)
	}
	return nil
}

type DocsCreateCmd struct {
	Title  string `arg:"" name:"title" help:"Doc title"`
	Parent string `name:"parent" help:"Destination folder ID"`
	File   string `name:"file" help:"Markdown file to import. Supports inline images via ![alt](url); append {width=N height=N} to control size in points. Local images must be in the same directory as the markdown file or a subdirectory (use relative paths). Remote URLs (https://…) are used directly." type:"existingfile"`
}

func (c *DocsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	driveSvc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	f := &drive.File{
		Name:     title,
		MimeType: "application/vnd.google-apps.document",
	}
	parent := strings.TrimSpace(c.Parent)
	if parent != "" {
		f.Parents = []string{parent}
	}

	createCall := driveSvc.Files.Create(f).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, webViewLink")

	// When --file is set, upload the markdown content and let Drive convert it.
	var images []markdownImage
	if c.File != "" {
		raw, readErr := os.ReadFile(c.File)
		if readErr != nil {
			return fmt.Errorf("read markdown file: %w", readErr)
		}
		content := string(raw)

		var cleaned string
		cleaned, images = extractMarkdownImages(content)

		createCall = createCall.Media(
			strings.NewReader(cleaned),
			gapi.ContentType("text/markdown"),
		)
	}

	created, err := createCall.Context(ctx).Do()
	if err != nil {
		return err
	}
	if created == nil {
		return errors.New("create failed")
	}

	// Pass 2: insert images if any were found.
	if len(images) > 0 {
		if err := c.insertImages(ctx, account, created.Id, images); err != nil {
			return fmt.Errorf("insert images: %w", err)
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{strFile: created})
	}

	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("name\t%s", created.Name)
	u.Out().Printf("mime\t%s", created.MimeType)
	if created.WebViewLink != "" {
		u.Out().Printf("link\t%s", created.WebViewLink)
	}
	return nil
}

// insertImages performs pass 2: reads back the created doc, resolves image URLs,
// and replaces placeholder text with inline images.
func (c *DocsCreateCmd) insertImages(ctx context.Context, account string, docID string, images []markdownImage) error {
	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}
	return insertImagesIntoDocs(ctx, account, svc, docID, images, c.File)
}

type DocsCopyCmd struct {
	DocID  string `arg:"" name:"docId" help:"Doc ID"`
	Title  string `arg:"" name:"title" help:"New title"`
	Parent string `name:"parent" help:"Destination folder ID"`
}

func (c *DocsCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return copyViaDrive(ctx, flags, copyViaDriveOptions{
		ArgName:      "docId",
		ExpectedMime: "application/vnd.google-apps.document",
		KindLabel:    "Google Doc",
	}, c.DocID, c.Title, c.Parent)
}

type DocsCatCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	MaxBytes int64  `name:"max-bytes" help:"Max bytes to read (0 = unlimited)" default:"2000000"`
	Tab      string `name:"tab" help:"Tab title or ID to read (omit for default behavior)"`
	AllTabs  bool   `name:"all-tabs" help:"Show all tabs with headers"`
	Raw      bool   `name:"raw" help:"Output the raw Google Docs API JSON response without modifications"`
}

func (c *DocsCatCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	// --raw: dump the full Google Docs API response as JSON.
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

	// Use tabs API when --tab or --all-tabs is specified.
	if c.Tab != "" || c.AllTabs {
		return c.runWithTabs(ctx, svc, id)
	}

	// Default: original behavior (no tabs API).
	doc, err := svc.Documents.Get(id).
		Context(ctx).
		Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if doc == nil {
		return errors.New("doc not found")
	}

	text := docsPlainText(doc, c.MaxBytes)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"text": text})
	}
	_, err = io.WriteString(os.Stdout, text)
	return err
}

type DocsUpdateCmd struct {
	DocID       string `arg:"" name:"docId" help:"Doc ID"`
	Content     string `name:"content" help:"Text content to insert (mutually exclusive with --content-file)"`
	ContentFile string `name:"content-file" help:"File containing text content to insert"`
	Format      string `name:"format" help:"Content format: plain|markdown" default:"plain"`
	Append      bool   `name:"append" help:"Append to end of document instead of replacing all content"`
	Debug       bool   `name:"debug" help:"Enable debug output for markdown formatter"`
}

const (
	docsContentFormatPlain    = "plain"
	docsContentFormatMarkdown = "markdown"
)

func (c *DocsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if c.Debug {
		debugMarkdown = true
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	var content string
	switch {
	case c.ContentFile != "":
		var data []byte
		data, err = os.ReadFile(c.ContentFile)
		if err != nil {
			return fmt.Errorf("read content file: %w", err)
		}
		content = string(data)
	case c.Content != "":
		content = c.Content
	default:
		return usage("either --content or --content-file is required")
	}

	format := strings.ToLower(strings.TrimSpace(c.Format))
	if format == "" {
		format = docsContentFormatPlain
	}
	switch format {
	case docsContentFormatPlain, docsContentFormatMarkdown:
	default:
		return usage("format must be plain or markdown")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	doc, err := svc.Documents.Get(id).
		Context(ctx).
		Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if doc == nil {
		return errors.New("doc not found")
	}

	insertIndex := int64(1)
	if c.Append && doc.Body != nil && len(doc.Body.Content) > 0 {
		lastEl := doc.Body.Content[len(doc.Body.Content)-1]
		if lastEl != nil && lastEl.EndIndex > 1 {
			insertIndex = lastEl.EndIndex - 1
		}
	}

	baseIndex := int64(1)
	if c.Append {
		baseIndex = insertIndex
	}

	var requests []*docs.Request
	var textToInsert string
	var formattingRequests []*docs.Request
	var tables []TableData

	if format == docsContentFormatMarkdown {
		elements := ParseMarkdown(content)
		formattingRequests, textToInsert, tables = MarkdownToDocsRequests(elements, baseIndex)
	} else {
		textToInsert = content
	}

	if c.Append {
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: insertIndex},
				Text:     textToInsert,
			},
		})
		if format == docsContentFormatMarkdown {
			requests = append(requests, formattingRequests...)
		}
	} else {
		if doc.Body != nil && len(doc.Body.Content) > 0 {
			lastEl := doc.Body.Content[len(doc.Body.Content)-1]
			if lastEl != nil && lastEl.EndIndex > 2 {
				endIdx := lastEl.EndIndex - 1
				requests = append(requests, &docs.Request{
					DeleteContentRange: &docs.DeleteContentRangeRequest{
						Range: &docs.Range{
							StartIndex: 1,
							EndIndex:   endIdx,
						},
					},
				})
			}
		}

		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: 1},
				Text:     textToInsert,
			},
		})

		if format == docsContentFormatMarkdown {
			requests = append(requests, formattingRequests...)
		}
	}

	_, err = svc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update document: %w", err)
	}

	if len(tables) > 0 {
		tableInserter := NewTableInserter(svc, id)
		tableOffset := int64(0)
		for _, table := range tables {
			tableIndex := table.StartIndex + tableOffset
			tableEnd, err := tableInserter.InsertNativeTable(ctx, tableIndex, table.Cells)
			if err != nil {
				return fmt.Errorf("insert native table: %w", err)
			}
			if tableEnd > tableIndex {
				tableOffset += (tableEnd - tableIndex) - 1
			}
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"success": true,
			"docId":   id,
			"action":  map[string]any{"append": c.Append},
		})
	}

	action := "Updated"
	if c.Append {
		action = "Appended to"
	}
	u.Out().Printf("%s document %s", action, id)
	return nil
}

func (c *DocsCatCmd) runWithTabs(ctx context.Context, svc *docs.Service, id string) error {
	doc, err := svc.Documents.Get(id).
		IncludeTabsContent(true).
		Context(ctx).
		Do()
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
		tab := findTab(tabs, c.Tab)
		if tab == nil {
			return fmt.Errorf("tab not found: %s", c.Tab)
		}
		text := tabPlainText(tab, c.MaxBytes)
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
				"tab": tabJSON(tab, text),
			})
		}
		_, err = io.WriteString(os.Stdout, text)
		return err
	}

	// --all-tabs
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	doc, err := svc.Documents.Get(id).
		IncludeTabsContent(true).
		Context(ctx).
		Do()
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
			u.Out().Printf("%s\t%s\t%d",
				tab.TabProperties.TabId,
				tab.TabProperties.Title,
				tab.TabProperties.Index,
			)
		}
	}
	return nil
}

// --- Write / Insert / Delete / Find-Replace commands ---

type DocsWriteCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	Content  string `arg:"" optional:"" name:"content" help:"Content to write (or use --file / stdin)"`
	File     string `name:"file" short:"f" help:"Read content from file (use - for stdin)"`
	Replace  bool   `name:"replace" help:"Replace all content (default: append)"`
	Markdown bool   `name:"markdown" help:"Convert markdown to Google Docs formatting (requires --replace)"`
}

func (c *DocsWriteCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

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

	if c.Markdown {
		return c.writeMarkdown(ctx, account, docID, content)
	}
	return c.writePlainText(ctx, account, docID, content)
}

func (c *DocsWriteCmd) writeMarkdown(ctx context.Context, account, docID, content string) error {
	u := ui.FromContext(ctx)

	if !c.Replace {
		return usage("--markdown requires --replace (cannot append formatted markdown)")
	}

	driveSvc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	updated, err := driveSvc.Files.Update(docID, &drive.File{}).
		Media(strings.NewReader(content), gapi.ContentType("text/markdown")).
		SupportsAllDrives(true).
		Fields("id, name, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("writing markdown to document: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId": updated.Id,
			"written":    len(content),
			"replaced":   true,
			"markdown":   true,
		})
	}

	u.Out().Printf("documentId\t%s", updated.Id)
	u.Out().Printf("written\t%d bytes", len(content))
	u.Out().Printf("mode\treplaced (markdown converted)")
	if updated.WebViewLink != "" {
		u.Out().Printf("link\t%s", updated.WebViewLink)
	}
	return nil
}

func (c *DocsWriteCmd) writePlainText(ctx context.Context, account, docID, content string) error {
	u := ui.FromContext(ctx)

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	var requests []*docs.Request

	if c.Replace {
		var doc *docs.Document
		doc, err = svc.Documents.Get(docID).Context(ctx).Do()
		if err != nil {
			if isDocsNotFound(err) {
				return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
			}
			return fmt.Errorf("getting document: %w", err)
		}
		if doc == nil {
			return errors.New("doc not found")
		}

		endIndex := int64(0)
		if doc.Body != nil && len(doc.Body.Content) > 0 {
			lastEl := doc.Body.Content[len(doc.Body.Content)-1]
			if lastEl != nil && lastEl.EndIndex > 1 {
				endIndex = lastEl.EndIndex - 1
			}
		}
		if endIndex > 1 {
			requests = append(requests, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: 1,
						EndIndex:   endIndex,
					},
				},
			})
		}
	}

	requests = append(requests, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Text:                 content,
			EndOfSegmentLocation: &docs.EndOfSegmentLocation{},
		},
	})

	result, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("writing to document: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId": result.DocumentId,
			"written":    len(content),
			"replaced":   c.Replace,
		})
	}

	u.Out().Printf("documentId\t%s", result.DocumentId)
	u.Out().Printf("written\t%d bytes", len(content))
	if c.Replace {
		u.Out().Printf("mode\treplaced")
	} else {
		u.Out().Printf("mode\tappended")
	}
	return nil
}

type DocsInsertCmd struct {
	DocID   string `arg:"" name:"docId" help:"Doc ID"`
	Content string `arg:"" optional:"" name:"content" help:"Text to insert (or use --file / stdin)"`
	Index   int64  `name:"index" help:"Character index to insert at (1 = beginning)" default:"1"`
	File    string `name:"file" short:"f" help:"Read content from file (use - for stdin)"`
}

func (c *DocsInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

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

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	result, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			InsertText: &docs.InsertTextRequest{
				Text: content,
				Location: &docs.Location{
					Index: c.Index,
				},
			},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("inserting text: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId": result.DocumentId,
			"inserted":   len(content),
			"atIndex":    c.Index,
		})
	}

	u.Out().Printf("documentId\t%s", result.DocumentId)
	u.Out().Printf("inserted\t%d bytes", len(content))
	u.Out().Printf("atIndex\t%d", c.Index)
	return nil
}

type DocsDeleteCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Start int64  `name:"start" required:"" help:"Start index (>= 1)"`
	End   int64  `name:"end" required:"" help:"End index (> start)"`
}

func (c *DocsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

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

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	result, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			DeleteContentRange: &docs.DeleteContentRangeRequest{
				Range: &docs.Range{
					StartIndex: c.Start,
					EndIndex:   c.End,
				},
			},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("deleting content: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId": result.DocumentId,
			"deleted":    c.End - c.Start,
			"startIndex": c.Start,
			"endIndex":   c.End,
		})
	}

	u.Out().Printf("documentId\t%s", result.DocumentId)
	u.Out().Printf("deleted\t%d characters", c.End-c.Start)
	u.Out().Printf("range\t%d-%d", c.Start, c.End)
	return nil
}

type DocsClearCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
}

func (c *DocsClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	// Clear delegates to: gog docs sed <docId> 's/^$//'
	// s/^$// with empty replacement on a non-empty doc = clear all content.
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}

	sedCmd := DocsSedCmd{
		DocID:      docID,
		Expression: `s/^$//`,
	}
	return sedCmd.Run(ctx, flags)
}

type DocsFindReplaceCmd struct {
	DocID       string `arg:"" name:"docId" help:"Document ID or URL."`
	Find        string `arg:"" name:"find" help:"Text to search for in the document."`
	ReplaceText string `arg:"" optional:"" name:"replace" help:"Replacement text (omit when using --content-file)."`
	ContentFile string `name:"content-file" help:"Read replacement from a file instead of the positional argument."`
	MatchCase   bool   `name:"match-case" help:"Case-sensitive matching (default: case-insensitive)."`
	Format      string `name:"format" help:"Replacement format: 'plain' (default) or 'markdown'. Markdown converts bold, italic, headings, lists, code blocks, tables, and inline images into native Google Docs formatting. Images use ![alt](url) syntax; append {width=N} or {width=N height=N} (or shorthand {w=N h=N}) to control size in points (default: 468pt full width). Local images must be in the same directory as --content-file or a subdirectory; remote URLs (https://…) are used directly." default:"plain" enum:"plain,markdown"`
	First       bool   `name:"first" help:"Replace only the first occurrence instead of all."`
}

func (c *DocsFindReplaceCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	if c.Find == "" {
		return usage("find text cannot be empty")
	}

	// Resolve replacement text from arg or --content-file.
	replaceText, err := c.resolveReplaceText()
	if err != nil {
		return err
	}

	format := strings.ToLower(strings.TrimSpace(c.Format))
	if format == "" {
		format = docsContentFormatPlain
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	// Plain replace-all (default path): use ReplaceAllText API.
	if !c.First && format == docsContentFormatPlain {
		return c.runReplaceAll(ctx, u, svc, docID, replaceText)
	}

	// All other paths need the document body to find occurrences.
	doc, err := svc.Documents.Get(docID).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}
	if doc == nil {
		return errors.New("doc not found")
	}

	if c.First {
		// --first: single-occurrence replace.
		startIdx, endIdx, total := findTextInDoc(doc, c.Find, c.MatchCase)
		if total == 0 {
			return c.printFirstResult(ctx, u, docID, 0, 0)
		}
		if format == docsContentFormatMarkdown {
			err = c.runMarkdown(ctx, svc, account, doc, startIdx, endIdx, replaceText)
		} else {
			err = c.runPlain(ctx, svc, doc, startIdx, endIdx, replaceText)
		}
		if err != nil {
			return err
		}
		return c.printFirstResult(ctx, u, docID, 1, total)
	}

	// Markdown replace-all (no --first): loop — find first, apply, repeat.
	for {
		startIdx, endIdx, total := findTextInDoc(doc, c.Find, c.MatchCase)
		if total == 0 {
			break
		}
		if err = c.runMarkdown(ctx, svc, account, doc, startIdx, endIdx, replaceText); err != nil {
			return err
		}
		// Re-read the document for fresh indices.
		doc, err = svc.Documents.Get(docID).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("re-reading document: %w", err)
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId": docID,
			"find":       c.Find,
			"success":    true,
		})
	}
	u.Out().Printf("documentId\t%s", docID)
	u.Out().Printf("find\t%s", c.Find)
	u.Out().Printf("mode\tmarkdown replace-all")
	return nil
}

// runReplaceAll uses the ReplaceAllText API for plain-text replace-all (the default path).
func (c *DocsFindReplaceCmd) runReplaceAll(ctx context.Context, u *ui.UI, svc *docs.Service, docID, replaceText string) error {
	result, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			ReplaceAllText: &docs.ReplaceAllTextRequest{
				ContainsText: &docs.SubstringMatchCriteria{
					Text:      c.Find,
					MatchCase: c.MatchCase,
				},
				ReplaceText: replaceText,
			},
		}},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("find-replace: %w", err)
	}

	replacements := int64(0)
	if len(result.Replies) > 0 && result.Replies[0].ReplaceAllText != nil {
		replacements = result.Replies[0].ReplaceAllText.OccurrencesChanged
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId":   result.DocumentId,
			"find":         c.Find,
			"replace":      replaceText,
			"replacements": replacements,
		})
	}

	u.Out().Printf("documentId\t%s", result.DocumentId)
	u.Out().Printf("find\t%s", c.Find)
	u.Out().Printf("replace\t%s", replaceText)
	u.Out().Printf("replacements\t%d", replacements)
	return nil
}

// runPlain deletes the first occurrence and inserts the replacement text.
func (c *DocsFindReplaceCmd) runPlain(ctx context.Context, svc *docs.Service, doc *docs.Document, startIdx, endIdx int64, replaceText string) error {
	_, err := svc.Documents.BatchUpdate(doc.DocumentId, &docs.BatchUpdateDocumentRequest{
		WriteControl: &docs.WriteControl{
			RequiredRevisionId: doc.RevisionId,
		},
		Requests: []*docs.Request{
			{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: startIdx,
						EndIndex:   endIdx,
					},
				},
			},
			{
				InsertText: &docs.InsertTextRequest{
					Location: &docs.Location{Index: startIdx},
					Text:     replaceText,
				},
			},
		},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("replace: %w", err)
	}
	return nil
}

// runMarkdown deletes the first occurrence and inserts markdown-formatted content.
// If the markdown contains ![alt](url) images, they are inserted as inline images
// in a second pass after the text is placed.
func (c *DocsFindReplaceCmd) runMarkdown(ctx context.Context, svc *docs.Service, account string, doc *docs.Document, startIdx, endIdx int64, replaceText string) error {
	// Extract images before parsing markdown — they become <<IMG_N>> placeholders.
	cleaned, images := extractMarkdownImages(replaceText)

	// Parse markdown and build requests.
	elements := ParseMarkdown(cleaned)
	formattingRequests, textToInsert, tables := MarkdownToDocsRequests(elements, startIdx)

	// Build batch: delete old text, insert new text, apply formatting.
	requests := make([]*docs.Request, 0, 2+len(formattingRequests))
	requests = append(requests, &docs.Request{
		DeleteContentRange: &docs.DeleteContentRangeRequest{
			Range: &docs.Range{
				StartIndex: startIdx,
				EndIndex:   endIdx,
			},
		},
	})
	requests = append(requests, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: startIdx},
			Text:     textToInsert,
		},
	})
	requests = append(requests, formattingRequests...)

	_, err := svc.Documents.BatchUpdate(doc.DocumentId, &docs.BatchUpdateDocumentRequest{
		WriteControl: &docs.WriteControl{
			RequiredRevisionId: doc.RevisionId,
		},
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("replace (markdown): %w", err)
	}

	// Handle native tables if any.
	if len(tables) > 0 {
		tableInserter := NewTableInserter(svc, doc.DocumentId)
		tableOffset := int64(0)
		for _, table := range tables {
			tableIndex := table.StartIndex + tableOffset
			tableEnd, tableErr := tableInserter.InsertNativeTable(ctx, tableIndex, table.Cells)
			if tableErr != nil {
				return fmt.Errorf("insert native table: %w", tableErr)
			}
			if tableEnd > tableIndex {
				tableOffset += (tableEnd - tableIndex) - 1
			}
		}
	}

	// Pass 2: insert inline images if any were found.
	if len(images) > 0 {
		imgErr := c.insertImages(ctx, account, svc, doc.DocumentId, images)
		// Always clean up <<IMG_N>> placeholders — even on success (belt and
		// suspenders) and especially on failure so they never leak into the doc.
		cleanupImagePlaceholders(ctx, svc, doc.DocumentId, images)
		if imgErr != nil {
			return fmt.Errorf("insert images: %w", imgErr)
		}
	}

	return nil
}

// insertImages reads back the document to find <<IMG_N>> placeholders, resolves
// image URLs, and replaces the placeholders with inline images.
func (c *DocsFindReplaceCmd) insertImages(ctx context.Context, account string, svc *docs.Service, docID string, images []markdownImage) error {
	basePath := c.ContentFile
	if basePath == "" {
		basePath = "."
	}
	return insertImagesIntoDocs(ctx, account, svc, docID, images, basePath)
}

// printFirstResult outputs the result for a single-occurrence replace (--first).
func (c *DocsFindReplaceCmd) printFirstResult(ctx context.Context, u *ui.UI, docID string, replacements, total int) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId":   docID,
			"find":         c.Find,
			"replacements": replacements,
			"remaining":    total - replacements,
		})
	}

	u.Out().Printf("documentId\t%s", docID)
	u.Out().Printf("find\t%s", c.Find)
	u.Out().Printf("replacements\t%d", replacements)
	remaining := total - replacements
	if remaining > 0 {
		u.Out().Printf("remaining\t%d", remaining)
	}
	return nil
}

func (c *DocsFindReplaceCmd) resolveReplaceText() (string, error) {
	if c.ContentFile != "" && c.ReplaceText != "" {
		return "", usage("cannot use both replace argument and --content-file")
	}
	if c.ContentFile != "" {
		data, err := os.ReadFile(c.ContentFile)
		if err != nil {
			return "", fmt.Errorf("read content file: %w", err)
		}
		return string(data), nil
	}
	return c.ReplaceText, nil
}

// cleanupImagePlaceholders removes any leftover image placeholder strings
// from the document. This is called after image insertion (success or failure)
// to guarantee placeholders never leak into the final document.
func cleanupImagePlaceholders(ctx context.Context, svc *docs.Service, docID string, images []markdownImage) {
	reqs := make([]*docs.Request, 0, len(images))
	for _, img := range images {
		reqs = append(reqs, &docs.Request{
			ReplaceAllText: &docs.ReplaceAllTextRequest{
				ContainsText: &docs.SubstringMatchCriteria{
					Text:      img.placeholder(),
					MatchCase: true,
				},
				ReplaceText: "",
			},
		})
	}
	// Best-effort: if cleanup fails, we tried.
	_, _ = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: reqs,
	}).Context(ctx).Do()
}

// findTextInDoc searches document body for the first occurrence of searchText.
// Returns startIndex, endIndex of first match, and total match count.
func findTextInDoc(doc *docs.Document, searchText string, matchCase bool) (int64, int64, int) {
	if doc.Body == nil {
		return 0, 0, 0
	}

	find := searchText
	if !matchCase {
		find = strings.ToLower(find)
	}

	var firstStart, firstEnd int64
	total := 0

	for _, el := range doc.Body.Content {
		if el == nil || el.Paragraph == nil {
			continue
		}
		// Build paragraph text with index tracking.
		var paraText strings.Builder
		var paraStart int64
		first := true
		for _, pe := range el.Paragraph.Elements {
			if pe.TextRun == nil {
				continue
			}
			if first {
				paraStart = pe.StartIndex
				first = false
			}
			paraText.WriteString(pe.TextRun.Content)
		}

		text := paraText.String()
		compareText := text
		if !matchCase {
			compareText = strings.ToLower(text)
		}

		offset := 0
		for {
			idx := strings.Index(compareText[offset:], find)
			if idx < 0 {
				break
			}
			absIdx := offset + idx
			matchStart := paraStart + utf16Len(text[:absIdx])
			matchEnd := matchStart + utf16Len(searchText)
			total++
			if total == 1 {
				firstStart = matchStart
				firstEnd = matchEnd
			}
			offset = absIdx + len(find)
		}
	}

	return firstStart, firstEnd, total
}

// resolveContentInput reads content from an argument, file, or stdin.
func resolveContentInput(content, filePath string) (string, error) {
	if content != "" {
		return content, nil
	}
	if filePath != "" {
		if filePath == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return "", fmt.Errorf("reading stdin: %w", err)
			}
			return string(data), nil
		}
		data, err := os.ReadFile(filePath) //nolint:gosec // user-provided path
		if err != nil {
			return "", fmt.Errorf("reading file: %w", err)
		}
		return string(data), nil
	}
	// Check if stdin has data.
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(data), nil
	}
	return "", nil
}

func docsWebViewLink(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return "https://docs.google.com/document/d/" + id + "/edit"
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
			if rowIdx > 0 {
				if !appendLimited(buf, maxBytes, "\n") {
					return false
				}
			}
			for cellIdx, cell := range row.TableCells {
				if cellIdx > 0 {
					if !appendLimited(buf, maxBytes, "\t") {
						return false
					}
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

// flattenTabs recursively collects all tabs (including nested child tabs)
// into a flat slice in document order.
func flattenTabs(tabs []*docs.Tab) []*docs.Tab {
	var result []*docs.Tab
	for _, tab := range tabs {
		if tab == nil {
			continue
		}
		result = append(result, tab)
		if len(tab.ChildTabs) > 0 {
			result = append(result, flattenTabs(tab.ChildTabs)...)
		}
	}
	return result
}

// findTab looks up a tab by title or ID (case-insensitive title match).
func findTab(tabs []*docs.Tab, query string) *docs.Tab {
	query = strings.TrimSpace(query)
	// Try exact ID match first.
	for _, tab := range tabs {
		if tab.TabProperties != nil && tab.TabProperties.TabId == query {
			return tab
		}
	}
	// Fall back to case-insensitive title match.
	lower := strings.ToLower(query)
	for _, tab := range tabs {
		if tab.TabProperties != nil && strings.ToLower(tab.TabProperties.Title) == lower {
			return tab
		}
	}
	return nil
}

// tabTitle returns the display title for a tab.
func tabTitle(tab *docs.Tab) string {
	if tab.TabProperties != nil && tab.TabProperties.Title != "" {
		return tab.TabProperties.Title
	}
	return "(untitled)"
}

// tabPlainText extracts plain text from a tab's document content.
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

// tabJSON returns a JSON-friendly map for a tab with its text content.
func tabJSON(tab *docs.Tab, text string) map[string]any {
	m := map[string]any{"text": text}
	if tab.TabProperties != nil {
		m["id"] = tab.TabProperties.TabId
		m["title"] = tab.TabProperties.Title
		m["index"] = tab.TabProperties.Index
	}
	return m
}

// tabInfoJSON returns a JSON-friendly map for a tab's metadata (no content).
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

func isDocsNotFound(err error) bool {
	var apiErr *gapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == http.StatusNotFound
}
