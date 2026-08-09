package cmd

import (
	"context"
	"strconv"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetschart"
	"github.com/openclaw/gogcli/internal/ui"
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

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(charts(chartId,spec(title,basicChart(chartType)),position(overlayPosition(anchorCell))),properties(sheetId,title))").
		Do()
	if err != nil {
		return err
	}

	items := make([]sheetsChartItem, 0)
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
			it := sheetsChartItem{
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"charts": items})
	}

	if len(items) == 0 {
		u.Err().Println("No charts")
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), items, sheetsChartColumns())
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
	if c.ChartID <= 0 {
		return usage("chartId must be greater than 0")
	}

	svc, err := sheetsService(ctx, account)
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
				return outfmt.WriteJSON(ctx, stdoutWriter(ctx), ch)
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
			u.Out().Linef("chartId\t%d", ch.ChartId)
			u.Out().Linef("title\t%s", title)
			u.Out().Linef("type\t%s", chartType)
			u.Out().Linef("sheet\t%s", sheetTitle)
			return nil
		}
	}

	return usagef("chart %d not found", c.ChartID)
}

// ---------- create ----------

type SheetsChartCreateCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	SpecJSON      string `name:"spec-json" required:"" help:"ChartSpec or EmbeddedChart JSON (inline or @file)"`
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
	if c.Width <= 0 {
		return usage("--width must be greater than 0")
	}
	if c.Height <= 0 {
		return usage("--height must be greater than 0")
	}

	specBytes, err := resolveInlineOrFileBytes(c.SpecJSON, stdinReader(ctx))
	if err != nil {
		return usagef("read --spec-json: %v", err)
	}
	if len(specBytes) == 0 {
		return usage("empty --spec-json")
	}

	chart, err := sheetschart.ParseEmbedded(specBytes)
	if err != nil {
		return usagef("invalid --spec-json: %v", err)
	}
	if c.Anchor != "" {
		if _, anchorErr := sheetschart.ParseAnchor(c.Anchor); anchorErr != nil {
			return usagef("invalid --anchor %q: %v", c.Anchor, anchorErr)
		}
	}

	if dryErr := dryRunExit(ctx, flags, "sheets.chart.create", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet":          c.Sheet,
		"anchor":         c.Anchor,
		"width":          c.Width,
		"height":         c.Height,
	}); dryErr != nil {
		return dryErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	needsSheetResolution := c.Sheet != "" || c.Anchor != "" || sheetschart.HasZeroSheetIDs(chart.Spec)
	var sheet chartSheetResolution
	if needsSheetResolution {
		var posErr error
		sheet, posErr = resolveChartSheetResolution(svc, spreadsheetID, c.Sheet)
		if posErr != nil {
			return posErr
		}
		sheetschart.NormalizeZeroSheetIDs(chart.Spec, sheet.SheetID, sheet.HasSheetIDZero)
	}

	// Resolve sheet name → ID for the anchor position.
	if c.Sheet != "" || c.Anchor != "" {
		pos, posErr := sheetschart.BuildPosition(sheet.SheetID, c.Anchor, c.Width, c.Height)
		if posErr != nil {
			return usagef("invalid --anchor %q: %v", c.Anchor, posErr)
		}
		chart.Position = pos
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{AddChart: &sheets.AddChartRequest{Chart: chart}},
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"chartId":       chartID,
		})
	}

	u.Out().Linef("Created chart %d in spreadsheet %s", chartID, spreadsheetID)
	return nil
}

// ---------- update ----------

type SheetsChartUpdateCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	ChartID       int64  `arg:"" name:"chartId" help:"Chart ID to update"`
	SpecJSON      string `name:"spec-json" required:"" help:"ChartSpec or EmbeddedChart JSON (inline or @file)"`
}

func (c *SheetsChartUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if c.ChartID <= 0 {
		return usage("chartId must be greater than 0")
	}

	specBytes, err := resolveInlineOrFileBytes(c.SpecJSON, stdinReader(ctx))
	if err != nil {
		return usagef("read --spec-json: %v", err)
	}
	if len(specBytes) == 0 {
		return usage("empty --spec-json")
	}

	spec, err := sheetschart.ParseSpec(specBytes)
	if err != nil {
		return usagef("invalid --spec-json: %v", err)
	}

	if dryErr := dryRunExit(ctx, flags, "sheets.chart.update", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"chart_id":       c.ChartID,
	}); dryErr != nil {
		return dryErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	sheet, err := findChartSheetResolution(svc, spreadsheetID, c.ChartID)
	if err != nil {
		return err
	}
	sheetschart.NormalizeZeroSheetIDs(spec, sheet.SheetID, sheet.HasSheetIDZero)

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				UpdateChartSpec: &sheets.UpdateChartSpecRequest{
					ChartId: c.ChartID,
					Spec:    spec,
				},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"chartId":       c.ChartID,
		})
	}

	u.Out().Linef("Updated chart %d in spreadsheet %s", c.ChartID, spreadsheetID)
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
	if c.ChartID <= 0 {
		return usage("chartId must be greater than 0")
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

	svc, err := sheetsService(ctx, account)
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"chartId":       c.ChartID,
		})
	}

	u.Out().Linef("Deleted chart %d from spreadsheet %s", c.ChartID, spreadsheetID)
	return nil
}
