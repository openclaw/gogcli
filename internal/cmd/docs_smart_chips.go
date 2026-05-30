package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsInsertPersonCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Email string `name:"email" required:"" help:"Email address for the person chip"`
	Index *int64 `name:"index" help:"Character index to insert at. Omit or use --at-end for end-of-doc."`
	AtEnd bool   `name:"at-end" help:"Insert at end-of-doc/tab (mutually exclusive with --index)"`
	Tab   string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type DocsInsertFileChipCmd struct {
	DocID  string `arg:"" name:"docId" help:"Doc ID"`
	FileID string `name:"file-id" required:"" help:"Drive file ID to insert as a smart chip"`
	Index  *int64 `name:"index" help:"Character index to insert at. Omit or use --at-end for end-of-doc."`
	AtEnd  bool   `name:"at-end" help:"Insert at end-of-doc/tab (mutually exclusive with --index)"`
	Tab    string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type DocsInsertDateChipCmd struct {
	DocID  string `arg:"" name:"docId" help:"Doc ID"`
	Date   string `name:"date" required:"" help:"Date to insert as YYYY-MM-DD"`
	Format string `name:"format" help:"Date display format: abbreviated|full|iso" default:"abbreviated"`
	Index  *int64 `name:"index" help:"Character index to insert at. Omit or use --at-end for end-of-doc."`
	AtEnd  bool   `name:"at-end" help:"Insert at end-of-doc/tab (mutually exclusive with --index)"`
	Tab    string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

const docsDateChipFormatFull = "full"

type docsInsertLocationFlags struct {
	docID string
	index *int64
	atEnd bool
	tab   string
}

func (c *DocsInsertPersonCmd) Run(ctx context.Context, flags *RootFlags) error {
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty --email")
	}
	req := &docs.Request{InsertPerson: &docs.InsertPersonRequest{
		PersonProperties: &docs.PersonProperties{Email: email},
	}}
	return runDocsSingleInsert(ctx, flags, "docs.insert-person", docsInsertLocationFlags{
		docID: c.DocID,
		index: c.Index,
		atEnd: c.AtEnd,
		tab:   c.Tab,
	}, req, map[string]any{"email": email})
}

func (c *DocsInsertFileChipCmd) Run(ctx context.Context, flags *RootFlags) error {
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty --file-id")
	}
	loc := docsInsertLocationFlags{
		docID: c.DocID,
		index: c.Index,
		atEnd: c.AtEnd,
		tab:   c.Tab,
	}
	if dryRunErr := dryRunDocsSingleInsert(ctx, flags, "docs.insert-file-chip", loc, map[string]any{"fileId": fileID}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	driveSvc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}
	file, err := driveSvc.Files.Get(fileID).
		SupportsAllDrives(true).
		Fields("id,name,mimeType,webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("get Drive file: %w", err)
	}
	uri := file.WebViewLink
	if uri == "" {
		uri = bestEffortWebURL("drive", fileID)
	}
	req := &docs.Request{InsertRichLink: &docs.InsertRichLinkRequest{
		RichLinkProperties: &docs.RichLinkProperties{
			Uri: uri,
		},
	}}
	return runDocsSingleInsert(ctx, flags, "docs.insert-file-chip", docsInsertLocationFlags{
		docID: c.DocID,
		index: c.Index,
		atEnd: c.AtEnd,
		tab:   c.Tab,
	}, req, map[string]any{"fileId": fileID, "title": file.Name})
}

func (c *DocsInsertDateChipCmd) Run(ctx context.Context, flags *RootFlags) error {
	date := strings.TrimSpace(c.Date)
	if date == "" {
		return usage("empty --date")
	}
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return usage("--date must be YYYY-MM-DD")
	}
	dateFormat, err := normalizeDocsDateChipFormat(c.Format)
	if err != nil {
		return err
	}
	req := &docs.Request{InsertDate: &docs.InsertDateRequest{
		DateElementProperties: &docs.DateElementProperties{
			DateFormat: dateFormat,
			TimeFormat: "TIME_FORMAT_DISABLED",
			Timestamp:  t.UTC().Format(time.RFC3339),
		},
	}}
	return runDocsSingleInsert(ctx, flags, "docs.insert-date-chip", docsInsertLocationFlags{
		docID: c.DocID,
		index: c.Index,
		atEnd: c.AtEnd,
		tab:   c.Tab,
	}, req, map[string]any{"date": date, "format": c.Format})
}

func normalizeDocsDateChipFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "abbreviated", "abbr":
		return "DATE_FORMAT_MONTH_DAY_YEAR_ABBREVIATED", nil
	case docsDateChipFormatFull:
		return "DATE_FORMAT_MONTH_DAY_FULL", nil
	case "iso", "iso8601":
		return "DATE_FORMAT_ISO8601", nil
	default:
		return "", usage("--format must be abbreviated, full, or iso")
	}
}

func runDocsSingleInsert(ctx context.Context, flags *RootFlags, action string, loc docsInsertLocationFlags, req *docs.Request, payload map[string]any) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(loc.docID)
	if docID == "" {
		return usage("empty docId")
	}
	if loc.atEnd && loc.index != nil {
		return usage("--at-end and --index are mutually exclusive")
	}
	if loc.index != nil && *loc.index < 1 {
		return usage("--index must be >= 1 (index 0 is reserved)")
	}

	if err := dryRunDocsSingleInsert(ctx, flags, action, loc, payload); err != nil {
		return err
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	insertIndex, tabID, err := resolveDocsInsertLocation(ctx, svc, docID, loc)
	if err != nil {
		return err
	}
	setDocsInsertRequestLocation(req, insertIndex, tabID)
	resp, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{Requests: []*docs.Request{req}}).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}
	if outfmt.IsJSON(ctx) {
		result := map[string]any{"documentId": resp.DocumentId, "atIndex": insertIndex, "inserted": true}
		if tabID != "" {
			result["tabId"] = tabID
		}
		for k, v := range payload {
			result[k] = v
		}
		return outfmt.WriteJSON(ctx, os.Stdout, result)
	}
	u.Out().Linef("documentId\t%s", resp.DocumentId)
	u.Out().Linef("atIndex\t%d", insertIndex)
	u.Out().Linef("inserted\ttrue")
	if tabID != "" {
		u.Out().Linef("tabId\t%s", tabID)
	}
	return nil
}

func dryRunDocsSingleInsert(ctx context.Context, flags *RootFlags, action string, loc docsInsertLocationFlags, payload map[string]any) error {
	docID := strings.TrimSpace(loc.docID)
	if docID == "" {
		return usage("empty docId")
	}
	if loc.atEnd && loc.index != nil {
		return usage("--at-end and --index are mutually exclusive")
	}
	if loc.index != nil && *loc.index < 1 {
		return usage("--index must be >= 1 (index 0 is reserved)")
	}
	dryRunPayload := map[string]any{"documentId": docID, "tab": loc.tab}
	for k, v := range payload {
		dryRunPayload[k] = v
	}
	if loc.atEnd || loc.index == nil {
		dryRunPayload["atIndex"] = docsAtIndexEnd
	} else {
		dryRunPayload["atIndex"] = *loc.index
	}
	return dryRunExit(ctx, flags, action, dryRunPayload)
}

func resolveDocsInsertLocation(ctx context.Context, svc *docs.Service, docID string, loc docsInsertLocationFlags) (int64, string, error) {
	if loc.atEnd || loc.index == nil {
		endIndex, tabID, err := docsTargetEndIndexAndTabID(ctx, svc, docID, loc.tab)
		if err != nil {
			return 0, "", err
		}
		return docsAppendIndex(endIndex), tabID, nil
	}
	tabID := ""
	if strings.TrimSpace(loc.tab) != "" {
		resolved, err := resolveDocsTabID(ctx, svc, docID, loc.tab)
		if err != nil {
			return 0, "", err
		}
		tabID = resolved
	}
	return *loc.index, tabID, nil
}

func setDocsInsertRequestLocation(req *docs.Request, index int64, tabID string) {
	location := &docs.Location{Index: index, TabId: tabID}
	switch {
	case req.InsertPerson != nil:
		req.InsertPerson.Location = location
	case req.InsertRichLink != nil:
		req.InsertRichLink.Location = location
	case req.InsertDate != nil:
		req.InsertDate.Location = location
	case req.InsertInlineImage != nil:
		req.InsertInlineImage.Location = location
	}
}
