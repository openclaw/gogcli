package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/sheetsa1"
	"github.com/steipete/gogcli/internal/sheetsvalues"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	sheetsDefaultValueInputOption = "USER_ENTERED"
	sheetsConditionOneOfList      = "ONE_OF_LIST"
	sheetsTypeDropdown            = "DROPDOWN"
	sheetsTypeText                = "TEXT"
	sheetsDimensionRows           = "ROWS"
	sheetsDimensionColumns        = "COLUMNS"
)

// cleanRange removes shell escape sequences from range arguments.
// Some shells escape ! to \! (bash history expansion), which breaks Google Sheets API calls.
func cleanRange(r string) string {
	return strings.ReplaceAll(r, `\!`, "!")
}

type SheetsCmd struct {
	Get           SheetsGetCmd             `cmd:"" name:"get" aliases:"read,show" help:"Get values from a range"`
	Update        SheetsUpdateCmd          `cmd:"" name:"update" aliases:"edit,set" help:"Update values in a range"`
	BatchUpdate   SheetsBatchUpdateCmd     `cmd:"" name:"batch-update" aliases:"batch" help:"Update values in multiple ranges with one API request"`
	Append        SheetsAppendCmd          `cmd:"" name:"append" aliases:"add" help:"Append values to a range"`
	Insert        SheetsInsertCmd          `cmd:"" name:"insert" help:"Insert empty rows or columns into a sheet"`
	DeleteDim     SheetsDeleteDimensionCmd `cmd:"" name:"delete-dimension" aliases:"delete-dim" help:"Delete rows or columns while preserving intersecting tables"`
	Clear         SheetsClearCmd           `cmd:"" name:"clear" help:"Clear values in a range"`
	Format        SheetsFormatCmd          `cmd:"" name:"format" help:"Apply cell formatting to a range"`
	Conditional   SheetsConditionalCmd     `cmd:"" name:"conditional-format" aliases:"cf,conditional-formats" help:"Manage conditional formatting rules"`
	Validation    SheetsValidationCmd      `cmd:"" name:"validation" aliases:"data-validation,validations" help:"Manage cell data validation rules"`
	Banding       SheetsBandingCmd         `cmd:"" name:"banding" aliases:"banded-ranges" help:"Manage alternating color banding"`
	Filter        SheetsFilterCmd          `cmd:"" name:"filter" aliases:"filters,basic-filter,basic-filters" help:"Manage basic filters"`
	Merge         SheetsMergeCmd           `cmd:"" name:"merge" help:"Merge cells in a range"`
	Unmerge       SheetsUnmergeCmd         `cmd:"" name:"unmerge" help:"Unmerge cells in a range"`
	CopyPaste     SheetsCopyPasteCmd       `cmd:"" name:"copy-paste" aliases:"fill,copy-range" help:"Copy a range's values/formulas/format to another range (tiles to fill down/across)"`
	NumberFormat  SheetsNumberFormatCmd    `cmd:"" name:"number-format" help:"Apply number format to a range"`
	Freeze        SheetsFreezeCmd          `cmd:"" name:"freeze" help:"Freeze rows and columns on a sheet"`
	ResizeColumns SheetsResizeColumnsCmd   `cmd:"" name:"resize-columns" help:"Resize sheet columns"`
	ResizeRows    SheetsResizeRowsCmd      `cmd:"" name:"resize-rows" help:"Resize sheet rows"`
	ReadFormat    SheetsReadFormatCmd      `cmd:"" name:"read-format" aliases:"get-format,format-read" help:"Read cell formatting from a range"`
	Notes         SheetsNotesCmd           `cmd:"" name:"notes" help:"Get cell notes from a range"`
	UpdateNote    SheetsUpdateNoteCmd      `cmd:"" name:"update-note" aliases:"set-note" help:"Set or clear a cell note"`
	FindReplace   SheetsFindReplaceCmd     `cmd:"" name:"find-replace" help:"Find and replace text across a spreadsheet"`
	Links         SheetsLinksCmd           `cmd:"" name:"links" aliases:"hyperlinks" help:"Get or set cell hyperlinks"`
	Named         SheetsNamedRangesCmd     `cmd:"" name:"named-ranges" aliases:"namedranges,nr" help:"Manage named ranges"`
	Table         SheetsTableCmd           `cmd:"" name:"table" aliases:"tables" help:"Manage Google Sheets tables"`
	Metadata      SheetsMetadataCmd        `cmd:"" name:"metadata" aliases:"info" help:"Get spreadsheet metadata"`
	Raw           SheetsRawCmd             `cmd:"" name:"raw" help:"Dump raw Google Sheets API response as JSON (Spreadsheets.Get; lossless; for scripting and LLM consumption)"`
	Create        SheetsCreateCmd          `cmd:"" name:"create" aliases:"new" help:"Create a new spreadsheet"`
	Copy          SheetsCopyCmd            `cmd:"" name:"copy" aliases:"cp,duplicate" help:"Copy a Google Sheet"`
	Export        SheetsExportCmd          `cmd:"" name:"export" aliases:"download,dl" help:"Export a Google Sheet (pdf|xlsx|csv) via Drive"`
	Chart         SheetsChartCmd           `cmd:"" name:"chart" aliases:"charts" help:"Manage spreadsheet charts"`
	AddTab        SheetsAddTabCmd          `cmd:"" name:"add-tab" aliases:"add-sheet" help:"Add a new tab/sheet to a spreadsheet"`
	RenameTab     SheetsRenameTabCmd       `cmd:"" name:"rename-tab" aliases:"rename-sheet" help:"Rename a tab/sheet in a spreadsheet"`
	DeleteTab     SheetsDeleteTabCmd       `cmd:"" name:"delete-tab" aliases:"delete-sheet" help:"Delete a tab/sheet from a spreadsheet (use --force to skip confirmation)"`
	ReorderTab    SheetsReorderTabCmd      `cmd:"" name:"reorder-tab" aliases:"move-tab,reorder-sheet,move-sheet" help:"Move a tab/sheet to a specific 0-based position in the spreadsheet"`
}

type SheetsExportCmd struct {
	SpreadsheetID string         `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Output        OutputPathFlag `embed:""`
	Format        string         `name:"format" help:"Export format: pdf|xlsx|csv" default:"xlsx"`
	Overwrite     bool           `name:"overwrite" help:"Overwrite an existing output file"`
}

func (c *SheetsExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	return exportViaDrive(ctx, flags, exportViaDriveOptions{
		Op:            "sheets.export",
		ArgName:       "spreadsheetId",
		ExpectedMime:  "application/vnd.google-apps.spreadsheet",
		KindLabel:     "Google Sheet",
		DefaultFormat: "xlsx",
		FormatHelp:    "Export format: pdf|xlsx|csv",
	}, c.SpreadsheetID, c.Output.Path, c.Format, c.Overwrite)
}

type SheetsCopyCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Title         string `arg:"" name:"title" help:"New spreadsheet title"`
	Parent        string `name:"parent" help:"Destination folder ID"`
}

func (c *SheetsCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return copyViaDrive(ctx, flags, copyViaDriveOptions{
		Op:           "sheets.copy",
		ArgName:      "spreadsheetId",
		ExpectedMime: "application/vnd.google-apps.spreadsheet",
		KindLabel:    "Google Sheet",
	}, c.SpreadsheetID, c.Title, c.Parent)
}

type SheetsGetCmd struct {
	SpreadsheetID     string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range             string `arg:"" name:"range" help:"Range (A1 notation or named range name; e.g. Sheet1!A1:B10 or MyNamedRange)"`
	MajorDimension    string `name:"dimension" help:"Major dimension: ROWS or COLUMNS"`
	ValueRenderOption string `name:"render" help:"Value render option: FORMATTED_VALUE, UNFORMATTED_VALUE, or FORMULA"`
}

func (c *SheetsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Spreadsheets.Values.Get(spreadsheetID, rangeSpec)
	if strings.TrimSpace(c.MajorDimension) != "" {
		call = call.MajorDimension(c.MajorDimension)
	}
	if strings.TrimSpace(c.ValueRenderOption) != "" {
		call = call.ValueRenderOption(c.ValueRenderOption)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		values := resp.Values
		if values == nil {
			values = [][]interface{}{}
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"range":  resp.Range,
			"values": values,
		})
	}

	if len(resp.Values) == 0 {
		u.Err().Println("No data found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	for _, row := range resp.Values {
		cells := make([]string, len(row))
		for i, cell := range row {
			cells[i] = fmt.Sprintf("%v", cell)
		}
		fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
	return nil
}

type SheetsUpdateCmd struct {
	SpreadsheetID      string   `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range              string   `arg:"" name:"range" help:"Range (A1 notation or named range name; e.g. Sheet1!A1:B2 or MyNamedRange)"`
	Values             []string `arg:"" optional:"" name:"values" help:"Values (comma-separated rows, pipe-separated cells)"`
	ValueInput         string   `name:"input" help:"Value input option: RAW or USER_ENTERED" default:"USER_ENTERED"`
	ValuesJSON         string   `name:"values-json" help:"Values as a JSON 2D array, @file, or @- for stdin"`
	CopyValidationFrom string   `name:"copy-validation-from" help:"Copy data validation from an A1 range or named range (e.g. 'Sheet1!A2:D2' or MyNamedRange) to the updated cells"`
	FailOnFormulaError bool     `name:"fail-on-formula-error" help:"Read back the updated range and fail if any cell has a Sheets formula error"`
}

func (c *SheetsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	var values [][]interface{}
	switch {
	case strings.TrimSpace(c.ValuesJSON) != "":
		b, err := resolveInlineOrFileBytes(c.ValuesJSON, stdinReader(ctx))
		if err != nil {
			return usagef("read --values-json: %v", err)
		}

		values, err = sheetsvalues.DecodeStrict(b)
		if err != nil {
			return sheetsValuesPlannerError(err)
		}
	case len(c.Values) > 0:
		values = sheetsvalues.ParseArgs(c.Values)
	default:
		return usage("provide values as args or via --values-json")
	}

	valueInputOption := strings.TrimSpace(c.ValueInput)
	if valueInputOption == "" {
		valueInputOption = sheetsDefaultValueInputOption
	}

	if err := dryRunExit(ctx, flags, "sheets.update", map[string]any{
		"spreadsheet_id":          spreadsheetID,
		"range":                   rangeSpec,
		"values":                  values,
		"value_input_option":      valueInputOption,
		"copy_validation_from":    strings.TrimSpace(c.CopyValidationFrom),
		"copy_validation_to_hint": "updatedRange",
		"fail_on_formula_error":   c.FailOnFormulaError,
	}); err != nil {
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

	vr := &sheets.ValueRange{
		Values: values,
	}

	call := svc.Spreadsheets.Values.Update(spreadsheetID, rangeSpec, vr)
	call = call.ValueInputOption(valueInputOption)

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.CopyValidationFrom) != "" {
		if strings.TrimSpace(resp.UpdatedRange) == "" {
			return fmt.Errorf("update response missing updated range for validation copy")
		}
		if copyErr := copyDataValidation(ctx, svc, spreadsheetID, c.CopyValidationFrom, resp.UpdatedRange); copyErr != nil {
			return copyErr
		}
	}

	formulaErrors := make([]sheetsFormulaError, 0)
	if c.FailOnFormulaError {
		if strings.TrimSpace(resp.UpdatedRange) == "" {
			return fmt.Errorf("update response missing updated range for formula verification")
		}
		formulaErrors, err = readSheetsFormulaErrors(ctx, svc, spreadsheetID, resp.UpdatedRange)
		if err != nil {
			return fmt.Errorf("verify updated formulas: %w", err)
		}
	}

	result := map[string]any{
		"updatedRange":   resp.UpdatedRange,
		"updatedRows":    resp.UpdatedRows,
		"updatedColumns": resp.UpdatedColumns,
		"updatedCells":   resp.UpdatedCells,
	}
	if c.FailOnFormulaError {
		result["formulaErrors"] = formulaErrors
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), result); err != nil {
			return err
		}
		if len(formulaErrors) > 0 {
			return sheetsFormulaVerificationError(formulaErrors)
		}
		return nil
	}

	u.Out().Linef("Updated %d cells in %s", resp.UpdatedCells, resp.UpdatedRange)
	if len(formulaErrors) > 0 {
		return sheetsFormulaVerificationError(formulaErrors)
	}
	return nil
}

type sheetsFormulaError struct {
	Cell    string `json:"cell"`
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
}

func readSheetsFormulaErrors(ctx context.Context, svc *sheets.Service, spreadsheetID, updatedRange string) ([]sheetsFormulaError, error) {
	spreadsheet, err := svc.Spreadsheets.Get(spreadsheetID).
		Ranges(updatedRange).
		IncludeGridData(true).
		Fields("sheets(properties(title),data(startRow,startColumn,rowData(values(effectiveValue(errorValue)))))").
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}

	formulaErrors := make([]sheetsFormulaError, 0)
	for _, sheet := range spreadsheet.Sheets {
		if sheet == nil {
			continue
		}
		title := ""
		if sheet.Properties != nil {
			title = sheet.Properties.Title
		}
		for _, grid := range sheet.Data {
			if grid == nil {
				continue
			}
			for rowOffset, row := range grid.RowData {
				if row == nil {
					continue
				}
				for columnOffset, cell := range row.Values {
					if cell == nil || cell.EffectiveValue == nil || cell.EffectiveValue.ErrorValue == nil {
						continue
					}
					formulaErrors = append(formulaErrors, sheetsFormulaError{
						Cell: sheetsa1.FormatCell(
							title,
							int(grid.StartRow)+rowOffset+1,
							int(grid.StartColumn)+columnOffset+1,
						),
						Type:    cell.EffectiveValue.ErrorValue.Type,
						Message: cell.EffectiveValue.ErrorValue.Message,
					})
				}
			}
		}
	}

	return formulaErrors, nil
}

func sheetsFormulaVerificationError(formulaErrors []sheetsFormulaError) error {
	parts := make([]string, 0, len(formulaErrors))
	for _, formulaError := range formulaErrors {
		parts = append(parts, fmt.Sprintf("%s (%s): %s", formulaError.Cell, formulaError.Type, formulaError.Message))
	}

	return fmt.Errorf("formula verification failed: %s", strings.Join(parts, "; "))
}

type SheetsBatchUpdateCmd struct {
	SpreadsheetID                string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	DataJSON                     string `name:"data-json" required:"" help:"Value ranges as JSON array, or @file (e.g. [{\"range\":\"Sheet1!A1:B2\",\"values\":[[\"a\",\"b\"]]}])"`
	ValueInput                   string `name:"input" help:"Value input option: RAW or USER_ENTERED" default:"USER_ENTERED"`
	IncludeValuesInResponse      bool   `name:"include-values-in-response" help:"Include updated values in the response"`
	ResponseValueRenderOption    string `name:"response-render" help:"Response value render option: FORMATTED_VALUE, UNFORMATTED_VALUE, or FORMULA"`
	ResponseDateTimeRenderOption string `name:"response-date-time-render" help:"Response date/time render option: SERIAL_NUMBER or FORMATTED_STRING"`
}

func (c *SheetsBatchUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	data, err := parseSheetsBatchUpdateData(c.DataJSON, stdinReader(ctx))
	if err != nil {
		return err
	}

	valueInputOption := strings.TrimSpace(c.ValueInput)
	if valueInputOption == "" {
		valueInputOption = sheetsDefaultValueInputOption
	}
	req := &sheets.BatchUpdateValuesRequest{
		Data:                    data,
		ValueInputOption:        valueInputOption,
		IncludeValuesInResponse: c.IncludeValuesInResponse,
	}
	if strings.TrimSpace(c.ResponseValueRenderOption) != "" {
		req.ResponseValueRenderOption = strings.TrimSpace(c.ResponseValueRenderOption)
	}
	if strings.TrimSpace(c.ResponseDateTimeRenderOption) != "" {
		req.ResponseDateTimeRenderOption = strings.TrimSpace(c.ResponseDateTimeRenderOption)
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.batch-update", map[string]any{
		"spreadsheet_id":                   spreadsheetID,
		"value_input_option":               req.ValueInputOption,
		"include_values_in_response":       req.IncludeValuesInResponse,
		"response_value_render_option":     req.ResponseValueRenderOption,
		"response_date_time_render_option": req.ResponseDateTimeRenderOption,
		"data":                             req.Data,
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

	resp, err := svc.Spreadsheets.Values.BatchUpdate(spreadsheetID, req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId":       resp.SpreadsheetId,
			"totalUpdatedRows":    resp.TotalUpdatedRows,
			"totalUpdatedColumns": resp.TotalUpdatedColumns,
			"totalUpdatedCells":   resp.TotalUpdatedCells,
			"totalUpdatedSheets":  resp.TotalUpdatedSheets,
			"responses":           resp.Responses,
		})
	}

	u.Out().Linef("Updated %d cells across %d ranges in %s", resp.TotalUpdatedCells, len(resp.Responses), spreadsheetID)
	return nil
}

func parseSheetsBatchUpdateData(dataJSON string, input io.Reader) ([]*sheets.ValueRange, error) {
	if strings.TrimSpace(dataJSON) == "" {
		return nil, usage("empty data-json")
	}
	b, err := resolveInlineOrFileBytes(dataJSON, input)
	if err != nil {
		return nil, usagef("read --data-json: %v", err)
	}

	data, err := sheetsvalues.DecodeRanges(b)
	if err != nil {
		return nil, sheetsValuesPlannerError(err)
	}

	return data, nil
}

type SheetsAppendCmd struct {
	SpreadsheetID      string   `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range              string   `arg:"" name:"range" help:"Range (A1 notation or named range name; e.g. Sheet1!A:C or MyNamedRange)"`
	Values             []string `arg:"" optional:"" name:"values" help:"Values (comma-separated rows, pipe-separated cells)"`
	ValueInput         string   `name:"input" help:"Value input option: RAW or USER_ENTERED" default:"USER_ENTERED"`
	Insert             string   `name:"insert" help:"Insert data option: OVERWRITE or INSERT_ROWS"`
	ValuesJSON         string   `name:"values-json" help:"Values as JSON 2D array"`
	CopyValidationFrom string   `name:"copy-validation-from" help:"Copy data validation from an A1 range or named range (e.g. 'Sheet1!A2:D2' or MyNamedRange) to the appended cells"`
}

func (c *SheetsAppendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	var values [][]interface{}

	switch {
	case strings.TrimSpace(c.ValuesJSON) != "":
		b, err := resolveInlineOrFileBytes(c.ValuesJSON, stdinReader(ctx))
		if err != nil {
			return usagef("read --values-json: %v", err)
		}

		values, err = sheetsvalues.Decode(b)
		if err != nil {
			return sheetsValuesPlannerError(err)
		}
	case len(c.Values) > 0:
		values = sheetsvalues.ParseArgs(c.Values)
	default:
		return usage("provide values as args or via --values-json")
	}

	valueInputOption := strings.TrimSpace(c.ValueInput)
	if valueInputOption == "" {
		valueInputOption = sheetsDefaultValueInputOption
	}
	insertDataOption := strings.TrimSpace(c.Insert)

	if err := dryRunExit(ctx, flags, "sheets.append", map[string]any{
		"spreadsheet_id":       spreadsheetID,
		"range":                rangeSpec,
		"values":               values,
		"value_input_option":   valueInputOption,
		"insert_data_option":   insertDataOption,
		"copy_validation_from": strings.TrimSpace(c.CopyValidationFrom),
	}); err != nil {
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

	vr := &sheets.ValueRange{
		Values: values,
	}

	call := svc.Spreadsheets.Values.Append(spreadsheetID, rangeSpec, vr)
	call = call.ValueInputOption(valueInputOption)
	if insertDataOption != "" {
		call = call.InsertDataOption(insertDataOption)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.CopyValidationFrom) != "" {
		if resp.Updates == nil || strings.TrimSpace(resp.Updates.UpdatedRange) == "" {
			return fmt.Errorf("append response missing updated range for validation copy")
		}
		if err := copyDataValidation(ctx, svc, spreadsheetID, c.CopyValidationFrom, resp.Updates.UpdatedRange); err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"updatedRange":   resp.Updates.UpdatedRange,
			"updatedRows":    resp.Updates.UpdatedRows,
			"updatedColumns": resp.Updates.UpdatedColumns,
			"updatedCells":   resp.Updates.UpdatedCells,
		})
	}

	u.Out().Linef("Appended %d cells to %s", resp.Updates.UpdatedCells, resp.Updates.UpdatedRange)
	return nil
}

type SheetsClearCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (A1 notation or named range name; e.g. Sheet1!A1:B2 or MyNamedRange)"`
}

func (c *SheetsClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	if err := dryRunExit(ctx, flags, "sheets.clear", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
	}); err != nil {
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

	resp, err := svc.Spreadsheets.Values.Clear(spreadsheetID, rangeSpec, &sheets.ClearValuesRequest{}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"clearedRange": resp.ClearedRange,
		})
	}

	u.Out().Linef("Cleared %s", resp.ClearedRange)
	return nil
}

// SheetsRawCmd dumps the full Spreadsheets.Get response as JSON, with no
// Fields restriction. `--include-grid-data` opts into returning cell-level
// data; it is off by default because grid payloads can be multi-MB and are
// the primary leakage vector (formulas may embed API keys or tokens).
//
// REST reference: https://developers.google.com/sheets/api/reference/rest/v4/spreadsheets/get
// Go type: https://pkg.go.dev/google.golang.org/api/sheets/v4#Spreadsheet
type SheetsRawCmd struct {
	SpreadsheetID   string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	IncludeGridData bool   `name:"include-grid-data" help:"Include cell-level grid data in the response (off by default; payloads can be large and may contain secrets in formulas)"`
	Pretty          bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *SheetsRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	_, svc, err := requireSheetsService(ctx, flags)
	if err != nil {
		return err
	}

	call := svc.Spreadsheets.Get(spreadsheetID).Context(ctx)
	if c.IncludeGridData {
		call = call.IncludeGridData(true)
		u.Err().Println("warning: --include-grid-data may expose cell-level formulas that contain API keys or hardcoded secrets")
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}
	resp, err = requireRawResponse(resp, "spreadsheet not found")
	if err != nil {
		return err
	}

	if len(resp.DeveloperMetadata) > 0 {
		u.Err().Println("warning: response contains developerMetadata which may hold third-party app secrets")
	}

	return writeRawJSON(ctx, resp, c.Pretty)
}

type SheetsMetadataCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
}

func (c *SheetsMetadataCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	resp, err := svc.Spreadsheets.Get(spreadsheetID).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": resp.SpreadsheetId,
			"title":         resp.Properties.Title,
			"locale":        resp.Properties.Locale,
			"timeZone":      resp.Properties.TimeZone,
			"sheets":        resp.Sheets,
		})
	}

	u.Out().Linef("ID\t%s", resp.SpreadsheetId)
	u.Out().Linef("Title\t%s", resp.Properties.Title)
	u.Out().Linef("Locale\t%s", resp.Properties.Locale)
	u.Out().Linef("TimeZone\t%s", resp.Properties.TimeZone)
	u.Out().Linef("URL\t%s", resp.SpreadsheetUrl)
	u.Out().Println("")
	u.Out().Println("Sheets:")

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), resp.Sheets, sheetsMetadataColumns())
}

type SheetsCreateCmd struct {
	Title  string `arg:"" name:"title" help:"Spreadsheet title"`
	Sheets string `name:"sheets" help:"Comma-separated sheet names to create"`
	Parent string `name:"parent" help:"Destination folder ID"`
}

func (c *SheetsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	names := splitCSV(c.Sheets)
	parent := normalizeGoogleID(strings.TrimSpace(c.Parent))
	if err := dryRunExit(ctx, flags, "sheets.create", map[string]any{
		"title":  title,
		"sheets": names,
		"parent": parent,
	}); err != nil {
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

	spreadsheet := &sheets.Spreadsheet{
		Properties: &sheets.SpreadsheetProperties{
			Title: title,
		},
	}

	if len(names) > 0 {
		spreadsheet.Sheets = make([]*sheets.Sheet, len(names))
		for i, name := range names {
			spreadsheet.Sheets[i] = &sheets.Sheet{
				Properties: &sheets.SheetProperties{
					Title: strings.TrimSpace(name),
				},
			}
		}
	}

	resp, err := svc.Spreadsheets.Create(spreadsheet).Do()
	if err != nil {
		return err
	}

	movedToParent := false
	moveError := ""
	if parent != "" {
		parentDriveSvc, driveErr := driveService(ctx, account)
		if driveErr == nil {
			var meta *drive.File
			meta, driveErr = parentDriveSvc.Files.Get(resp.SpreadsheetId).
				SupportsAllDrives(true).
				Fields("id, parents").
				Context(ctx).
				Do()
			if driveErr == nil {
				moveCall := parentDriveSvc.Files.Update(resp.SpreadsheetId, &drive.File{}).
					AddParents(parent).
					SupportsAllDrives(true).
					Context(ctx)
				if len(meta.Parents) > 0 {
					moveCall = moveCall.RemoveParents(strings.Join(meta.Parents, ","))
				}
				_, driveErr = moveCall.Do()
			}
		}
		if driveErr != nil {
			moveError = driveErr.Error()
			u.Err().Errorf("failed to move spreadsheet to folder: %v", driveErr)
			u.Err().Println("Spreadsheet created in Drive root. Move to desired folder if needed.")
		} else {
			movedToParent = true
		}
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"spreadsheetId":  resp.SpreadsheetId,
			"title":          resp.Properties.Title,
			"spreadsheetUrl": resp.SpreadsheetUrl,
		}
		if parent != "" {
			payload["parent"] = parent
			payload["movedToParent"] = movedToParent
			if moveError != "" {
				payload["moveError"] = moveError
			}
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("Created spreadsheet: %s", resp.Properties.Title)
	u.Out().Linef("ID: %s", resp.SpreadsheetId)
	u.Out().Linef("URL: %s", resp.SpreadsheetUrl)
	return nil
}
