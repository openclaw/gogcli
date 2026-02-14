package cmd

import (
	"bytes"
	"context"
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
	FindReplace DocsFindReplaceCmd `cmd:"" name:"find-replace" help:"Find and replace text in document"`
	Update      DocsUpdateCmd      `cmd:"" name:"update" help:"Update content in a Google Doc"`
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
	File   string `name:"file" help:"Markdown file to import" type:"existingfile"`
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
		if err := c.insertImages(ctx, account, driveSvc, created.Id, images); err != nil {
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
func (c *DocsCreateCmd) insertImages(ctx context.Context, account string, driveSvc *drive.Service, docID string, images []markdownImage) error {
	docsSvc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	// Read back the document to find placeholder positions.
	doc, err := docsSvc.Documents.Get(docID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("read back document: %w", err)
	}

	placeholders := findPlaceholderIndices(doc, len(images))
	if len(placeholders) == 0 {
		return nil
	}

	// Resolve image URLs — upload local files to Drive temporarily.
	imageURLs := make(map[int]string)
	var tempFileIDs []string
	defer cleanupDriveFileIDsBestEffort(ctx, driveSvc, tempFileIDs)

	for _, img := range images {
		if _, ok := placeholders[img.placeholder()]; !ok {
			continue
		}
		if img.isRemote() {
			imageURLs[img.index] = img.originalRef
			continue
		}

		realPath, resolveErr := resolveMarkdownImagePath(c.File, img.originalRef)
		if resolveErr != nil {
			return resolveErr
		}

		url, fileID, uploadErr := uploadLocalImage(ctx, driveSvc, realPath)
		if uploadErr != nil {
			return uploadErr
		}
		tempFileIDs = append(tempFileIDs, fileID)
		imageURLs[img.index] = url
	}

	reqs := buildImageInsertRequests(placeholders, images, imageURLs)
	if len(reqs) == 0 {
		return nil
	}

	_, err = docsSvc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: reqs,
	}).Context(ctx).Do()
	return err
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

type DocsFindReplaceCmd struct {
	DocID       string `arg:"" name:"docId" help:"Doc ID"`
	Find        string `arg:"" name:"find" help:"Text to find"`
	ReplaceText string `arg:"" name:"replace" help:"Replacement text"`
	MatchCase   bool   `name:"match-case" help:"Case-sensitive matching"`
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

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	result, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			ReplaceAllText: &docs.ReplaceAllTextRequest{
				ContainsText: &docs.SubstringMatchCriteria{
					Text:      c.Find,
					MatchCase: c.MatchCase,
				},
				ReplaceText: c.ReplaceText,
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
			"replace":      c.ReplaceText,
			"replacements": replacements,
		})
	}

	u.Out().Printf("documentId\t%s", result.DocumentId)
	u.Out().Printf("find\t%s", c.Find)
	u.Out().Printf("replace\t%s", c.ReplaceText)
	u.Out().Printf("replacements\t%d", replacements)
	return nil
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
