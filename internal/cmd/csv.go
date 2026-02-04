package cmd

import (
	"context"
	"fmt"
	"strings"

	csvproc "github.com/steipete/gogcli/internal/csv"
	"github.com/steipete/gogcli/internal/ui"
)

type CSVCmd struct {
	File     string   `arg:"" name:"file" help:"CSV file path"`
	Command  []string `arg:"" name:"command" help:"Command template to execute. Use ~field for substitution (e.g., 'users create ~email --first-name ~firstName')"`
	Fields   string   `name:"fields" help:"Comma-separated list of fields to include"`
	Match    []string `name:"matchfield" help:"Only process rows where FIELD:REGEX matches (e.g., 'status:^active$')"`
	Skip     []string `name:"skipfield" help:"Skip rows where FIELD:REGEX matches (e.g., 'email:@test\\.com$')"`
	SkipRows int      `name:"skiprows" help:"Skip first N data rows"`
	MaxRows  int      `name:"maxrows" help:"Max number of rows to process"`
	DryRun   bool     `name:"dry-run" help:"Preview commands without executing"`
}

func (c *CSVCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if strings.TrimSpace(c.File) == "" {
		return usage("file is required")
	}
	if len(c.Command) == 0 {
		return usage("command is required")
	}

	matchFilters, err := csvproc.ParseFieldFilters(c.Match)
	if err != nil {
		return err
	}
	skipFilters, err := csvproc.ParseFieldFilters(c.Skip)
	if err != nil {
		return err
	}

	fields := splitCSV(c.Fields)
	processed := 0
	failed := 0

	err = csvproc.Process(c.File, csvproc.Options{
		Fields:   fields,
		Match:    matchFilters,
		Skip:     skipFilters,
		SkipRows: c.SkipRows,
		MaxRows:  c.MaxRows,
	}, func(row csvproc.Row) error {
		processed++
		args, err := csvproc.SubstituteArgs(c.Command, row)
		if err != nil {
			failed++
			return fmt.Errorf("row %d: %w", row.Index, err)
		}
		if c.DryRun {
			if u != nil {
				u.Err().Printf("[dry-run] row %d: %s\n", row.Index, strings.Join(args, " "))
			}
			return nil
		}
		if err := executeSubcommand(ctx, flags, args); err != nil {
			failed++
			return fmt.Errorf("row %d: %w", row.Index, err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if u != nil {
		if c.DryRun {
			u.Err().Printf("CSV preview: processed=%d failed=%d (no commands executed)\n", processed, failed)
		} else {
			u.Err().Printf("CSV complete: processed=%d failed=%d\n", processed, failed)
		}
	}
	return nil
}
