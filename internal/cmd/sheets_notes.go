package cmd

import (
	"context"
	"strings"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsa1"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsNotesCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (A1 notation or named range name; e.g. Sheet1!A1:B10 or MyNamedRange)"`
}

func (c *SheetsNotesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}

	_, svc, err := requireSheetsService(ctx, flags)
	if err != nil {
		return err
	}

	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Ranges(rangeSpec).
		IncludeGridData(true).
		Fields("sheets(properties(title),data(startRow,startColumn,rowData(values(note,formattedValue))))").
		Do()
	if err != nil {
		return err
	}

	var notes []sheetsCellNote

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
					if cell.Note == "" {
						continue
					}
					absRow := startRow + ri + 1
					absCol := startCol + ci + 1
					notes = append(notes, sheetsCellNote{
						Sheet: sheetTitle,
						A1:    sheetsa1.FormatCell(sheetTitle, absRow, absCol),
						Row:   absRow,
						Col:   absCol,
						Value: cell.FormattedValue,
						Note:  cell.Note,
					})
				}
			}
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"range":         rangeSpec,
			"notes":         notes,
		})
	}

	if len(notes) == 0 {
		u.Err().Println("No notes found")
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), notes, sheetsNoteColumns())
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	// Keep output parseable in tables/TSV.
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
