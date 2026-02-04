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
	Export   DocsExportCmd   `cmd:"" name:"export" help:"Export a Google Doc (pdf|docx|txt)"`
	Info     DocsInfoCmd     `cmd:"" name:"info" help:"Get Google Doc metadata"`
	Create   DocsCreateCmd   `cmd:"" name:"create" help:"Create a Google Doc"`
	Copy     DocsCopyCmd     `cmd:"" name:"copy" help:"Copy a Google Doc"`
	Write    DocsWriteCmd    `cmd:"" name:"write" help:"Write content to a Google Doc"`
	Update   DocsUpdateCmd   `cmd:"" name:"update" help:"Insert text at a specific index in a Google Doc"`
	Cat      DocsCatCmd      `cmd:"" name:"cat" help:"Print a Google Doc as plain text"`
	Comments DocsCommentsCmd `cmd:"" name:"comments" help:"Manage comments on a Google Doc"`
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{
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

	svc, err := newDriveService(ctx, account)
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

	created, err := svc.Files.Create(f).
		SupportsAllDrives(true).
		Fields("id, name, mimeType, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if created == nil {
		return errors.New("create failed")
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{strFile: created})
	}

	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("name\t%s", created.Name)
	u.Out().Printf("mime\t%s", created.MimeType)
	if created.WebViewLink != "" {
		u.Out().Printf("link\t%s", created.WebViewLink)
	}
	return nil
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
	DocID  string `arg:"" name:"docId" help:"Doc ID"`
	Text   string `name:"text" help:"Text to write"`
	File   string `name:"file" help:"Text file path ('-' for stdin)"`
	Append bool   `name:"append" help:"Append instead of replacing the document body"`
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
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(os.Stdout, payload)
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("requests\t%d", len(reqs))
	u.Out().Printf("append\t%t", c.Append)
	u.Out().Printf("index\t%d", insertIndex)
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

type DocsUpdateCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Text  string `name:"text" help:"Text to insert"`
	File  string `name:"file" help:"Text file path ('-' for stdin)"`
	Index int64  `name:"index" help:"Insert index (default: end of document)"`
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

	reqs := []*docs.Request{
		{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: insertIndex},
				Text:     text,
			},
		},
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
		if resp.WriteControl != nil {
			payload["writeControl"] = resp.WriteControl
		}
		return outfmt.WriteJSON(os.Stdout, payload)
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("requests\t%d", len(reqs))
	u.Out().Printf("index\t%d", insertIndex)
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

type DocsCatCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	MaxBytes int64  `name:"max-bytes" help:"Max bytes to read (0 = unlimited)" default:"2000000"`
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"text": text})
	}
	_, err = io.WriteString(os.Stdout, text)
	return err
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
