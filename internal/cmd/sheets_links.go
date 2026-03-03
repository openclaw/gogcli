package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsLinksCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"Range (eg. Sheet1!A1:B10)"`
}

func (c *SheetsLinksCmd) Run(ctx context.Context, flags *RootFlags) error {
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
		Fields("sheets(properties(title),data(startRow,startColumn,rowData(values(hyperlink,formattedValue,userEnteredFormat(textFormat(link(uri))),textFormatRuns(format(link(uri)))))))").
		Do()
	if err != nil {
		return err
	}

	type cellLink struct {
		Sheet string `json:"sheet"`
		A1    string `json:"a1"`
		Row   int    `json:"row"`
		Col   int    `json:"col"`
		Value string `json:"value"`
		Link  string `json:"link"`
	}

	var links []cellLink

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
					cellLinks := extractCellLinks(cell)
					if len(cellLinks) == 0 {
						continue
					}
					absRow := startRow + ri + 1
					absCol := startCol + ci + 1
					for _, link := range cellLinks {
						links = append(links, cellLink{
							Sheet: sheetTitle,
							A1:    formatA1Cell(sheetTitle, absRow, absCol),
							Row:   absRow,
							Col:   absCol,
							Value: cell.FormattedValue,
							Link:  link,
						})
					}
				}
			}
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"range":         rangeSpec,
			"links":         links,
		})
	}

	if len(links) == 0 {
		u.Err().Println("No links found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "A1\tVALUE\tLINK")
	for _, l := range links {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			oneLine(l.A1),
			oneLine(l.Value),
			oneLine(l.Link),
		)
	}
	return nil
}

func extractCellLinks(cell *sheets.CellData) []string {
	if cell == nil {
		return nil
	}

	seen := make(map[string]struct{})
	links := make([]string, 0, 1)
	add := func(link string) {
		trimmed := strings.TrimSpace(link)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		links = append(links, trimmed)
	}

	add(cell.Hyperlink)

	if cell.UserEnteredFormat != nil && cell.UserEnteredFormat.TextFormat != nil && cell.UserEnteredFormat.TextFormat.Link != nil {
		add(cell.UserEnteredFormat.TextFormat.Link.Uri)
	}

	for _, run := range cell.TextFormatRuns {
		if run == nil || run.Format == nil || run.Format.Link == nil {
			continue
		}
		add(run.Format.Link.Uri)
	}

	return links
}
