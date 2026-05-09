package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newDocsService = googleapi.NewDocs

type DocsCmd struct {
	Export      DocsExportCmd      `cmd:"" name:"export" aliases:"download,dl" help:"Export a Google Doc (pdf|docx|txt|md|html)"`
	Info        DocsInfoCmd        `cmd:"" name:"info" aliases:"get,show" help:"Get Google Doc metadata"`
	Create      DocsCreateCmd      `cmd:"" name:"create" aliases:"add,new" help:"Create a Google Doc"`
	Copy        DocsCopyCmd        `cmd:"" name:"copy" aliases:"cp,duplicate" help:"Copy a Google Doc"`
	Cat         DocsCatCmd         `cmd:"" name:"cat" aliases:"text,read" help:"Print a Google Doc as plain text"`
	Comments    DocsCommentsCmd    `cmd:"" name:"comments" help:"Manage comments on files"`
	AddTab      DocsAddTabCmd      `cmd:"" name:"add-tab" help:"Add a tab to a Google Doc"`
	RenameTab   DocsRenameTabCmd   `cmd:"" name:"rename-tab" help:"Rename a tab in a Google Doc"`
	DeleteTab   DocsDeleteTabCmd   `cmd:"" name:"delete-tab" help:"Delete a tab from a Google Doc"`
	ListTabs    DocsListTabsCmd    `cmd:"" name:"list-tabs" help:"List all tabs in a Google Doc"`
	Write       DocsWriteCmd       `cmd:"" name:"write" help:"Write content to a Google Doc"`
	Insert      DocsInsertCmd      `cmd:"" name:"insert" help:"Insert text at a specific position"`
	Delete      DocsDeleteCmd      `cmd:"" name:"delete" help:"Delete text range from document"`
	FindReplace DocsFindReplaceCmd `cmd:"" name:"find-replace" help:"Find and replace text. Supports plain text or markdown with images; use --first for a single occurrence."`
	Update      DocsUpdateCmd      `cmd:"" name:"update" help:"Insert text at a specific index in a Google Doc"`
	Edit        DocsEditCmd        `cmd:"" name:"edit" help:"Find and replace text in a Google Doc"`
	Format      DocsFormatCmd      `cmd:"" name:"format" help:"Apply text or paragraph formatting to a Google Doc"`
	Sed         DocsSedCmd         `cmd:"" name:"sed" help:"Regex find/replace (sed-style: s/pattern/replacement/g)"`
	Clear       DocsClearCmd       `cmd:"" name:"clear" help:"Clear all content from a Google Doc"`
	Structure   DocsStructureCmd   `cmd:"" name:"structure" aliases:"struct" help:"Show document structure with numbered paragraphs"`
	Raw         DocsRawCmd         `cmd:"" name:"raw" help:"Dump raw Google Docs API response as JSON (Documents.Get; lossless; for scripting and LLM consumption)"`
}

// DocsRawCmd dumps the full Documents.Get response as JSON, with no Fields
// restriction. Intended for programmatic / LLM consumption where the caller
// wants the canonical Google Docs API tree (tables, suggestions, per-run
// styling, list nesting, named ranges, inline objects) that `info` drops.
//
// REST reference: https://developers.google.com/docs/api/reference/rest/v1/documents/get
// Go type: https://pkg.go.dev/google.golang.org/api/docs/v1#Document
type DocsRawCmd struct {
	DocID  string `arg:"" name:"docId" help:"Doc ID"`
	Pretty bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *DocsRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
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
	doc, err = requireRawResponse(doc, "doc not found")
	if err != nil {
		return err
	}

	return writeRawJSON(ctx, doc, c.Pretty)
}

type DocsExportCmd struct {
	DocID  string         `arg:"" name:"docId" help:"Doc ID"`
	Output OutputPathFlag `embed:""`
	Format string         `name:"format" help:"Export format: pdf|docx|txt|md|html" default:"pdf"`
	Tab    string         `name:"tab" help:"(experimental) Export a specific tab by title or ID (see 'gog docs list-tabs')"`
}

func (c *DocsExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	if tab := strings.TrimSpace(c.Tab); tab != "" {
		return runDocsTabExport(ctx, flags, tabExportParams{
			DocID:    c.DocID,
			OutFlag:  c.Output.Path,
			Format:   c.Format,
			TabQuery: tab,
		})
	}
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
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	svc, err := requireDocsService(ctx, flags)
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
	Title    string `arg:"" name:"title" help:"Doc title"`
	Parent   string `name:"parent" help:"Destination folder ID"`
	File     string `name:"file" help:"Markdown file to import. Supports inline images from public HTTPS URLs via ![alt](url); append {width=N height=N} to control size in points." type:"existingfile"`
	Pageless bool   `name:"pageless" help:"Set document to pageless mode"`
}

func (c *DocsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	f := &drive.File{
		Name:     title,
		MimeType: "application/vnd.google-apps.document",
	}
	parent := strings.TrimSpace(c.Parent)
	if parent != "" {
		f.Parents = []string{parent}
	}

	if err := dryRunExit(ctx, flags, "docs.create", map[string]any{
		strFile:      f,
		"sourceFile": c.File,
		"parent":     parent,
		"pageless":   c.Pageless,
	}); err != nil {
		return err
	}

	account, driveSvc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
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
	if c.Pageless {
		docsSvc, svcErr := newDocsService(ctx, account)
		if svcErr != nil {
			return svcErr
		}
		if err := setDocumentPageless(ctx, docsSvc, created.Id); err != nil {
			return fmt.Errorf("set pageless mode: %w", err)
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
	return insertImagesIntoDocs(ctx, svc, docID, images, "")
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
