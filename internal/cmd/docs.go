package cmd

import (
	"context"
	"errors"
	"fmt"
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
	Export           DocsExportCmd           `cmd:"" name:"export" aliases:"download,dl" help:"Export a Google Doc (pdf|docx|txt|md|html)"`
	Info             DocsInfoCmd             `cmd:"" name:"info" aliases:"get,show" help:"Get Google Doc metadata"`
	Create           DocsCreateCmd           `cmd:"" name:"create" aliases:"add,new" help:"Create a Google Doc"`
	Copy             DocsCopyCmd             `cmd:"" name:"copy" aliases:"cp,duplicate" help:"Copy a Google Doc"`
	Cat              DocsCatCmd              `cmd:"" name:"cat" aliases:"text,read" help:"Print a Google Doc as plain text"`
	Comments         DocsCommentsCmd         `cmd:"" name:"comments" help:"Manage comments on files"`
	Tabs             DocsTabsCmd             `cmd:"" name:"tabs" help:"Manage Google Doc tabs"`
	AddTab           DocsAddTabCmd           `cmd:"" name:"add-tab" help:"Add a tab to a Google Doc"`
	RenameTab        DocsRenameTabCmd        `cmd:"" name:"rename-tab" help:"Rename a tab in a Google Doc"`
	DeleteTab        DocsDeleteTabCmd        `cmd:"" name:"delete-tab" help:"Delete a tab from a Google Doc"`
	ListTabs         DocsListTabsCmd         `cmd:"" name:"list-tabs" help:"List all tabs in a Google Doc"`
	Write            DocsWriteCmd            `cmd:"" name:"write" help:"Write content to a Google Doc"`
	Insert           DocsInsertCmd           `cmd:"" name:"insert" help:"Insert text at a specific position"`
	InsertTable      DocsInsertTableCmd      `cmd:"" name:"insert-table" help:"Insert a native table at a specific position (or end-of-doc with --at-end), optionally populated via --values-json"`
	CellUpdate       DocsCellUpdateCmd       `cmd:"" name:"cell-update" aliases:"update-cell" help:"Replace or append content inside a specific table cell"`
	CellStyle        DocsCellStyleCmd        `cmd:"" name:"cell-style" help:"Apply table cell background and text styling"`
	TableRow         DocsTableRowCmd         `cmd:"" name:"table-row" help:"Insert or delete native table rows"`
	TableColumn      DocsTableColumnCmd      `cmd:"" name:"table-column" help:"Insert or delete native table columns"`
	TableMerge       DocsTableMergeCmd       `cmd:"" name:"table-merge" help:"Merge a native table cell range"`
	TableUnmerge     DocsTableUnmergeCmd     `cmd:"" name:"table-unmerge" aliases:"table-split" help:"Unmerge the region containing a native table cell"`
	TableColumnWidth DocsTableColumnWidthCmd `cmd:"" name:"table-column-width" aliases:"table-width,column-width" help:"Set or reset native table column widths"`
	InsertImage      DocsInsertImageCmd      `cmd:"" name:"insert-image" help:"Upload a local image and insert it into a Google Doc"`
	InsertPerson     DocsInsertPersonCmd     `cmd:"" name:"insert-person" help:"Insert a native person smart chip"`
	InsertFileChip   DocsInsertFileChipCmd   `cmd:"" name:"insert-file-chip" aliases:"insert-rich-link" help:"Insert a native Drive file smart chip"`
	InsertDateChip   DocsInsertDateChipCmd   `cmd:"" name:"insert-date-chip" help:"Insert a native date smart chip"`
	InsertPageBreak  DocsInsertPageBreakCmd  `cmd:"" name:"insert-page-break" aliases:"page-break,pb" help:"Insert a page break at a specific position (or end-of-doc with --at-end)"`
	Delete           DocsDeleteCmd           `cmd:"" name:"delete" help:"Delete text range from document"`
	FindRange        DocsFindRangeCmd        `cmd:"" name:"find-range" help:"Find text and print Docs API UTF-16 index ranges"`
	FindReplace      DocsFindReplaceCmd      `cmd:"" name:"find-replace" help:"Find and replace text. Supports plain text or markdown with images; use --first for a single occurrence."`
	Update           DocsUpdateCmd           `cmd:"" name:"update" help:"Insert or replace text at a specific index or range in a Google Doc"`
	Edit             DocsEditCmd             `cmd:"" name:"edit" help:"Find and replace text in a Google Doc"`
	Format           DocsFormatCmd           `cmd:"" name:"format" help:"Apply text or paragraph formatting to a Google Doc"`
	Sed              DocsSedCmd              `cmd:"" name:"sed" help:"Regex find/replace (sed-style: s/pattern/replacement/g)"`
	Clear            DocsClearCmd            `cmd:"" name:"clear" help:"Clear all content from a Google Doc"`
	Structure        DocsStructureCmd        `cmd:"" name:"structure" aliases:"struct" help:"Show document structure with numbered paragraphs"`
	Tables           DocsTablesCmd           `cmd:"" name:"tables" help:"List native tables"`
	Images           DocsImagesCmd           `cmd:"" name:"images" help:"List document images"`
	Headings         DocsHeadingsCmd         `cmd:"" name:"headings" help:"List document headings"`
	Paragraphs       DocsParagraphsCmd       `cmd:"" name:"paragraphs" help:"List document paragraphs"`
	NamedRanges      DocsNamedRangesCmd      `cmd:"" name:"named-range" aliases:"named-ranges,namedranges,nr" help:"Manage named ranges"`
	Raw              DocsRawCmd              `cmd:"" name:"raw" help:"Dump raw Google Docs API response as JSON (Documents.Get; lossless; for scripting and LLM consumption)"`
	PageLayout       DocsPageLayoutCmd       `cmd:"" name:"page-layout" aliases:"set-page-layout,page-setup" help:"Set page layout (pageless|pages) on an existing Google Doc"`
}

type DocsTabsCmd struct {
	List   DocsListTabsCmd  `cmd:"" name:"list" aliases:"ls" help:"List all tabs in a Google Doc"`
	Add    DocsAddTabCmd    `cmd:"" name:"add" aliases:"create,new" help:"Add a tab to a Google Doc"`
	Rename DocsRenameTabCmd `cmd:"" name:"rename" aliases:"move" help:"Rename a tab in a Google Doc"`
	Delete DocsDeleteTabCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete a tab from a Google Doc"`
}

// DocsRawCmd dumps the full Documents.Get response as JSON, with no Fields
// restriction. Intended for programmatic / LLM consumption where the caller
// wants the canonical Google Docs API tree (tables, suggestions, per-run
// styling, list nesting, named ranges, inline objects) that `info` drops.
//
// REST reference: https://developers.google.com/docs/api/reference/rest/v1/documents/get
// Go type: https://pkg.go.dev/google.golang.org/api/docs/v1#Document
type DocsRawCmd struct {
	DocID   string `arg:"" name:"docId" help:"Doc ID"`
	Pretty  bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
	Tab     string `name:"tab" help:"Return one tab by title or ID in the legacy top-level Document shape"`
	AllTabs bool   `name:"all-tabs" help:"Return the canonical Document response with all tab content populated"`
}

func (c *DocsRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	if strings.TrimSpace(c.Tab) != "" && c.AllTabs {
		return usage("--tab and --all-tabs cannot be used together")
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	getCall := svc.Documents.Get(id).Context(ctx)
	if strings.TrimSpace(c.Tab) != "" || c.AllTabs {
		getCall = getCall.IncludeTabsContent(true)
	}
	doc, err := getCall.Do()
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

	if strings.TrimSpace(c.Tab) != "" {
		tab, tabErr := findTab(flattenTabs(doc.Tabs), c.Tab)
		if tabErr != nil {
			return tabErr
		}
		doc, err = projectRawDocumentTab(doc, tab)
		if err != nil {
			return err
		}
	}

	return writeRawJSON(ctx, doc, c.Pretty)
}

func projectRawDocumentTab(doc *docs.Document, tab *docs.Tab) (*docs.Document, error) {
	if tab == nil || tab.DocumentTab == nil {
		return nil, errors.New("selected tab has no document content")
	}

	content := tab.DocumentTab
	return &docs.Document{
		Body:                          content.Body,
		DocumentId:                    doc.DocumentId,
		DocumentStyle:                 content.DocumentStyle,
		Footers:                       content.Footers,
		Footnotes:                     content.Footnotes,
		Headers:                       content.Headers,
		InlineObjects:                 content.InlineObjects,
		Lists:                         content.Lists,
		NamedRanges:                   content.NamedRanges,
		NamedStyles:                   content.NamedStyles,
		PositionedObjects:             content.PositionedObjects,
		RevisionId:                    doc.RevisionId,
		SuggestedDocumentStyleChanges: content.SuggestedDocumentStyleChanges,
		SuggestedNamedStylesChanges:   content.SuggestedNamedStylesChanges,
		SuggestionsViewMode:           doc.SuggestionsViewMode,
		Title:                         doc.Title,
	}, nil
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

	u.Out().Linef("id\t%s", doc.DocumentId)
	u.Out().Linef("name\t%s", doc.Title)
	u.Out().Linef("mime\t%s", driveMimeGoogleDoc)
	if link := docsWebViewLink(doc.DocumentId); link != "" {
		u.Out().Linef("link\t%s", link)
	}
	if doc.RevisionId != "" {
		u.Out().Linef("revision\t%s", doc.RevisionId)
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

	u.Out().Linef("id\t%s", created.Id)
	u.Out().Linef("name\t%s", created.Name)
	u.Out().Linef("mime\t%s", created.MimeType)
	if created.WebViewLink != "" {
		u.Out().Linef("link\t%s", created.WebViewLink)
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
		Op:           "docs.copy",
		ArgName:      "docId",
		ExpectedMime: "application/vnd.google-apps.document",
		KindLabel:    "Google Doc",
	}, c.DocID, c.Title, c.Parent)
}
