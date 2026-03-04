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

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/config"
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
	Comments    DocsCommentsCmd    `cmd:"" name:"comments" help:"Manage comments on files"`
	ListTabs    DocsListTabsCmd    `cmd:"" name:"list-tabs" help:"List all tabs in a Google Doc"`
	Write       DocsWriteCmd       `cmd:"" name:"write" help:"Write content to a Google Doc"`
	Insert      DocsInsertCmd      `cmd:"" name:"insert" help:"Insert text at a specific position"`
	Delete      DocsDeleteCmd      `cmd:"" name:"delete" help:"Delete text range from document"`
	FindReplace DocsFindReplaceCmd `cmd:"" name:"find-replace" help:"Find and replace text in document"`
	Color       DocsColorCmd       `cmd:"" name:"color" help:"Apply text color to existing text in a document"`
	Update      DocsUpdateCmd      `cmd:"" name:"update" help:"Insert text at a specific index in a Google Doc"`
	Edit        DocsEditCmd        `cmd:"" name:"edit" help:"Find and replace text in a Google Doc"`
	Sed         DocsSedCmd         `cmd:"" name:"sed" help:"Regex find/replace (sed-style: s/pattern/replacement/g)"`
	Clear       DocsClearCmd       `cmd:"" name:"clear" help:"Clear all content from a Google Doc"`
	Structure   DocsStructureCmd   `cmd:"" name:"structure" aliases:"struct" help:"Show document structure with numbered paragraphs"`
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

type DocsWriteCmd struct {
	DocID     string `arg:"" name:"docId" help:"Doc ID"`
	Text      string `name:"text" help:"Text to write"`
	File      string `name:"file" help:"Text file path ('-' for stdin)"`
	Append    bool   `name:"append" help:"Append instead of replacing the document body"`
	TextColor string `name:"text-color" help:"Set text color (hex #RRGGBB or name: red, blue, green, ...)"`
}

func (c *DocsWriteCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

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

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	doc, err := svc.Documents.Get(id).
		Fields("documentId,body/content(startIndex,endIndex)").
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

	endIndex := docsDocumentEndIndex(doc)
	insertIndex := int64(1)
	if c.Append {
		insertIndex = docsAppendIndex(endIndex)
	}

	// Parse text color if specified
	var textColor *docs.OptionalColor
	if c.TextColor != "" {
		textColor, err = ParseTextColor(c.TextColor)
		if err != nil {
			return err
		}
	}

	reqs := []*docs.Request{}
	if !c.Append {
		deleteEnd := endIndex - 1
		if deleteEnd > 1 {
			reqs = append(reqs, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: 1,
						EndIndex:   deleteEnd,
					},
				},
			})
		}
	}

	reqs = append(reqs, &docs.Request{
		InsertText: &docs.InsertTextRequest{
			Location: &docs.Location{Index: insertIndex},
			Text:     text,
		},
	})

	// Apply text color to inserted text
	if textColor != nil {
		textEnd := insertIndex + utf16Len(text)
		reqs = append(reqs, BuildColorRequest(insertIndex, textEnd, textColor))
	}

	resp, err := svc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{Requests: reqs}).
		Context(ctx).
		Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": resp.DocumentId,
			"requests":   len(reqs),
			"append":     c.Append,
			"index":      insertIndex,
		}
		if textColor != nil {
			payload["textColor"] = c.TextColor
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
	if textColor != nil {
		u.Out().Printf("textColor\t%s", c.TextColor)
	}
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

type DocsUpdateCmd struct {
	DocID     string `arg:"" name:"docId" help:"Doc ID"`
	Text      string `name:"text" help:"Text to insert"`
	File      string `name:"file" help:"Text file path ('-' for stdin)"`
	Index     int64  `name:"index" help:"Insert index (default: end of document)"`
	TextColor string `name:"text-color" help:"Set text color (hex #RRGGBB or name: red, blue, green, ...)"`
}

func (c *DocsUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

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

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	insertIndex := c.Index
	if insertIndex <= 0 {
		var doc *docs.Document
		doc, err = svc.Documents.Get(id).
			Fields("documentId,body/content(startIndex,endIndex)").
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
		insertIndex = docsAppendIndex(docsDocumentEndIndex(doc))
	}

	// Parse text color if specified
	var textColor *docs.OptionalColor
	if c.TextColor != "" {
		textColor, err = ParseTextColor(c.TextColor)
		if err != nil {
			return err
		}
	}

	reqs := []*docs.Request{
		{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: insertIndex},
				Text:     text,
			},
		},
	}

	// Apply text color to inserted text
	if textColor != nil {
		textEnd := insertIndex + utf16Len(text)
		reqs = append(reqs, BuildColorRequest(insertIndex, textEnd, textColor))
	}

	resp, err := svc.Documents.BatchUpdate(id, &docs.BatchUpdateDocumentRequest{Requests: reqs}).
		Context(ctx).
		Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": resp.DocumentId,
			"requests":   len(reqs),
			"index":      insertIndex,
		}
		if textColor != nil {
			payload["textColor"] = c.TextColor
		}
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("requests\t%d", len(reqs))
	u.Out().Printf("index\t%d", insertIndex)
	if textColor != nil {
		u.Out().Printf("textColor\t%s", c.TextColor)
	}
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

type DocsCatCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	MaxBytes int64  `name:"max-bytes" help:"Max bytes to read (0 = unlimited)" default:"2000000"`
	Tab      string `name:"tab" help:"Tab title or ID to read (omit for default behavior)"`
	AllTabs  bool   `name:"all-tabs" help:"Show all tabs with headers"`
	Raw      bool   `name:"raw" help:"Output the raw Google Docs API JSON response without modifications"`
	Numbered bool   `name:"numbered" short:"N" help:"Prefix each paragraph with its number"`
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
		if c.Numbered {
			return c.printNumbered(ctx, doc, c.Tab)
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

type DocsInsertCmd struct {
	DocID     string `arg:"" name:"docId" help:"Doc ID"`
	Content   string `arg:"" optional:"" name:"content" help:"Text to insert (or use --file / stdin)"`
	Index     int64  `name:"index" help:"Character index to insert at (1 = beginning)" default:"1"`
	File      string `name:"file" short:"f" help:"Read content from file (use - for stdin)"`
	TextColor string `name:"text-color" help:"Set text color (hex #RRGGBB or name: red, blue, green, ...)"`
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

	// Parse text color if specified
	var textColor *docs.OptionalColor
	if c.TextColor != "" {
		var colorErr error
		textColor, colorErr = ParseTextColor(c.TextColor)
		if colorErr != nil {
			return colorErr
		}
	}

	reqs := []*docs.Request{{
		InsertText: &docs.InsertTextRequest{
			Text: content,
			Location: &docs.Location{
				Index: c.Index,
			},
		},
	}}

	// Apply text color to inserted text
	if textColor != nil {
		textEnd := c.Index + utf16Len(content)
		reqs = append(reqs, BuildColorRequest(c.Index, textEnd, textColor))
	}

	result, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: reqs,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("inserting text: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": result.DocumentId,
			"inserted":   len(content),
			"atIndex":    c.Index,
		}
		if textColor != nil {
			payload["textColor"] = c.TextColor
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}

	u.Out().Printf("documentId\t%s", result.DocumentId)
	u.Out().Printf("inserted\t%d bytes", len(content))
	u.Out().Printf("atIndex\t%d", c.Index)
	if textColor != nil {
		u.Out().Printf("textColor\t%s", c.TextColor)
	}
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

// --- Structure / Numbered commands ---

// DocsStructureCmd displays document structure with numbered paragraphs.
type DocsStructureCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Tab   string `name:"tab" help:"Tab title or ID (omit for default)"`
}

func (c *DocsStructureCmd) Run(ctx context.Context, flags *RootFlags) error {
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

// printNumbered prints document content with [N] paragraph number prefixes.
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

type DocsColorCmd struct {
	DocID     string `arg:"" name:"docId" help:"Doc ID"`
	Find      string `arg:"" name:"find" help:"Text to find and color"`
	TextColor string `arg:"" name:"color" help:"Text color (hex #RRGGBB or name: red, blue, green, ...)"`
	MatchCase bool   `name:"match-case" help:"Case-sensitive matching" default:"true"`
	All       bool   `name:"all" help:"Color all occurrences (default: first only)"`
	Paragraph bool   `name:"paragraph" short:"P" help:"Color the entire paragraph(s) containing the match"`
}

func (c *DocsColorCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	color, err := ParseTextColor(c.TextColor)
	if err != nil {
		return fmt.Errorf("invalid color: %w", err)
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	// Get document structure to find text indices
	doc, err := svc.Documents.Get(docID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get document: %w", err)
	}

	// Walk structural elements to find the text
	type match struct {
		start int64
		end   int64
	}
	var matches []match

	findText := c.Find
	for _, elem := range doc.Body.Content {
		if elem.Paragraph == nil {
			continue
		}
		// Build full paragraph text and track element boundaries
		var paraText string
		var paraStart, paraEnd int64
		if len(elem.Paragraph.Elements) > 0 {
			paraStart = elem.Paragraph.Elements[0].StartIndex
			paraEnd = elem.Paragraph.Elements[len(elem.Paragraph.Elements)-1].EndIndex
		}
		for _, pe := range elem.Paragraph.Elements {
			if pe.TextRun != nil {
				paraText += pe.TextRun.Content
			}
		}

		// Search within the paragraph text
		searchIn := paraText
		searchFor := findText
		if !c.MatchCase {
			searchIn = strings.ToLower(searchIn)
			searchFor = strings.ToLower(searchFor)
		}

		offset := 0
		for {
			idx := strings.Index(searchIn[offset:], searchFor)
			if idx < 0 {
				break
			}
			if c.Paragraph {
				// Color the entire paragraph, excluding trailing newline
				end := paraEnd
				if len(paraText) > 0 && paraText[len(paraText)-1] == '\n' {
					end--
				}
				matches = append(matches, match{start: paraStart, end: end})
			} else {
				absIdx := offset + idx
				start := paraStart + int64(absIdx)
				end := start + int64(len(findText))
				matches = append(matches, match{start: start, end: end})
			}
			if !c.All {
				break
			}
			offset = offset + idx + len(searchFor)
			if c.Paragraph {
				break // one match per paragraph is enough
			}
		}
		if len(matches) > 0 && !c.All {
			break
		}
	}

	if len(matches) == 0 {
		return fmt.Errorf("text not found in document: %q", c.Find)
	}

	// Build color requests
	var reqs []*docs.Request
	for _, m := range matches {
		reqs = append(reqs, BuildColorRequest(m.start, m.end, color))
	}

	_, err = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: reqs,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("apply color: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId": docID,
			"find":       c.Find,
			"color":      c.TextColor,
			"matches":    len(matches),
		})
	}

	u.Out().Printf("documentId\t%s", docID)
	u.Out().Printf("find\t%s", c.Find)
	u.Out().Printf("color\t%s", c.TextColor)
	u.Out().Printf("matches\t%d", len(matches))
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

func resolveTextInput(text, file string, kctx *kong.Context, textFlag, fileFlag string) (string, bool, error) {
	file = strings.TrimSpace(file)
	textProvided := text != "" || flagProvided(kctx, textFlag)
	fileProvided := file != "" || flagProvided(kctx, fileFlag)
	if textProvided && fileProvided {
		return "", true, usage(fmt.Sprintf("use only one of --%s or --%s", textFlag, fileFlag))
	}
	if fileProvided {
		b, err := readTextInput(file)
		if err != nil {
			return "", true, err
		}
		return string(b), true, nil
	}
	if textProvided {
		return text, true, nil
	}
	return text, false, nil
}

func readTextInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	expanded, err := config.ExpandPath(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(expanded) //nolint:gosec // user-provided path
}

func docsDocumentEndIndex(doc *docs.Document) int64 {
	if doc == nil || doc.Body == nil {
		return 1
	}
	end := int64(1)
	for _, el := range doc.Body.Content {
		if el == nil {
			continue
		}
		if el.EndIndex > end {
			end = el.EndIndex
		}
	}
	return end
}

func docsAppendIndex(endIndex int64) int64 {
	if endIndex > 1 {
		return endIndex - 1
	}
	return 1
}

func isDocsNotFound(err error) bool {
	var apiErr *gapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == http.StatusNotFound
}
