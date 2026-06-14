package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docsedit"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsInsertPersonCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Email      string `name:"email" required:"" help:"Email address for the person chip"`
	Index      *int64 `name:"index" help:"Character index to insert at. Omit or use --at-end for end-of-doc."`
	At         string `name:"at" help:"Anchor by literal text, delete the match, and insert the person chip there"`
	Occurrence *int   `name:"occurrence" help:"Use the Nth --at match (1-based; required when --at is ambiguous)"`
	MatchCase  bool   `name:"match-case" help:"Use case-sensitive --at matching"`
	AtEnd      bool   `name:"at-end" help:"Insert at end-of-doc/tab (mutually exclusive with --index)"`
	Tab        string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	Batch      string `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
}

type DocsInsertFileChipCmd struct {
	DocID  string `arg:"" name:"docId" help:"Doc ID"`
	FileID string `name:"file-id" required:"" help:"Drive file ID to insert as a smart chip"`
	Index  *int64 `name:"index" help:"Character index to insert at. Omit or use --at-end for end-of-doc."`
	AtEnd  bool   `name:"at-end" help:"Insert at end-of-doc/tab (mutually exclusive with --index)"`
	Tab    string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	Batch  string `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
}

type DocsInsertDateChipCmd struct {
	DocID  string `arg:"" name:"docId" help:"Doc ID"`
	Date   string `name:"date" required:"" help:"Date to insert as YYYY-MM-DD"`
	Format string `name:"format" help:"Date display format: abbreviated|full|iso" default:"abbreviated"`
	Index  *int64 `name:"index" help:"Character index to insert at. Omit or use --at-end for end-of-doc."`
	AtEnd  bool   `name:"at-end" help:"Insert at end-of-doc/tab (mutually exclusive with --index)"`
	Tab    string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	Batch  string `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
}

const docsDateChipFormatFull = "full"

type docsInsertLocationFlags struct {
	docID      string
	index      *int64
	at         string
	atProvided bool
	occurrence *int
	matchCase  bool
	replaceAt  bool
	atEnd      bool
	tab        string
	batch      string
}

func (c *DocsInsertPersonCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty --email")
	}
	req := &docs.Request{InsertPerson: &docs.InsertPersonRequest{
		PersonProperties: &docs.PersonProperties{Email: email},
	}}
	return runDocsSingleInsert(ctx, flags, "docs.insert-person", docsInsertLocationFlags{
		docID:      c.DocID,
		index:      c.Index,
		at:         c.At,
		atProvided: flagProvided(kctx, "at"),
		occurrence: c.Occurrence,
		matchCase:  c.MatchCase,
		replaceAt:  true,
		atEnd:      c.AtEnd,
		tab:        c.Tab,
		batch:      c.Batch,
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
		batch: c.Batch,
	}
	if dryRunErr := dryRunDocsSingleInsert(ctx, flags, "docs.insert-file-chip", loc, map[string]any{"fileId": fileID}); dryRunErr != nil {
		return dryRunErr
	}
	if err := validateDocsBatchTarget(ctx, flags, c.Batch, c.DocID); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	driveSvc, err := driveService(ctx, account)
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
		batch: c.Batch,
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
		batch: c.Batch,
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
	placement, err := planDocsInsertLocation(loc)
	if err != nil {
		return err
	}
	if dryRunErr := dryRunDocsSingleInsert(ctx, flags, action, loc, payload); dryRunErr != nil {
		return dryRunErr
	}
	if batchErr := validateDocsBatchTarget(ctx, flags, loc.batch, docID); batchErr != nil {
		return batchErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	batchRevision, err := captureDocsBatchRevision(ctx, svc, loc.batch, docID)
	if err != nil {
		return err
	}
	resolvedPlacement, err := resolveDocsPlacement(ctx, svc, docID, loc.tab, placement)
	if err != nil {
		return err
	}
	insertIndex := resolvedPlacement.Index
	tabID := resolvedPlacement.TabID
	setDocsInsertRequestLocation(req, insertIndex, tabID)
	reqs := make([]*docs.Request, 0, 2)
	if loc.replaceAt && resolvedPlacement.Anchored {
		reqs = append(reqs, &docs.Request{DeleteContentRange: &docs.DeleteContentRangeRequest{
			Range: &docs.Range{
				StartIndex: resolvedPlacement.Range.Start,
				EndIndex:   resolvedPlacement.Range.End,
				TabId:      tabID,
			},
		}})
	}
	reqs = append(reqs, req)
	batchReq := &docs.BatchUpdateDocumentRequest{
		Requests:     reqs,
		WriteControl: docsRequiredRevisionWriteControl(resolvedPlacement.RequiredRevisionID),
	}
	if queued, queueErr := queueDocsBatchRequests(ctx, flags, loc.batch, docID, action, batchRevision, reqs, false); queued || queueErr != nil {
		return queueErr
	}
	resp, err := svc.Documents.BatchUpdate(docID, batchReq).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return err
	}
	if outfmt.IsJSON(ctx) {
		result := map[string]any{"documentId": resp.DocumentId, "atIndex": insertIndex, "inserted": true}
		if len(reqs) > 1 {
			result["requests"] = len(reqs)
			result["replaced"] = true
		}
		if tabID != "" {
			result["tabId"] = tabID
		}
		for k, v := range payload {
			result[k] = v
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}
	u.Out().Linef("documentId\t%s", resp.DocumentId)
	u.Out().Linef("atIndex\t%d", insertIndex)
	u.Out().Linef("inserted\ttrue")
	if len(reqs) > 1 {
		u.Out().Linef("requests\t%d", len(reqs))
		u.Out().Linef("replaced\ttrue")
	}
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
	placement, err := planDocsInsertLocation(loc)
	if err != nil {
		return err
	}
	dryRunPayload := map[string]any{"documentId": docID, "tab": loc.tab, "batch": loc.batch}
	for k, v := range payload {
		dryRunPayload[k] = v
	}
	switch placement.Kind {
	case docsedit.PlacementAnchor:
		dryRunPayload["atIndex"] = docsAtIndexAnchorStart
		addDocsAtAnchorDryRunPayload(dryRunPayload, docsAtAnchorFlags{At: placement.Anchor.Text, Occurrence: placement.Anchor.Occurrence, MatchCase: placement.Anchor.MatchCase})
		if loc.replaceAt {
			dryRunPayload["replaceAt"] = true
		}
	case docsedit.PlacementEnd:
		dryRunPayload["atIndex"] = docsAtIndexEnd
	case docsedit.PlacementIndex:
		dryRunPayload["atIndex"] = placement.Index
	}
	return dryRunExit(ctx, flags, action, dryRunPayload)
}

func planDocsInsertLocation(loc docsInsertLocationFlags) (docsedit.Placement, error) {
	placement, err := docsedit.PlanEndInsertPlacement(docsedit.EndInsertPlacementOptions{
		Index: loc.index,
		AtEnd: loc.atEnd,
		Anchor: docsedit.AnchorOptions{
			Text:       loc.at,
			Provided:   loc.atProvided,
			Occurrence: loc.occurrence,
			MatchCase:  loc.matchCase,
		},
	})
	if err != nil {
		return docsedit.Placement{}, usage(err.Error())
	}
	return placement, nil
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
