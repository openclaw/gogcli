package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/sheetsa1"
)

type SheetsImportCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID or URL"`
	Range         string `arg:"" name:"range" help:"Bounded destination A1 range including sheet name"`
	File          string `name:"file" required:"" help:"CSV/TSV path, or - for stdin" type:"path"`
	Format        string `name:"format" enum:"csv,tsv" default:"csv" help:"Input format"`
	MatchHeaders  bool   `name:"match-headers" help:"Match input header names to destination headers; keep unmentioned columns"`
}

func (c *SheetsImportCmd) Run(ctx context.Context, flags *RootFlags) error {
	r, err := sheetsa1.Parse(c.Range)
	if err != nil || r.SheetName == "" || r.StartRow < 1 || r.EndRow < 1 || r.StartCol < 1 || r.EndCol < 1 {
		return usage("range must be a bounded A1 rectangle with a sheet name")
	}
	reader := stdinReader(ctx)
	if c.File != "-" {
		f, openErr := os.Open(c.File)
		if openErr != nil {
			return openErr
		}
		defer func() { _ = f.Close() }()
		reader = f
	}
	rows, err := readSheetsDelimited(reader, c.Format)
	if err != nil {
		return usagef("read import: %v", err)
	}
	if len(rows) > r.EndRow-r.StartRow+1 {
		return usage("input exceeds destination row count")
	}
	data := []*sheets.ValueRange{}
	if !c.MatchHeaders {
		if len(rows[0]) > r.EndCol-r.StartCol+1 {
			return usage("input exceeds destination column count")
		}
		data = append(data, &sheets.ValueRange{Range: c.Range, Values: rows})
	} else {
		account, authErr := requireAccount(flags)
		if authErr != nil {
			return authErr
		}
		svc, serviceErr := sheetsService(ctx, account)
		if serviceErr != nil {
			return serviceErr
		}
		headerRange := sheetsa1.FormatCell(r.SheetName, r.StartRow, r.StartCol) + ":" + sheetsa1.FormatCell("", r.StartRow, r.EndCol)
		headers, readErr := svc.Spreadsheets.Values.Get(normalizeGoogleID(c.SpreadsheetID), headerRange).ValueRenderOption("FORMULA").Context(ctx).Do()
		if readErr != nil {
			return readErr
		}
		if len(headers.Values) == 0 {
			return usage("destination header row is empty")
		}
		data, err = mapSheetsImportHeaders(r, rows, headers.Values[0])
		if err != nil {
			return usage(err.Error())
		}
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return (&SheetsBatchUpdateCmd{SpreadsheetID: c.SpreadsheetID, DataJSON: string(b), ValueInput: "RAW"}).Run(ctx, flags)
}

func readSheetsDelimited(reader io.Reader, format string) ([][]any, error) {
	const maxBytes = 32 << 20
	b, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxBytes {
		return nil, fmt.Errorf("input exceeds 32 MiB")
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(b), "\ufeff")))
	if format == "tsv" {
		r.Comma = '\t'
	}
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 || len(records[0]) == 0 {
		return nil, fmt.Errorf("empty input")
	}
	rows := make([][]any, len(records))
	for i, record := range records {
		rows[i] = make([]any, len(record))
		for j, cell := range record {
			rows[i][j] = cell
		}
	}
	return rows, nil
}

func mapSheetsImportHeaders(r sheetsa1.Range, rows [][]any, headers []any) ([]*sheets.ValueRange, error) {
	columns := map[string]int{}
	for j, cell := range headers {
		name, ok := cell.(string)
		if !ok || name == "" {
			continue
		}
		if _, exists := columns[name]; exists {
			return nil, fmt.Errorf("duplicate destination header %q", name)
		}
		columns[name] = j
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("header matching requires at least one data row")
	}
	data := make([]*sheets.ValueRange, 0, len(rows[0]))
	seen := map[string]bool{}
	for j, cell := range rows[0] {
		name, ok := cell.(string)
		column, exists := columns[name]
		if !ok || name == "" || !exists || seen[name] {
			return nil, fmt.Errorf("missing, empty, or duplicate input header %q", name)
		}
		seen[name] = true
		values := make([][]any, 0, len(rows)-1)
		for _, row := range rows[1:] {
			values = append(values, []any{row[j]})
		}
		data = append(data, &sheets.ValueRange{
			Range:  sheetsa1.FormatCell(r.SheetName, r.StartRow+1, r.StartCol+column) + ":" + sheetsa1.FormatCell("", r.StartRow+len(rows)-1, r.StartCol+column),
			Values: values,
		})
	}
	return data, nil
}
