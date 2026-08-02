package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// linkRun is one styled segment of a cell's text. A run with an empty Uri is
// plain (unlinked) text; runs with a Uri become a hyperlinked segment.
type linkRun struct {
	Text string `json:"text"`
	Uri  string `json:"uri"`
}

// linkCellSpec is one cell to write, from --cells-json or the positional form.
// Provide either Runs (multi-link rich text) or URL[+Text] (single link).
type linkCellSpec struct {
	Cell string    `json:"cell"`
	URL  string    `json:"url"`
	Text string    `json:"text"`
	Runs []linkRun `json:"runs"`
}

// resolvedCell is a spec turned into the concrete payload to send.
type resolvedCell struct {
	a1   string
	text string
	runs []*sheets.TextFormatRun
	uris []string
}

type SheetsLinksSetCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Cell          string `arg:"" optional:"" name:"cell" help:"Target cell (eg. Sheet1!B2). Omit when using --cells-json."`
	URL           string `arg:"" optional:"" name:"url" help:"URL to link to."`
	Text          string `arg:"" optional:"" name:"text" help:"Display text (defaults to the URL)."`

	RunsJSON  string `name:"runs-json" help:"Multi-link cell: JSON array of runs, eg. [{\"text\":\"Act A\",\"uri\":\"https://a\"},{\"text\":\" / \"},{\"text\":\"Act B\",\"uri\":\"https://b\"}]. A run with an empty uri is plain text."`
	CellsJSON string `name:"cells-json" help:"Batch: JSON array, @file, or @- for stdin containing {cell,url,text} or {cell,runs:[{text,uri}]} objects, written in one request."`
}

func (c *SheetsLinksSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	specs, err := c.collectSpecs(ctx)
	if err != nil {
		return err
	}

	// Resolve each spec into text + runs, validating cells (single cell each)
	// before any network call so dry-run reports the real plan.
	resolved := make([]resolvedCell, 0, len(specs))
	for i, s := range specs {
		rc, rcErr := resolveLinkCell(s, i)
		if rcErr != nil {
			return rcErr
		}
		resolved = append(resolved, rc)
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.links.set", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"cells":          summarizeResolved(resolved),
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}
	sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}

	requests := make([]*sheets.Request, 0, len(resolved))
	for _, rc := range resolved {
		parsed, perr := parseSheetRange(rc.a1, "links")
		if perr != nil {
			return perr
		}
		grid, gerr := gridRangeFromMap(parsed, sheetIDs, "links")
		if gerr != nil {
			return gerr
		}
		start := &sheets.GridCoordinate{
			SheetId:         grid.SheetId,
			RowIndex:        grid.StartRowIndex,
			ColumnIndex:     grid.StartColumnIndex,
			ForceSendFields: []string{"SheetId", "RowIndex", "ColumnIndex"},
		}
		text := rc.text
		cell := &sheets.CellData{
			UserEnteredValue: &sheets.ExtendedValue{
				StringValue:     &text,
				ForceSendFields: []string{"StringValue"},
			},
			TextFormatRuns: rc.runs,
		}
		requests = append(requests, &sheets.Request{
			UpdateCells: &sheets.UpdateCellsRequest{
				Start: start,
				Rows:  []*sheets.RowData{{Values: []*sheets.CellData{cell}}},
				// Also clear any pre-existing whole-cell hyperlink
				// (userEnteredFormat.textFormat.link): the cell above leaves
				// that field unset, so listing it in the mask resets it to none.
				// Without this, a cell that previously held a whole-cell link
				// would keep the stale URL alongside the new runs and `links
				// get` would still report it — the write wouldn't round-trip.
				Fields: "userEnteredValue,textFormatRuns,userEnteredFormat.textFormat.link",
			},
		})
	}

	batchReq := &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}
	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, batchReq).Do(); err != nil {
		return fmt.Errorf("set links: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"cellsUpdated":  len(resolved),
			"cells":         summarizeResolved(resolved),
		})
	}

	if len(resolved) == 1 {
		u.Out().Linef("Set links on %s", resolved[0].a1)
	} else {
		u.Out().Linef("Set links on %d cells", len(resolved))
	}
	return nil
}

// collectSpecs builds the list of cells to write from either --cells-json
// (batch) or the positional/single-cell form, rejecting a mix of the two.
func (c *SheetsLinksSetCmd) collectSpecs(ctx context.Context) ([]linkCellSpec, error) {
	if strings.TrimSpace(c.CellsJSON) != "" {
		if c.Cell != "" || c.URL != "" || c.Text != "" || c.RunsJSON != "" {
			return nil, usage("use either positional cell args or --cells-json, not both")
		}
		b, err := resolveInlineOrFileBytes(c.CellsJSON, stdinReader(ctx))
		if err != nil {
			return nil, usagef("read --cells-json: %v", err)
		}
		var specs []linkCellSpec
		if err := json.Unmarshal(b, &specs); err != nil {
			return nil, usagef("parse --cells-json: %v", err)
		}
		if len(specs) == 0 {
			return nil, usage("--cells-json is empty")
		}
		return specs, nil
	}

	if strings.TrimSpace(c.Cell) == "" {
		return nil, usage("provide a target cell (or use --cells-json)")
	}
	spec := linkCellSpec{Cell: c.Cell, URL: c.URL, Text: c.Text}
	if strings.TrimSpace(c.RunsJSON) != "" {
		if c.URL != "" {
			return nil, usage("use either url/text or --runs-json, not both")
		}
		if err := json.Unmarshal([]byte(c.RunsJSON), &spec.Runs); err != nil {
			return nil, usagef("parse --runs-json: %v", err)
		}
	}
	return []linkCellSpec{spec}, nil
}

// resolveLinkCell turns a spec into the concrete cell text + text-format runs.
func resolveLinkCell(s linkCellSpec, idx int) (resolvedCell, error) {
	cellA1 := cleanRange(s.Cell)
	if strings.TrimSpace(cellA1) == "" {
		return resolvedCell{}, usagef("entry %d: empty cell", idx)
	}
	// Each write targets exactly one cell.
	parsed, err := parseSheetRange(cellA1, "links")
	if err != nil {
		return resolvedCell{}, err
	}
	if parsed.StartRow != parsed.EndRow || parsed.StartCol != parsed.EndCol ||
		parsed.StartRow == 0 || parsed.StartCol == 0 {
		return resolvedCell{}, usagef("links set targets a single cell, got %q", cellA1)
	}

	if len(s.Runs) > 0 {
		var sb strings.Builder
		runs := make([]*sheets.TextFormatRun, 0, len(s.Runs))
		uris := make([]string, 0, len(s.Runs))
		offset := 0
		for _, r := range s.Runs {
			run := &sheets.TextFormatRun{
				StartIndex:      int64(offset),
				Format:          &sheets.TextFormat{},
				ForceSendFields: []string{"StartIndex"},
			}
			if strings.TrimSpace(r.Uri) != "" {
				run.Format.Link = &sheets.Link{Uri: r.Uri}
				uris = append(uris, r.Uri)
			}
			runs = append(runs, run)
			sb.WriteString(r.Text)
			offset += len(utf16.Encode([]rune(r.Text)))
		}
		text := sb.String()
		if text == "" {
			return resolvedCell{}, usagef("entry %d (%s): runs produce empty text", idx, cellA1)
		}
		return resolvedCell{a1: cellA1, text: text, runs: runs, uris: uris}, nil
	}

	url := strings.TrimSpace(s.URL)
	if url == "" {
		return resolvedCell{}, usagef("entry %d (%s): provide url (or runs)", idx, cellA1)
	}
	text := s.Text
	if text == "" {
		text = url
	}
	run := &sheets.TextFormatRun{
		StartIndex:      0,
		Format:          &sheets.TextFormat{Link: &sheets.Link{Uri: url}},
		ForceSendFields: []string{"StartIndex"},
	}
	return resolvedCell{a1: cellA1, text: text, runs: []*sheets.TextFormatRun{run}, uris: []string{url}}, nil
}

func summarizeResolved(resolved []resolvedCell) []map[string]any {
	out := make([]map[string]any, 0, len(resolved))
	for _, rc := range resolved {
		out = append(out, map[string]any{
			"cell": rc.a1,
			"text": rc.text,
			"uris": rc.uris,
		})
	}
	return out
}
