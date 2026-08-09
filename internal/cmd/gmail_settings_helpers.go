package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{jsonKey: raw})
	}

	u := ui.FromContext(ctx)
	if len(rows) == 0 {
		u.Err().Println(emptyMessage)
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), rows, gmailEmailStatusColumns())
}

func writeGmailEmailStatusItem(ctx context.Context, jsonKey string, raw any, emailKey string, row gmailEmailStatusRow) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{jsonKey: raw})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("%s\t%s", emailKey, row.Email)
	u.Out().Linef("verification_status\t%s", row.Status)
	return nil
}

func writeGmailEmailStatusCreateResult(ctx context.Context, jsonKey string, raw any, emailKey string, row gmailEmailStatusRow, successMessage string, notes ...string) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{jsonKey: raw})
	}

	u := ui.FromContext(ctx)
	u.Out().Println(successMessage)
	u.Out().Linef("%s\t%s", emailKey, row.Email)
	u.Out().Linef("verification_status\t%s", row.Status)
	for _, note := range notes {
		if note == "" {
			continue
		}
		u.Out().Println(note)
	}
	return nil
}

func normalizeGmailSettingsItems[T any](items []*T) []*T {
	if items == nil {
		return []*T{}
	}
	return items
}

type gmailSettingsEmailDelete struct {
	flagName    string
	op          string
	payloadKey  string
	jsonKey     string
	action      func(string) string
	successText func(string) string
	delete      func(*gmail.Service, string) error
}

func runGmailSettingsEmailDelete(ctx context.Context, flags *RootFlags, rawEmail string, operation gmailSettingsEmailDelete) error {
	email := strings.TrimSpace(rawEmail)
	if email == "" {
		return usage("empty " + operation.flagName)
	}
	if err := validateGmailSettingsEmail(operation.flagName, email); err != nil {
		return err
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, operation.op, map[string]any{
		operation.payloadKey: email,
	}, operation.action(email)); err != nil {
		return err
	}

	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}
	if err := operation.delete(svc, email); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"success":         true,
			operation.jsonKey: email,
		})
	}

	ui.FromContext(ctx).Out().Println(operation.successText(email))
	return nil
}
