package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsNotesCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B10)"`
}

func (c *SheetsNotesCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Ranges(rangeSpec).
		IncludeGridData(true).
		Fields("sheets(properties(title),data(startRow,startColumn,rowData(values(note,formattedValue))))").
		Do()
	if err != nil {
		return err
	}

	type cellNote struct {
		Sheet string `json:"sheet"`
		A1    string `json:"a1"`
		Row   int    `json:"row"`
		Col   int    `json:"col"`
		Value string `json:"value"`
		Note  string `json:"note"`
	}

	var notes []cellNote

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
					notes = append(notes, cellNote{
						Sheet: sheetTitle,
						A1:    formatA1Cell(sheetTitle, absRow, absCol),
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"range":         rangeSpec,
			"notes":         notes,
		})
	}

	if len(notes) == 0 {
		u.Err().Println("No notes found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "A1\tVALUE\tNOTE")
	for _, n := range notes {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			oneLine(n.A1),
			oneLine(n.Value),
			oneLine(n.Note),
		)
	}
	return nil
}

var simpleSheetNameRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func formatA1Cell(sheetTitle string, row, col int) string {
	colLetters, err := colIndexToLetters(col)
	if err != nil || row <= 0 {
		return ""
	}
	cell := fmt.Sprintf("%s%d", colLetters, row)
	if strings.TrimSpace(sheetTitle) == "" {
		return cell
	}
	return formatSheetPrefix(sheetTitle) + cell
}

func formatSheetPrefix(sheetTitle string) string {
	title := strings.TrimSpace(sheetTitle)
	if title == "" {
		return ""
	}
	if simpleSheetNameRe.MatchString(title) {
		return title + "!"
	}
	escaped := strings.ReplaceAll(title, "'", "''")
	return "'" + escaped + "'!"
}

func colIndexToLetters(col int) (string, error) {
	if col <= 0 {
		return "", fmt.Errorf("invalid column index %d", col)
	}
	var b []byte
	for col > 0 {
		col--
		b = append(b, byte('A'+(col%26)))
		col /= 26
	}
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b), nil
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	// Keep output parseable in tables/TSV.
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
