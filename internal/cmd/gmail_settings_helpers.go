package cmd

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type gmailEmailStatusRow struct {
	Email  string
	Status string
}

func loadGmailSettingsService(ctx context.Context, flags *RootFlags) (*gmail.Service, error) {
	_, svc, err := requireGmailService(ctx, flags)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

func writeGmailEmailStatusList(ctx context.Context, jsonKey string, raw any, emptyMessage string, rows []gmailEmailStatusRow) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{jsonKey: raw})
	}

	u := ui.FromContext(ctx)
	if len(rows) == 0 {
		u.Err().Println(emptyMessage)
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "EMAIL\tSTATUS")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\n", sanitizeTab(row.Email), sanitizeTab(row.Status))
	}
	return nil
}

func writeGmailEmailStatusItem(ctx context.Context, jsonKey string, raw any, emailKey string, row gmailEmailStatusRow) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{jsonKey: raw})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("%s\t%s", emailKey, row.Email)
	u.Out().Printf("verification_status\t%s", row.Status)
	return nil
}

func writeGmailEmailStatusCreateResult(ctx context.Context, jsonKey string, raw any, emailKey string, row gmailEmailStatusRow, successMessage string, notes ...string) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{jsonKey: raw})
	}

	u := ui.FromContext(ctx)
	u.Out().Println(successMessage)
	u.Out().Printf("%s\t%s", emailKey, row.Email)
	u.Out().Printf("verification_status\t%s", row.Status)
	for _, note := range notes {
		if note == "" {
			continue
		}
		u.Out().Println(note)
	}
	return nil
}
