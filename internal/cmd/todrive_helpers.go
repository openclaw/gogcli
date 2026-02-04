package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/todrive"
	"github.com/steipete/gogcli/internal/ui"
)

type ToDriveFlags struct {
	ToDrive   bool   `name:"todrive" help:"Output to Google Sheets"`
	SheetName string `name:"todrive-sheet" help:"Sheet name"`
	Folder    string `name:"todrive-folder" help:"Drive folder ID"`
	Timestamp bool   `name:"todrive-timestamp" help:"Append timestamp to sheet name"`
	Notify    string `name:"todrive-notify" help:"Email to notify"`
	Update    bool   `name:"todrive-update" help:"Update existing sheet"`
}

func (t ToDriveFlags) enabled() bool { return t.ToDrive }

func writeToDrive(ctx context.Context, flags *RootFlags, title string, headers []string, rows [][]string, opts ToDriveFlags) (bool, error) {
	if !opts.enabled() {
		return false, nil
	}
	account, err := requireAccount(flags)
	if err != nil {
		return true, err
	}

	writer, err := todrive.New(ctx, account)
	if err != nil {
		return true, err
	}

	result, err := writer.Write(ctx, headers, rows, todrive.Options{
		SheetName: title,
		FolderID:  opts.Folder,
		Timestamp: opts.Timestamp,
		Notify:    opts.Notify,
		Update:    opts.Update,
	})
	if err != nil {
		return true, err
	}

	if outfmt.IsJSON(ctx) {
		return true, outfmt.WriteJSON(os.Stdout, map[string]any{
			"sheetId":   result.SpreadsheetID,
			"sheetName": result.SheetName,
			"url":       result.URL,
		})
	}

	u := ui.FromContext(ctx)
	if u != nil {
		u.Out().Printf("Saved to Google Sheets: %s\n", result.URL)
	}
	return true, nil
}

func toDriveTitle(base string, opts ToDriveFlags) string {
	if opts.SheetName != "" {
		return opts.SheetName
	}
	return base
}

func toDriveRow(values ...string) []string {
	row := make([]string, len(values))
	copy(row, values)
	return row
}

func toDriveBool(value bool) string {
	if value {
		return strTrue
	}
	return strFalse
}

func toDriveNumber(value int64) string {
	return fmt.Sprintf("%d", value)
}
