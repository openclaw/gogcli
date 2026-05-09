package cmd

import (
	"context"
	"strings"

	"github.com/steipete/gogcli/internal/tracking"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailTrackStatusCmd struct{}

func (c *GmailTrackStatusCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, cfg, err := loadTrackingConfigForAccount(flags)
	if err != nil {
		return err
	}

	path, _ := tracking.ConfigPath()
	if path != "" {
		u.Out().Printf("config_path\t%s", path)
	}
	u.Out().Printf("account\t%s", account)

	if !cfg.IsConfigured() {
		u.Out().Printf("configured\tfalse")
		return nil
	}

	u.Out().Printf("configured\ttrue")
	u.Out().Printf("worker_url\t%s", cfg.WorkerURL)
	if strings.TrimSpace(cfg.WorkerName) != "" {
		u.Out().Printf("worker_name\t%s", cfg.WorkerName)
	}
	if strings.TrimSpace(cfg.DatabaseName) != "" {
		u.Out().Printf("database_name\t%s", cfg.DatabaseName)
	}
	if strings.TrimSpace(cfg.DatabaseID) != "" {
		u.Out().Printf("database_id\t%s", cfg.DatabaseID)
	}
	if cfg.TrackingCurrentKeyVersion > 0 {
		u.Out().Printf("tracking_key_version\t%d", cfg.TrackingCurrentKeyVersion)
	}
	u.Out().Printf("admin_configured\t%t", strings.TrimSpace(cfg.AdminKey) != "")

	return nil
}
