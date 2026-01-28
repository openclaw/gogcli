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

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newDocsService = googleapi.NewDocs

type DocsCmd struct {
	Export  DocsExportCmd  `cmd:"" name:"export" help:"Export a Google Doc (pdf|docx|txt)"`
	Info    DocsInfoCmd    `cmd:"" name:"info" help:"Get Google Doc metadata"`
	Create  DocsCreateCmd  `cmd:"" name:"create" help:"Create a Google Doc"`
	Copy    DocsCopyCmd    `cmd:"" name:"copy" help:"Copy a Google Doc"`
	Cat     DocsCatCmd     `cmd:"" name:"cat" help:"Print a Google Doc as plain text"`
	Insert  DocsInsertCmd  `cmd:"" name:"insert" help:"Insert text into a Google Doc"`
	Replace DocsReplaceCmd `cmd:"" name:"replace" help:"Replace text in a Google Doc"`
	Update  DocsUpdateCmd  `cmd:"" name:"update" help:"Batch update a Google Doc (Docs API)"`
	Tabs    DocsTabsCmd    `cmd:"" name:"tabs" help:"Manage document tabs"`
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
	TabID string `name:"tab" help:"Tab ID (adds tab link/details)"`
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

	tabID := strings.TrimSpace(c.TabID)
	fields := "documentId,title,revisionId"
	if tabID != "" {
		fields = "documentId,title,revisionId,tabs"
	}
	doc, err := svc.Documents.Get(id).
		Fields(gapi.Field(fields)).
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

	var tabProps *docs.TabProperties
	if tabID != "" {
		tab := findDocsTab(doc.Tabs, tabID)
		if tab == nil || tab.TabProperties == nil {
			return fmt.Errorf("tab not found (id=%s)", tabID)
		}
		tabProps = tab.TabProperties
	}

	file := map[string]any{
		"id":       doc.DocumentId,
		"name":     doc.Title,
		"mimeType": driveMimeGoogleDoc,
	}
	if link := docsWebViewLink(doc.DocumentId, tabID); link != "" {
		file["webViewLink"] = link
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			strFile:    file,
			"document": doc,
		}
		if tabProps != nil {
			payload["tab"] = tabProps
		}
		return outfmt.WriteJSON(os.Stdout, payload)
	}

	u.Out().Printf("id\t%s", doc.DocumentId)
	u.Out().Printf("name\t%s", doc.Title)
	u.Out().Printf("mime\t%s", driveMimeGoogleDoc)
	if link := docsWebViewLink(doc.DocumentId, tabID); link != "" {
		u.Out().Printf("link\t%s", link)
	}
	if doc.RevisionId != "" {
		u.Out().Printf("revision\t%s", doc.RevisionId)
	}
	if tabProps != nil {
		u.Out().Printf("tab\t%s", tabProps.TabId)
		if tabProps.Title != "" {
			u.Out().Printf("tab-title\t%s", tabProps.Title)
		}
		if tabProps.ParentTabId != "" {
			u.Out().Printf("tab-parent\t%s", tabProps.ParentTabId)
		}
		if tabProps.IconEmoji != "" {
			u.Out().Printf("tab-icon\t%s", tabProps.IconEmoji)
		}
		u.Out().Printf("tab-index\t%d", tabProps.Index)
		u.Out().Printf("tab-level\t%d", tabProps.NestingLevel)
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

type DocsCatCmd struct {
	DocID    string `arg:"" name:"docId" help:"Doc ID"`
	MaxBytes int64  `name:"max-bytes" help:"Max bytes to read (0 = unlimited)" default:"2000000"`
	TabID    string `name:"tab" help:"Tab ID"`
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
	tabID := strings.TrimSpace(c.TabID)

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Documents.Get(id).Context(ctx)
	if tabID != "" {
		call = call.IncludeTabsContent(true)
	}
	doc, err := call.Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return err
	}
	if doc == nil {
		return errors.New("doc not found")
	}

	var text string
	if tabID != "" {
		tab := findDocsTab(doc.Tabs, tabID)
		if tab == nil || tab.DocumentTab == nil {
			return fmt.Errorf("tab not found (id=%s)", tabID)
		}
		text = docsTabPlainText(tab.DocumentTab, c.MaxBytes)
	} else {
		text = docsPlainText(doc, c.MaxBytes)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"text": text})
	}
	_, err = io.WriteString(os.Stdout, text)
	return err
}

type DocsInsertCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Text  string `name:"text" help:"Text to insert" required:""`
	Index *int64 `name:"index" help:"Zero-based UTF-16 index"`
	End   bool   `name:"end" help:"Insert at the end of the segment"`
	TabID string `name:"tab" help:"Tab ID"`
}

func (c *DocsInsertCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	if c.End && c.Index != nil {
		return usage("use only one of --index or --end")
	}
	if !c.End && c.Index == nil {
		return usage("missing --index (or use --end)")
	}

	tabID := strings.TrimSpace(c.TabID)
	insert := &docs.InsertTextRequest{
		Text: c.Text,
	}
	if c.End {
		insert.EndOfSegmentLocation = &docs.EndOfSegmentLocation{
			TabId: tabID,
		}
	} else {
		loc := &docs.Location{
			Index: *c.Index,
			TabId: tabID,
		}
		loc.ForceSendFields = append(loc.ForceSendFields, "Index")
		insert.Location = loc
	}

	req := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{
			{
				InsertText: insert,
			},
		},
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Documents.BatchUpdate(id, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		out := map[string]any{
			"documentId": resp.DocumentId,
			"text":       c.Text,
		}
		if c.Index != nil {
			out["index"] = *c.Index
		}
		if c.End {
			out["end"] = true
		}
		if tabID != "" {
			out["tabId"] = tabID
		}
		return outfmt.WriteJSON(os.Stdout, out)
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("inserted\t%d", len(c.Text))
	if tabID != "" {
		u.Out().Printf("tab\t%s", tabID)
	}
	if c.Index != nil {
		u.Out().Printf("index\t%d", *c.Index)
	}
	if c.End {
		u.Out().Printf("end\ttrue")
	}
	return nil
}

type DocsReplaceCmd struct {
	DocID     string `arg:"" name:"docId" help:"Doc ID"`
	Match     string `name:"match" help:"Text to match" required:""`
	Replace   string `name:"replace" help:"Replacement text" required:""`
	MatchCase bool   `name:"match-case" help:"Match case"`
	TabID     string `name:"tab" help:"Tab ID"`
}

func (c *DocsReplaceCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	tabID := strings.TrimSpace(c.TabID)
	req := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{
			{
				ReplaceAllText: &docs.ReplaceAllTextRequest{
					ContainsText: &docs.SubstringMatchCriteria{
						Text:      c.Match,
						MatchCase: c.MatchCase,
					},
					ReplaceText: c.Replace,
					TabsCriteria: func() *docs.TabsCriteria {
						if tabID == "" {
							return nil
						}
						return &docs.TabsCriteria{TabIds: []string{tabID}}
					}(),
				},
			},
		},
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Documents.BatchUpdate(id, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	var occurrences int64
	if len(resp.Replies) > 0 && resp.Replies[0].ReplaceAllText != nil {
		occurrences = resp.Replies[0].ReplaceAllText.OccurrencesChanged
	}

	if outfmt.IsJSON(ctx) {
		out := map[string]any{
			"documentId":         resp.DocumentId,
			"occurrencesChanged": occurrences,
		}
		if tabID != "" {
			out["tabId"] = tabID
		}
		return outfmt.WriteJSON(os.Stdout, out)
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("replaced\t%d", occurrences)
	if tabID != "" {
		u.Out().Printf("tab\t%s", tabID)
	}
	return nil
}

func docsWebViewLink(id, tabID string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	link := "https://docs.google.com/document/d/" + id + "/edit"
	tabID = strings.TrimSpace(tabID)
	if tabID != "" {
		link += "?tab=" + tabID
	}
	return link
}

func docsPlainText(doc *docs.Document, maxBytes int64) string {
	if doc == nil {
		return ""
	}
	return docsBodyPlainText(doc.Body, maxBytes)
}

func docsTabPlainText(tab *docs.DocumentTab, maxBytes int64) string {
	if tab == nil {
		return ""
	}
	return docsBodyPlainText(tab.Body, maxBytes)
}

func docsBodyPlainText(body *docs.Body, maxBytes int64) string {
	if body == nil {
		return ""
	}
	var buf bytes.Buffer
	for _, el := range body.Content {
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

func isDocsNotFound(err error) bool {
	var apiErr *gapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == http.StatusNotFound
}

type DocsUpdateCmd struct {
	DocID        string `arg:"" name:"docId" help:"Doc ID"`
	RequestsJSON string `name:"requests-json" help:"Batch update requests as JSON array"`
	RequestsFile string `name:"requests-file" help:"Requests JSON file path ('-' for stdin)"`
	BodyJSON     string `name:"body-json" help:"Full batchUpdate request JSON"`
	BodyFile     string `name:"body-file" help:"BatchUpdate request JSON file path ('-' for stdin)"`
	RequiredRev  string `name:"required-revision" help:"Require this revision ID"`
	TargetRev    string `name:"target-revision" help:"Target this revision ID"`
}

func (c *DocsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}

	bodyJSON := strings.TrimSpace(c.BodyJSON)
	bodyFile := strings.TrimSpace(c.BodyFile)
	reqJSON := strings.TrimSpace(c.RequestsJSON)
	reqFile := strings.TrimSpace(c.RequestsFile)

	if (bodyJSON != "" || bodyFile != "") && (reqJSON != "" || reqFile != "") {
		return usage("use either --body-json/--body-file or --requests-json/--requests-file")
	}

	var req docs.BatchUpdateDocumentRequest
	switch {
	case bodyJSON != "" || bodyFile != "":
		if strings.TrimSpace(c.RequiredRev) != "" || strings.TrimSpace(c.TargetRev) != "" {
			return usage("use only one of --body-* or revision flags")
		}
		var raw string
		raw, err = readJSONInput(bodyJSON, bodyFile, "body")
		if err != nil {
			return err
		}
		if err = json.Unmarshal([]byte(raw), &req); err != nil {
			return fmt.Errorf("invalid batchUpdate JSON: %w", err)
		}
	default:
		var raw string
		raw, err = readJSONInput(reqJSON, reqFile, "requests")
		if err != nil {
			return err
		}
		var requests []*docs.Request
		if err = json.Unmarshal([]byte(raw), &requests); err != nil {
			return fmt.Errorf("invalid requests JSON: %w", err)
		}
		req.Requests = requests
		if strings.TrimSpace(c.RequiredRev) != "" || strings.TrimSpace(c.TargetRev) != "" {
			req.WriteControl = &docs.WriteControl{
				RequiredRevisionId: strings.TrimSpace(c.RequiredRev),
				TargetRevisionId:   strings.TrimSpace(c.TargetRev),
			}
		}
	}

	if len(req.Requests) == 0 {
		return fmt.Errorf("no requests provided")
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Documents.BatchUpdate(id, &req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"documentId":   resp.DocumentId,
			"replies":      resp.Replies,
			"writeControl": resp.WriteControl,
		})
	}

	u.Out().Printf("id\t%s", resp.DocumentId)
	u.Out().Printf("requests\t%d", len(req.Requests))
	if resp.WriteControl != nil && resp.WriteControl.RequiredRevisionId != "" {
		u.Out().Printf("revision\t%s", resp.WriteControl.RequiredRevisionId)
	}
	return nil
}

type DocsTabsCmd struct {
	List   DocsTabsListCmd   `cmd:"" name:"list" help:"List document tabs"`
	Add    DocsTabsAddCmd    `cmd:"" name:"add" help:"Add a document tab"`
	Update DocsTabsUpdateCmd `cmd:"" name:"update" help:"Update a document tab" aliases:"rename,move"`
	Delete DocsTabsDeleteCmd `cmd:"" name:"delete" help:"Delete a document tab" aliases:"rm,del"`
}

type DocsTabsListCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
}

func (c *DocsTabsListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
		Fields("documentId,title,tabs").
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

	tabs := flattenDocsTabs(doc.Tabs)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"documentId": doc.DocumentId,
			"tabs":       tabs,
			"tabCount":   len(tabs),
		})
	}

	if len(tabs) == 0 {
		u.Err().Println("No tabs")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tPARENT\tINDEX\tLEVEL\tICON")
	for _, tab := range tabs {
		parent := tab.ParentID
		if parent == "" {
			parent = "-"
		}
		icon := tab.IconEmoji
		if icon == "" {
			icon = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\n", tab.ID, tab.Title, parent, tab.Index, tab.NestingLevel, icon)
	}
	return nil
}

type DocsTabsAddCmd struct {
	DocID  string `arg:"" name:"docId" help:"Doc ID"`
	Title  string `arg:"" name:"title" help:"Tab title"`
	Parent string `name:"parent" help:"Parent tab ID"`
	Index  *int64 `name:"index" help:"Zero-based tab index"`
	Emoji  string `name:"emoji" help:"Tab emoji icon"`
}

func (c *DocsTabsAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	props := &docs.TabProperties{
		Title: title,
	}
	if parent := strings.TrimSpace(c.Parent); parent != "" {
		props.ParentTabId = parent
	}
	if emoji := strings.TrimSpace(c.Emoji); emoji != "" {
		props.IconEmoji = emoji
	}
	if c.Index != nil {
		props.Index = *c.Index
		props.ForceSendFields = append(props.ForceSendFields, "Index")
	}

	req := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{
			{
				AddDocumentTab: &docs.AddDocumentTabRequest{
					TabProperties: props,
				},
			},
		},
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Documents.BatchUpdate(id, req).Context(ctx).Do()
	if err != nil {
		return err
	}
	var tabProps *docs.TabProperties
	if len(resp.Replies) > 0 && resp.Replies[0].AddDocumentTab != nil {
		tabProps = resp.Replies[0].AddDocumentTab.TabProperties
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"documentId": resp.DocumentId,
			"tab":        tabProps,
		})
	}

	if tabProps != nil {
		u.Out().Printf("tab\t%s", tabProps.TabId)
		u.Out().Printf("title\t%s", tabProps.Title)
		if tabProps.ParentTabId != "" {
			u.Out().Printf("parent\t%s", tabProps.ParentTabId)
		}
		u.Out().Printf("index\t%d", tabProps.Index)
		if tabProps.IconEmoji != "" {
			u.Out().Printf("icon\t%s", tabProps.IconEmoji)
		}
		return nil
	}
	u.Out().Printf("updated\ttrue")
	return nil
}

type DocsTabsUpdateCmd struct {
	DocID  string  `arg:"" name:"docId" help:"Doc ID"`
	TabID  string  `arg:"" name:"tabId" help:"Tab ID"`
	Title  *string `name:"title" help:"New tab title"`
	Parent *string `name:"parent" help:"New parent tab ID (empty = root)"`
	Index  *int64  `name:"index" help:"Zero-based tab index"`
	Emoji  *string `name:"emoji" help:"New tab emoji icon"`
}

func (c *DocsTabsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	tabID := strings.TrimSpace(c.TabID)
	if tabID == "" {
		return usage("empty tabId")
	}

	props := &docs.TabProperties{
		TabId: tabID,
	}
	fields := make([]string, 0, 4)
	if c.Title != nil {
		props.Title = *c.Title
		props.ForceSendFields = append(props.ForceSendFields, "Title")
		fields = append(fields, "title")
	}
	if c.Parent != nil {
		props.ParentTabId = *c.Parent
		props.ForceSendFields = append(props.ForceSendFields, "ParentTabId")
		fields = append(fields, "parentTabId")
	}
	if c.Index != nil {
		props.Index = *c.Index
		props.ForceSendFields = append(props.ForceSendFields, "Index")
		fields = append(fields, "index")
	}
	if c.Emoji != nil {
		props.IconEmoji = *c.Emoji
		props.ForceSendFields = append(props.ForceSendFields, "IconEmoji")
		fields = append(fields, "iconEmoji")
	}
	if len(fields) == 0 {
		return usage("no fields to update (set at least one of: --title, --parent, --index, --emoji)")
	}

	req := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{
			{
				UpdateDocumentTabProperties: &docs.UpdateDocumentTabPropertiesRequest{
					TabProperties: props,
					Fields:        strings.Join(fields, ","),
				},
			},
		},
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Documents.BatchUpdate(id, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"documentId": resp.DocumentId,
			"tabId":      tabID,
			"fields":     fields,
		})
	}

	u.Out().Printf("tab\t%s", tabID)
	u.Out().Printf("fields\t%s", strings.Join(fields, ","))
	return nil
}

type DocsTabsDeleteCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	TabID string `arg:"" name:"tabId" help:"Tab ID"`
}

func (c *DocsTabsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	tabID := strings.TrimSpace(c.TabID)
	if tabID == "" {
		return usage("empty tabId")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete tab %s from doc %s", tabID, id)); confirmErr != nil {
		return confirmErr
	}

	req := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{
			{
				DeleteTab: &docs.DeleteTabRequest{
					TabId: tabID,
				},
			},
		},
	}

	svc, err := newDocsService(ctx, account)
	if err != nil {
		return err
	}

	if _, err := svc.Documents.BatchUpdate(id, req).Context(ctx).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted":    true,
			"documentId": id,
			"tabId":      tabID,
		})
	}

	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("tab\t%s", tabID)
	return nil
}

type docsTabInfo struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	ParentID     string `json:"parentId,omitempty"`
	Index        int64  `json:"index"`
	NestingLevel int64  `json:"nestingLevel"`
	IconEmoji    string `json:"iconEmoji,omitempty"`
}

func flattenDocsTabs(tabs []*docs.Tab) []docsTabInfo {
	var out []docsTabInfo
	var walk func(items []*docs.Tab)
	walk = func(items []*docs.Tab) {
		for _, tab := range items {
			if tab == nil || tab.TabProperties == nil {
				continue
			}
			props := tab.TabProperties
			out = append(out, docsTabInfo{
				ID:           props.TabId,
				Title:        props.Title,
				ParentID:     props.ParentTabId,
				Index:        props.Index,
				NestingLevel: props.NestingLevel,
				IconEmoji:    props.IconEmoji,
			})
			if len(tab.ChildTabs) > 0 {
				walk(tab.ChildTabs)
			}
		}
	}
	walk(tabs)
	return out
}

func findDocsTab(tabs []*docs.Tab, tabID string) *docs.Tab {
	tabID = strings.TrimSpace(tabID)
	if tabID == "" {
		return nil
	}
	for _, tab := range tabs {
		if tab == nil || tab.TabProperties == nil {
			continue
		}
		if tab.TabProperties.TabId == tabID {
			return tab
		}
		if found := findDocsTab(tab.ChildTabs, tabID); found != nil {
			return found
		}
	}
	return nil
}

func readJSONInput(raw, path, label string) (string, error) {
	raw = strings.TrimSpace(raw)
	path = strings.TrimSpace(path)
	if raw == "" && path == "" {
		return "", usagef("provide %s via --%s-json or --%s-file", label, label, label)
	}
	if raw != "" && path != "" {
		return "", usagef("use only one of --%s-json or --%s-file", label, label)
	}
	if path == "" {
		return raw, nil
	}

	var (
		b   []byte
		err error
	)
	if path == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		path, err = config.ExpandPath(path)
		if err != nil {
			return "", err
		}
		b, err = os.ReadFile(path) //nolint:gosec // user-provided path
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}
