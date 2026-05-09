package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steipete/gogcli/internal/tracking"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailTrackKeyCmd struct {
	Rotate GmailTrackKeyRotateCmd `cmd:"" help:"Rotate tracking encryption key"`
}

type GmailTrackKeyRotateCmd struct {
	NoDeploy  bool   `name:"no-deploy" help:"Update local tracking keys without deploying the Worker"`
	WorkerDir string `name:"worker-dir" help:"Worker directory (default: internal/tracking/worker)"`
}

func (c *GmailTrackKeyRotateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, cfg, err := loadTrackingConfigForAccount(flags)
	if err != nil {
		return err
	}
	if !cfg.IsConfigured() {
		return fmt.Errorf("tracking not configured; run 'gog gmail track setup' first")
	}
	if strings.TrimSpace(cfg.AdminKey) == "" {
		return fmt.Errorf("tracking admin key not configured; run 'gog gmail track setup' again")
	}

	currentVersion := cfg.TrackingCurrentKeyVersion
	if currentVersion <= 0 {
		currentVersion = 1
	}
	knownVersions := tracking.NormalizeTrackingKeyVersions(cfg.TrackingKeyVersions, currentVersion)
	keys, currentVersion, err := tracking.LoadTrackingKeys(account, knownVersions, currentVersion)
	if err != nil {
		return fmt.Errorf("load tracking keys: %w", err)
	}
	if len(keys) == 0 && strings.TrimSpace(cfg.TrackingKey) != "" {
		keys[1] = cfg.TrackingKey
		currentVersion = 1
	}

	nextVersion := nextTrackingKeyVersion(keys, currentVersion)
	nextKey, err := tracking.GenerateKey()
	if err != nil {
		return fmt.Errorf("generate tracking key: %w", err)
	}
	keys[nextVersion] = nextKey

	versions := tracking.NormalizeTrackingKeyVersions(mapKeys(keys), nextVersion)
	request := map[string]any{
		"account":                      account,
		"worker_name":                  cfg.WorkerName,
		"database_name":                cfg.DatabaseName,
		"tracking_current_key_version": nextVersion,
		"tracking_key_versions":        versions,
		"deploy":                       !c.NoDeploy,
	}
	if err := dryRunExit(ctx, flags, "gmail.track.key.rotate", request); err != nil {
		return err
	}

	if !c.NoDeploy {
		workerName := strings.TrimSpace(cfg.WorkerName)
		if workerName == "" {
			return fmt.Errorf("tracking worker name not configured; run 'gog gmail track setup' again")
		}
		dbName := strings.TrimSpace(cfg.DatabaseName)
		if dbName == "" {
			dbName = workerName
		}
		workerDir := c.WorkerDir
		if workerDir == "" {
			workerDir = filepath.Join("internal", "tracking", "worker")
		}

		dbID, deployErr := tracking.DeployWorker(ctx, u.Err(), tracking.DeployOptions{
			WorkerDir:              workerDir,
			WorkerName:             workerName,
			DatabaseName:           dbName,
			TrackingKeys:           keys,
			TrackingCurrentVersion: nextVersion,
			AdminKey:               cfg.AdminKey,
		})
		if deployErr != nil {
			return deployErr
		}
		cfg.DatabaseID = dbID
		cfg.DatabaseName = dbName
		cfg.WorkerName = workerName
	}

	if err := tracking.SaveTrackingKeys(account, keys, nextVersion, cfg.AdminKey); err != nil {
		return fmt.Errorf("save tracking keys: %w", err)
	}

	cfg.SecretsInKeyring = true
	cfg.TrackingKey = ""
	cfg.TrackingCurrentKeyVersion = nextVersion
	cfg.TrackingKeyVersions = versions
	if err := tracking.SaveConfig(account, cfg); err != nil {
		return fmt.Errorf("save tracking config: %w", err)
	}

	u.Out().Printf("tracking_key_rotated\ttrue")
	u.Out().Printf("tracking_key_version\t%d", nextVersion)
	u.Out().Printf("tracking_key_versions\t%s", formatTrackingKeyVersions(versions))
	u.Out().Printf("deployed\t%t", !c.NoDeploy)

	return nil
}

func nextTrackingKeyVersion(keys map[int]string, currentVersion int) int {
	next := currentVersion
	for version := range keys {
		if version > next {
			next = version
		}
	}

	return next + 1
}

func mapKeys(values map[int]string) []int {
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	return keys
}

func formatTrackingKeyVersions(versions []int) string {
	parts := make([]string, 0, len(versions))
	for _, version := range versions {
		parts = append(parts, fmt.Sprintf("%d", version))
	}

	return strings.Join(parts, ",")
}
