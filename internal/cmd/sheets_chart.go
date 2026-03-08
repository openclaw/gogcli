package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsChartCmd struct {
	List   SheetsChartListCmd   `cmd:"" default:"withargs" help:"List charts in a spreadsheet"`
	Get    SheetsChartGetCmd    `cmd:"" name:"get" aliases:"show,info" help:"Get full chart definition (spec + position)"`
	Create SheetsChartCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a chart from a JSON spec"`
	Update SheetsChartUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update a chart spec"`
	Delete SheetsChartDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete a chart"`
}

// ---------- list ----------

type SheetsChartListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
}

func (c *SheetsChartListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(charts(chartId,spec(title,basicChart(chartType)),position(overlayPosition(anchorCell))),properties(sheetId,title))").
		Do()
	if err != nil {
		return err
	}

	type chartItem struct {
		ChartID    int64  `json:"chartId"`
		Title      string `json:"title"`
		Type       string `json:"type"`
		SheetID    int64  `json:"sheetId"`
		SheetTitle string `json:"sheetTitle"`
	}

	var items []chartItem
	for _, sheet := range resp.Sheets {
		sheetTitle := ""
		var sheetID int64
		if sheet.Properties != nil {
			sheetTitle = sheet.Properties.Title
			sheetID = sheet.Properties.SheetId
		}
		for _, ch := range sheet.Charts {
			if ch == nil {
				continue
			}
			it := chartItem{
				ChartID:    ch.ChartId,
				SheetID:    sheetID,
				SheetTitle: sheetTitle,
			}
			if ch.Spec != nil {
				it.Title = ch.Spec.Title
				if ch.Spec.BasicChart != nil {
					it.Type = ch.Spec.BasicChart.ChartType
				}
			}
			items = append(items, it)
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"charts": items})
	}

	if len(items) == 0 {
		u.Err().Println("No charts")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "CHART_ID\tTITLE\tTYPE\tSHEET_ID\tSHEET_TITLE")
	for _, it := range items {
		fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%s\n",
			it.ChartID, it.Title, it.Type, it.SheetID, it.SheetTitle,
		)
	}
	return nil
}

// ---------- get ----------

type SheetsChartGetCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	ChartID       int64  `arg:"" name:"chartId" help:"Chart ID"`
}

func (c *SheetsChartGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(charts,properties(sheetId,title))").
		Do()
	if err != nil {
		return err
	}

	for _, sheet := range resp.Sheets {
		for _, ch := range sheet.Charts {
			if ch == nil || ch.ChartId != c.ChartID {
				continue
			}

			if outfmt.IsJSON(ctx) {
				return outfmt.WriteJSON(ctx, os.Stdout, ch)
			}

			// Text mode: print key fields.
			title := ""
			chartType := ""
			if ch.Spec != nil {
				title = ch.Spec.Title
				if ch.Spec.BasicChart != nil {
					chartType = ch.Spec.BasicChart.ChartType
				}
			}
			sheetTitle := ""
			if sheet.Properties != nil {
				sheetTitle = sheet.Properties.Title
			}
			u.Out().Printf("chartId\t%d", ch.ChartId)
			u.Out().Printf("title\t%s", title)
			u.Out().Printf("type\t%s", chartType)
			u.Out().Printf("sheet\t%s", sheetTitle)
			return nil
		}
	}

	return usagef("chart %d not found", c.ChartID)
}

// ---------- create ----------

type SheetsChartCreateCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	SpecJSON      string `name:"spec-json" required:"" help:"EmbeddedChart JSON (inline or @file)"`
	Sheet         string `name:"sheet" help:"Sheet name for anchor (resolved to sheetId)"`
	Anchor        string `name:"anchor" help:"Anchor cell in A1 notation (e.g. A1, E10)"`
	Width         int64  `name:"width" help:"Chart width in pixels" default:"600"`
	Height        int64  `name:"height" help:"Chart height in pixels" default:"371"`
}

func (c *SheetsChartCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	specBytes, err := resolveInlineOrFileBytes(c.SpecJSON)
	if err != nil {
		return fmt.Errorf("read --spec-json: %w", err)
	}
	if len(specBytes) == 0 {
		return usage("empty --spec-json")
	}

	var chart sheets.EmbeddedChart
	if err := json.Unmarshal(specBytes, &chart); err != nil {
		return fmt.Errorf("invalid --spec-json: %w", err)
	}

	if err := dryRunExit(ctx, flags, "sheets.chart.create", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet":          c.Sheet,
		"anchor":         c.Anchor,
		"width":          c.Width,
		"height":         c.Height,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	// Resolve sheet name → ID for the anchor position.
	if c.Sheet != "" || c.Anchor != "" {
		pos, posErr := buildChartPosition(ctx, svc, spreadsheetID, c.Sheet, c.Anchor, c.Width, c.Height)
		if posErr != nil {
			return posErr
		}
		chart.Position = pos
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{AddChart: &sheets.AddChartRequest{Chart: &chart}},
		},
	}

	resp, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do()
	if err != nil {
		return err
	}

	var chartID int64
	if len(resp.Replies) > 0 && resp.Replies[0].AddChart != nil && resp.Replies[0].AddChart.Chart != nil {
		chartID = resp.Replies[0].AddChart.Chart.ChartId
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"chartId":       chartID,
		})
	}

	u.Out().Printf("Created chart %d in spreadsheet %s", chartID, spreadsheetID)
	return nil
}

// ---------- update ----------

type SheetsChartUpdateCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	ChartID       int64  `arg:"" name:"chartId" help:"Chart ID to update"`
	SpecJSON      string `name:"spec-json" required:"" help:"ChartSpec JSON (inline or @file)"`
}

func (c *SheetsChartUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	specBytes, err := resolveInlineOrFileBytes(c.SpecJSON)
	if err != nil {
		return fmt.Errorf("read --spec-json: %w", err)
	}
	if len(specBytes) == 0 {
		return usage("empty --spec-json")
	}

	var spec sheets.ChartSpec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		return fmt.Errorf("invalid --spec-json: %w", err)
	}

	if err := dryRunExit(ctx, flags, "sheets.chart.update", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"chart_id":       c.ChartID,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				UpdateChartSpec: &sheets.UpdateChartSpecRequest{
					ChartId: c.ChartID,
					Spec:    &spec,
				},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"chartId":       c.ChartID,
		})
	}

	u.Out().Printf("Updated chart %d in spreadsheet %s", c.ChartID, spreadsheetID)
	return nil
}

// ---------- delete ----------

type SheetsChartDeleteCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	ChartID       int64  `arg:"" name:"chartId" help:"Chart ID to delete"`
}

func (c *SheetsChartDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, "sheets.chart.delete", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"chart_id":       c.ChartID,
	}, "delete chart "+strconv.FormatInt(c.ChartID, 10)); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				DeleteEmbeddedObject: &sheets.DeleteEmbeddedObjectRequest{
					ObjectId: c.ChartID,
				},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"chartId":       c.ChartID,
		})
	}

	u.Out().Printf("Deleted chart %d from spreadsheet %s", c.ChartID, spreadsheetID)
	return nil
}

// ---------- helpers ----------

// buildChartPosition resolves a sheet name and anchor cell into an EmbeddedObjectPosition.
func buildChartPosition(ctx context.Context, svc *sheets.Service, spreadsheetID, sheetName, anchor string, width, height int64) (*sheets.EmbeddedObjectPosition, error) {
	var sheetID int64
	if sheetName != "" {
		sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
		if err != nil {
			return nil, err
		}
		id, ok := sheetIDs[sheetName]
		if !ok {
			return nil, usagef("unknown sheet %q", sheetName)
		}
		sheetID = id
	}

	var rowIndex, colIndex int64
	if anchor != "" {
		parsed, err := parseA1Cell(anchor)
		if err != nil {
			return nil, fmt.Errorf("invalid --anchor %q: %w", anchor, err)
		}
		rowIndex = int64(parsed.row - 1) // 0-indexed
		colIndex = int64(parsed.col - 1)
	}

	return &sheets.EmbeddedObjectPosition{
		OverlayPosition: &sheets.OverlayPosition{
			AnchorCell: &sheets.GridCoordinate{
				SheetId:     sheetID,
				RowIndex:    rowIndex,
				ColumnIndex: colIndex,
			},
			WidthPixels:  width,
			HeightPixels: height,
		},
	}, nil
}

// a1Cell holds a parsed single cell reference (row and column are 1-indexed).
type a1Cell struct {
	row int
	col int
}

// parseA1Cell parses a simple cell reference like "A1" or "E10" (no sheet prefix).
func parseA1Cell(cell string) (a1Cell, error) {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return a1Cell{}, fmt.Errorf("empty cell reference")
	}

	// Split letters and digits.
	i := 0
	for i < len(cell) && ((cell[i] >= 'A' && cell[i] <= 'Z') || (cell[i] >= 'a' && cell[i] <= 'z')) {
		i++
	}
	if i == 0 || i == len(cell) {
		return a1Cell{}, fmt.Errorf("invalid cell reference %q", cell)
	}

	col, err := colLettersToIndex(strings.ToUpper(cell[:i]))
	if err != nil {
		return a1Cell{}, err
	}
	row, err := strconv.Atoi(cell[i:])
	if err != nil || row < 1 {
		return a1Cell{}, fmt.Errorf("invalid row in cell reference %q", cell)
	}
	return a1Cell{row: row, col: col}, nil
}
