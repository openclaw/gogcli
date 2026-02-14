package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/steipete/gogcli/internal/input"
	"github.com/steipete/gogcli/internal/tracking"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailTrackSetupCmd struct {
	WorkerName   string `name:"worker-name" help:"Cloudflare Worker name (defaults to gog-email-tracker-<account>)"`
	DatabaseName string `name:"db-name" help:"D1 database name (defaults to worker name)"`
	WorkerURL    string `name:"worker-url" aliases:"domain" help:"Tracking worker base URL (e.g. https://gog-email-tracker.<acct>.workers.dev)"`
	TrackingKey  string `name:"tracking-key" help:"Tracking key (base64; generates one if omitted)"`
	AdminKey     string `name:"admin-key" help:"Admin key for /opens (generates one if omitted)"`
	Deploy       bool   `name:"deploy" help:"Provision D1 + deploy the worker (requires wrangler)"`
	WorkerDir    string `name:"worker-dir" help:"Worker directory (default: internal/tracking/worker)"`
}

func (c *GmailTrackSetupCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	// Avoid hitting the keyring for implicit account selection in dry-run mode.
	if flags != nil && flags.DryRun &&
		strings.TrimSpace(flags.Account) == "" &&
		strings.TrimSpace(os.Getenv("GOG_ACCOUNT")) == "" {
		return usage("missing --account (dry-run requires an explicit account and does not auto-select)")
	}

	account, cfg, err := loadTrackingConfigForAccount(flags)
	if err != nil {
		return err
	}

	workerName := strings.TrimSpace(c.WorkerName)
	if workerName == "" {
		workerName = strings.TrimSpace(cfg.WorkerName)
	}
	if workerName == "" {
		workerName = tracking.DefaultWorkerName(account)
	}
	workerName = tracking.SanitizeWorkerName(workerName)
	if workerName == "" {
		return fmt.Errorf("invalid worker name")
	}
	c.WorkerName = workerName

	dbName := strings.TrimSpace(c.DatabaseName)
	if dbName == "" {
		dbName = strings.TrimSpace(cfg.DatabaseName)
	}
	if dbName == "" {
		dbName = workerName
	}
	c.DatabaseName = dbName

	if c.WorkerURL == "" {
		c.WorkerURL = strings.TrimSpace(cfg.WorkerURL)
	}
	if c.WorkerURL == "" && !flags.NoInput && !(flags != nil && flags.DryRun) {
		line, readErr := input.PromptLine(ctx, "Tracking worker base URL (e.g. https://...workers.dev): ")
		if readErr != nil {
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, os.ErrClosed) {
				return &ExitError{Code: 1, Err: errors.New("cancelled")}
			}

			return fmt.Errorf("read worker url: %w", readErr)
		}

		c.WorkerURL = strings.TrimSpace(line)
	}
	c.WorkerURL = strings.TrimSpace(c.WorkerURL)
	if c.WorkerURL == "" {
		return usage("required: --worker-url")
	}

	key := strings.TrimSpace(c.TrackingKey)
	if key == "" {
		key = strings.TrimSpace(cfg.TrackingKey)
	}
	if key == "" {
		key, err = tracking.GenerateKey()
		if err != nil {
			return fmt.Errorf("generate tracking key: %w", err)
		}
	}

	adminKey := strings.TrimSpace(c.AdminKey)
	if adminKey == "" {
		adminKey = strings.TrimSpace(cfg.AdminKey)
	}
	if adminKey == "" {
		adminKey, err = generateAdminKey()
		if err != nil {
			return fmt.Errorf("generate admin key: %w", err)
		}
	}

	if c.WorkerDir == "" {
		c.WorkerDir = filepath.Join("internal", "tracking", "worker")
	}

	// Avoid touching keyring and avoid provisioning/deploying in dry-run mode.
	if err := dryRunExit(ctx, flags, "gmail.track.setup", map[string]any{
		"account":          account,
		"worker_url":       c.WorkerURL,
		"worker_name":      workerName,
		"database_name":    c.DatabaseName,
		"deploy":           c.Deploy,
		"worker_dir":       c.WorkerDir,
		"tracking_key_set": strings.TrimSpace(key) != "",
		"admin_key_set":    strings.TrimSpace(adminKey) != "",
	}); err != nil {
		return err
	}

	if err := tracking.SaveSecrets(account, key, adminKey); err != nil {
		return fmt.Errorf("save tracking secrets: %w", err)
	}

	cfg.Enabled = true
	cfg.WorkerURL = c.WorkerURL
	cfg.WorkerName = workerName
	cfg.DatabaseName = c.DatabaseName
	cfg.SecretsInKeyring = true
	cfg.TrackingKey = ""
	cfg.AdminKey = ""

	if c.Deploy {
		dbID, deployErr := tracking.DeployWorker(ctx, u.Err(), tracking.DeployOptions{
			WorkerDir:    c.WorkerDir,
			WorkerName:   workerName,
			DatabaseName: c.DatabaseName,
			TrackingKey:  key,
			AdminKey:     adminKey,
		})
		if deployErr != nil {
			return deployErr
		}
		cfg.DatabaseID = dbID
	}

	if err := tracking.SaveConfig(account, cfg); err != nil {
		return fmt.Errorf("save tracking config: %w", err)
	}

	path, _ := tracking.ConfigPath()
	u.Out().Printf("configured\ttrue")
	u.Out().Printf("account\t%s", account)
	if path != "" {
		u.Out().Printf("config_path\t%s", path)
	}
	u.Out().Printf("worker_url\t%s", cfg.WorkerURL)
	u.Out().Printf("worker_name\t%s", cfg.WorkerName)
	u.Out().Printf("database_name\t%s", cfg.DatabaseName)
	if cfg.DatabaseID != "" {
		u.Out().Printf("database_id\t%s", cfg.DatabaseID)
	}

	if !c.Deploy {
		u.Err().Println("")
		u.Err().Println("Next steps (manual worker deploy):")
		u.Err().Printf("  - cd %s", c.WorkerDir)
		u.Err().Println("  - use these values when prompted:")
		u.Err().Printf("    TRACKING_KEY=%s", key)
		u.Err().Printf("    ADMIN_KEY=%s", adminKey)
		u.Err().Printf("  - wrangler d1 create %s", c.DatabaseName)
		u.Err().Println("  - wrangler d1 execute <db> --file schema.sql --remote")
		u.Err().Printf("  - set wrangler.toml name=%s + database_id", cfg.WorkerName)
		u.Err().Println("  - wrangler secret put TRACKING_KEY")
		u.Err().Println("  - wrangler secret put ADMIN_KEY")
		u.Err().Println("  - wrangler deploy")
	}

	return nil
}

func generateAdminKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
