package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

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

	spreadsheetID := strings.TrimSpace(c.SpreadsheetID)
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
		Fields("sheets(data(rowData(values(note,formattedValue))))").
		Do()
	if err != nil {
		return err
	}

	type cellNote struct {
		Row   int    `json:"row"`
		Col   int    `json:"col"`
		Value string `json:"value"`
		Note  string `json:"note"`
	}

	var notes []cellNote

	for _, sheet := range resp.Sheets {
		for _, data := range sheet.Data {
			for ri, row := range data.RowData {
				for ci, cell := range row.Values {
					if cell.Note == "" {
						continue
					}
					notes = append(notes, cellNote{
						Row:   ri + 1,
						Col:   ci + 1,
						Value: cell.FormattedValue,
						Note:  cell.Note,
					})
				}
			}
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"range": rangeSpec,
			"notes": notes,
		})
	}

	if len(notes) == 0 {
		u.Err().Println("No notes found")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ROW\tCOL\tVALUE\tNOTE")
	for _, n := range notes {
		noteLine := strings.ReplaceAll(n.Note, "\n", "\\n")
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\n", n.Row, n.Col, n.Value, noteLine)
	}
	_ = tw.Flush()
	return nil
}
