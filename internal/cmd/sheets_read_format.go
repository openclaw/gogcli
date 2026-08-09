package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsa1"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsReadFormatCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B10)"`
	Effective     bool   `name:"effective" help:"Read effective format instead of user-entered format"`
}

type sheetsCellFormat struct {
	Sheet  string             `json:"sheet"`
	A1     string             `json:"a1"`
	Row    int                `json:"row"`
	Col    int                `json:"col"`
	Value  string             `json:"value,omitempty"`
	Format *sheets.CellFormat `json:"format"`
}

func (c *SheetsReadFormatCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	source := "userEnteredFormat"
	if c.Effective {
		source = "effectiveFormat"
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Ranges(rangeSpec).
		IncludeGridData(true).
		Fields(googleapi.Field(fmt.Sprintf("sheets(properties(title),data(startRow,startColumn,rowData(values(%s,formattedValue))))", source))).
		Do()
	if err != nil {
		return err
	}

	formats := make([]sheetsCellFormat, 0)
	for _, sheet := range resp.Sheets {
		if sheet == nil {
			continue
		}

		sheetTitle := ""
		if sheet.Properties != nil {
			sheetTitle = strings.TrimSpace(sheet.Properties.Title)
		}

		for _, data := range sheet.Data {
			if data == nil {
				continue
			}
			startRow := int(data.StartRow)
			startCol := int(data.StartColumn)

			for ri, row := range data.RowData {
				if row == nil {
					continue
				}
				for ci, cell := range row.Values {
					if cell == nil {
						continue
					}

					format := cell.UserEnteredFormat
					if c.Effective {
						format = cell.EffectiveFormat
					}
					if format == nil {
						continue
					}

					absRow := startRow + ri + 1
					absCol := startCol + ci + 1
					formats = append(formats, sheetsCellFormat{
						Sheet:  sheetTitle,
						A1:     sheetsa1.FormatCell(sheetTitle, absRow, absCol),
						Row:    absRow,
						Col:    absCol,
						Value:  cell.FormattedValue,
						Format: format,
					})
				}
			}
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"range":         rangeSpec,
			"source":        source,
			"formats":       formats,
		})
	}

	if len(formats) == 0 {
		u.Err().Linef("No %s found", source)
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), formats, sheetsCellFormatColumns())
}
