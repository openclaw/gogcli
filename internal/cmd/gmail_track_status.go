package cmd

import (
	"context"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailTrackStatusCmd struct{}

func (c *GmailTrackStatusCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, cfg, configStore, _, err := loadTrackingConfigForAccount(ctx, flags)
	if err != nil {
		return err
	}

	path := configStore.Path()
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"account":    account,
			"configPath": path,
			"configured": cfg.IsConfigured(),
		}
		if cfg.IsConfigured() {
			payload["adminConfigured"] = strings.TrimSpace(cfg.AdminKey) != ""
			payload["databaseId"] = cfg.DatabaseID
			payload["databaseName"] = cfg.DatabaseName
			payload["trackingKeyVersion"] = cfg.TrackingCurrentKeyVersion
			payload["workerName"] = cfg.WorkerName
			payload["workerUrl"] = cfg.WorkerURL
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	if path != "" {
		u.Out().Linef("config_path\t%s", path)
	}
	u.Out().Linef("account\t%s", account)

	if !cfg.IsConfigured() {
		u.Out().Linef("configured\tfalse")
		return nil
	}

	u.Out().Linef("configured\ttrue")
	u.Out().Linef("worker_url\t%s", cfg.WorkerURL)
	if strings.TrimSpace(cfg.WorkerName) != "" {
		u.Out().Linef("worker_name\t%s", cfg.WorkerName)
	}
	if strings.TrimSpace(cfg.DatabaseName) != "" {
		u.Out().Linef("database_name\t%s", cfg.DatabaseName)
	}
	if strings.TrimSpace(cfg.DatabaseID) != "" {
		u.Out().Linef("database_id\t%s", cfg.DatabaseID)
	}
	if cfg.TrackingCurrentKeyVersion > 0 {
		u.Out().Linef("tracking_key_version\t%d", cfg.TrackingCurrentKeyVersion)
	}
	u.Out().Linef("admin_configured\t%t", strings.TrimSpace(cfg.AdminKey) != "")

	return nil
}
