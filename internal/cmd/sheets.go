package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newSheetsService = googleapi.NewSheets

// cleanRange removes shell escape sequences from range arguments.
// Some shells escape ! to \! (bash history expansion), which breaks Google Sheets API calls.
func cleanRange(r string) string {
	return strings.ReplaceAll(r, `\!`, "!")
}

type SheetsCmd struct {
	Get      SheetsGetCmd      `cmd:"" name:"get" aliases:"read,show" help:"Get values from a range"`
	Update   SheetsUpdateCmd   `cmd:"" name:"update" aliases:"edit,set" help:"Update values in a range"`
	Append   SheetsAppendCmd   `cmd:"" name:"append" aliases:"add" help:"Append values to a range"`
	Clear    SheetsClearCmd    `cmd:"" name:"clear" help:"Clear values in a range"`
	Format   SheetsFormatCmd   `cmd:"" name:"format" help:"Apply cell formatting to a range"`
	Notes    SheetsNotesCmd    `cmd:"" name:"notes" help:"Get cell notes from a range"`
	Metadata SheetsMetadataCmd `cmd:"" name:"metadata" aliases:"info" help:"Get spreadsheet metadata"`
	Create   SheetsCreateCmd   `cmd:"" name:"create" aliases:"new" help:"Create a new spreadsheet"`
	Copy     SheetsCopyCmd     `cmd:"" name:"copy" aliases:"cp,duplicate" help:"Copy a Google Sheet"`
	Export   SheetsExportCmd   `cmd:"" name:"export" aliases:"download,dl" help:"Export a Google Sheet (pdf|xlsx|csv) via Drive"`
}

type SheetsExportCmd struct {
	SpreadsheetID string         `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Output        OutputPathFlag `embed:""`
	Format        string         `name:"format" help:"Export format: pdf|xlsx|csv" default:"xlsx"`
}

func (c *SheetsExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	return exportViaDrive(ctx, flags, exportViaDriveOptions{
		Op:            "sheets.export",
		ArgName:       "spreadsheetId",
		ExpectedMime:  "application/vnd.google-apps.spreadsheet",
		KindLabel:     "Google Sheet",
		DefaultFormat: "xlsx",
		FormatHelp:    "Export format: pdf|xlsx|csv",
	}, c.SpreadsheetID, c.Output.Path, c.Format)
}

type SheetsCopyCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Title         string `arg:"" name:"title" help:"New spreadsheet title"`
	Parent        string `name:"parent" help:"Destination folder ID"`
}

func (c *SheetsCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return copyViaDrive(ctx, flags, copyViaDriveOptions{
		ArgName:      "spreadsheetId",
		ExpectedMime: "application/vnd.google-apps.spreadsheet",
		KindLabel:    "Google Sheet",
	}, c.SpreadsheetID, c.Title, c.Parent)
}

type SheetsGetCmd struct {
	SpreadsheetID     string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range             string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B10)"`
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

	svc, err := newSheetsService(ctx, account)
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"range":  resp.Range,
			"values": resp.Values,
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
	Range              string   `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B2)"`
	Values             []string `arg:"" optional:"" name:"values" help:"Values (comma-separated rows, pipe-separated cells)"`
	ValueInput         string   `name:"input" help:"Value input option: RAW or USER_ENTERED" default:"USER_ENTERED"`
	ValuesJSON         string   `name:"values-json" help:"Values as JSON 2D array"`
	CopyValidationFrom string   `name:"copy-validation-from" help:"Copy data validation from an A1 range (eg. 'Sheet1!A2:D2') to the updated cells"`
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
		b, err := resolveInlineOrFileBytes(c.ValuesJSON)
		if err != nil {
			return fmt.Errorf("read --values-json: %w", err)
		}
		if unmarshalErr := json.Unmarshal(b, &values); unmarshalErr != nil {
			return fmt.Errorf("invalid JSON values: %w", unmarshalErr)
		}
	case len(c.Values) > 0:
		// Parse comma-separated rows, pipe-separated cells
		rawValues := strings.Join(c.Values, " ")
		rows := strings.Split(rawValues, ",")
		for _, row := range rows {
			cells := strings.Split(strings.TrimSpace(row), "|")
			rowData := make([]interface{}, len(cells))
			for i, cell := range cells {
				rowData[i] = strings.TrimSpace(cell)
			}
			values = append(values, rowData)
		}
	default:
		return fmt.Errorf("provide values as args or via --values-json")
	}

	valueInputOption := strings.TrimSpace(c.ValueInput)
	if valueInputOption == "" {
		valueInputOption = "USER_ENTERED"
	}

	if err := dryRunExit(ctx, flags, "sheets.update", map[string]any{
		"spreadsheet_id":          spreadsheetID,
		"range":                   rangeSpec,
		"values":                  values,
		"value_input_option":      valueInputOption,
		"copy_validation_from":    strings.TrimSpace(c.CopyValidationFrom),
		"copy_validation_to_hint": "updatedRange",
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
		if err := copyDataValidation(ctx, svc, spreadsheetID, c.CopyValidationFrom, resp.UpdatedRange); err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"updatedRange":   resp.UpdatedRange,
			"updatedRows":    resp.UpdatedRows,
			"updatedColumns": resp.UpdatedColumns,
			"updatedCells":   resp.UpdatedCells,
		})
	}

	u.Out().Printf("Updated %d cells in %s", resp.UpdatedCells, resp.UpdatedRange)
	return nil
}

type SheetsAppendCmd struct {
	SpreadsheetID      string   `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range              string   `arg:"" name:"range" help:"Range (eg. Sheet1!A:C)"`
	Values             []string `arg:"" optional:"" name:"values" help:"Values (comma-separated rows, pipe-separated cells)"`
	ValueInput         string   `name:"input" help:"Value input option: RAW or USER_ENTERED" default:"USER_ENTERED"`
	Insert             string   `name:"insert" help:"Insert data option: OVERWRITE or INSERT_ROWS"`
	ValuesJSON         string   `name:"values-json" help:"Values as JSON 2D array"`
	CopyValidationFrom string   `name:"copy-validation-from" help:"Copy data validation from an A1 range (eg. 'Sheet1!A2:D2') to the appended cells"`
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
		b, err := resolveInlineOrFileBytes(c.ValuesJSON)
		if err != nil {
			return fmt.Errorf("read --values-json: %w", err)
		}
		if unmarshalErr := json.Unmarshal(b, &values); unmarshalErr != nil {
			return fmt.Errorf("invalid JSON values: %w", unmarshalErr)
		}
	case len(c.Values) > 0:
		rawValues := strings.Join(c.Values, " ")
		rows := strings.Split(rawValues, ",")
		for _, row := range rows {
			cells := strings.Split(strings.TrimSpace(row), "|")
			rowData := make([]interface{}, len(cells))
			for i, cell := range cells {
				rowData[i] = strings.TrimSpace(cell)
			}
			values = append(values, rowData)
		}
	default:
		return fmt.Errorf("provide values as args or via --values-json")
	}

	valueInputOption := strings.TrimSpace(c.ValueInput)
	if valueInputOption == "" {
		valueInputOption = "USER_ENTERED"
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

	svc, err := newSheetsService(ctx, account)
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"updatedRange":   resp.Updates.UpdatedRange,
			"updatedRows":    resp.Updates.UpdatedRows,
			"updatedColumns": resp.Updates.UpdatedColumns,
			"updatedCells":   resp.Updates.UpdatedCells,
		})
	}

	u.Out().Printf("Appended %d cells to %s", resp.Updates.UpdatedCells, resp.Updates.UpdatedRange)
	return nil
}

type SheetsClearCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B2)"`
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

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Values.Clear(spreadsheetID, rangeSpec, &sheets.ClearValuesRequest{}).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"clearedRange": resp.ClearedRange,
		})
	}

	u.Out().Printf("Cleared %s", resp.ClearedRange)
	return nil
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

	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": resp.SpreadsheetId,
			"title":         resp.Properties.Title,
			"locale":        resp.Properties.Locale,
			"timeZone":      resp.Properties.TimeZone,
			"sheets":        resp.Sheets,
		})
	}

	u.Out().Printf("ID\t%s", resp.SpreadsheetId)
	u.Out().Printf("Title\t%s", resp.Properties.Title)
	u.Out().Printf("Locale\t%s", resp.Properties.Locale)
	u.Out().Printf("TimeZone\t%s", resp.Properties.TimeZone)
	u.Out().Printf("URL\t%s", resp.SpreadsheetUrl)
	u.Out().Println("")
	u.Out().Println("Sheets:")

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tROWS\tCOLS")
	for _, sheet := range resp.Sheets {
		props := sheet.Properties
		fmt.Fprintf(tw, "%d\t%s\t%d\t%d\n",
			props.SheetId,
			props.Title,
			props.GridProperties.RowCount,
			props.GridProperties.ColumnCount,
		)
	}
	_ = tw.Flush()
	return nil
}

type SheetsCreateCmd struct {
	Title  string `arg:"" name:"title" help:"Spreadsheet title"`
	Sheets string `name:"sheets" help:"Comma-separated sheet names to create"`
}

func (c *SheetsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	names := splitCSV(c.Sheets)
	if err := dryRunExit(ctx, flags, "sheets.create", map[string]any{
		"title":  title,
		"sheets": names,
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

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId":  resp.SpreadsheetId,
			"title":          resp.Properties.Title,
			"spreadsheetUrl": resp.SpreadsheetUrl,
		})
	}

	u.Out().Printf("Created spreadsheet: %s", resp.Properties.Title)
	u.Out().Printf("ID: %s", resp.SpreadsheetId)
	u.Out().Printf("URL: %s", resp.SpreadsheetUrl)
	return nil
}
