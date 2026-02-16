package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/steipete/gogcli/internal/tracking"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailTrackKeyCmd struct {
	Rotate GmailTrackKeyRotateCmd `cmd:"" help:"Rotate tracking encryption key"`
}

type GmailTrackKeyRotateCmd struct {
	NoDeploy bool `name:"no-deploy" help:"Update local config only and skip worker deploy"`
}

func (c *GmailTrackKeyRotateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	account, cfg, err := loadTrackingConfigForAccount(flags)
	if err != nil {
		return err
	}

	if strings.TrimSpace(cfg.WorkerName) == "" || strings.TrimSpace(cfg.WorkerURL) == "" {
		return fmt.Errorf("tracking not configured; run 'gog gmail track setup' first")
	}

	if strings.TrimSpace(cfg.AdminKey) == "" {
		return fmt.Errorf("tracking admin key not configured; run 'gog gmail track setup' again")
	}

	if !cfg.IsConfigured() {
		return fmt.Errorf("tracking not configured; run 'gog gmail track setup' first")
	}

	currentVersion := cfg.TrackingCurrentKeyVersion
	if currentVersion <= 0 {
		currentVersion = 1
	}

	knownVersions := append([]int{}, cfg.TrackingKeyVersions...)
	if len(knownVersions) == 0 {
		knownVersions = []int{currentVersion}
	}

	trackingKeys, detectedCurrentVersion, err := tracking.LoadTrackingKeys(account, knownVersions, currentVersion)
	if err != nil {
		return fmt.Errorf("load tracking keys: %w", err)
	}
	if detectedCurrentVersion <= 0 {
		return fmt.Errorf("invalid tracking key version: %d", detectedCurrentVersion)
	}

	for version, key := range trackingKeys {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("missing tracking key for version %d", version)
		}
	}

	nextVersion := detectedCurrentVersion
	for version := range trackingKeys {
		if version > nextVersion {
			nextVersion = version
		}
	}
	nextVersion++
	if nextVersion <= 0 || nextVersion > 255 {
		return fmt.Errorf("invalid tracking key version: %d", nextVersion)
	}

	nextKey, err := tracking.GenerateKey()
	if err != nil {
		return fmt.Errorf("generate tracking key: %w", err)
	}

	trackingKeys[nextVersion] = nextKey

	// Keep sorted list of configured versions for local state and future rotation.
	versions := make([]int, 0, len(trackingKeys))
	for version := range trackingKeys {
		versions = append(versions, version)
	}
	sort.Ints(versions)

	request := map[string]any{
		"account":                  account,
		"worker_name":              cfg.WorkerName,
		"database_name":            cfg.DatabaseName,
		"tracking_current_version":  nextVersion,
		"tracking_key_versions":     versions,
		"deploy":                   !c.NoDeploy,
	}
	if err := dryRunExit(ctx, flags, "gmail.track.key.rotate", request); err != nil {
		return err
	}

	if !c.NoDeploy {
		dbName := strings.TrimSpace(cfg.DatabaseName)
		if dbName == "" {
			dbName = strings.TrimSpace(cfg.WorkerName)
		}
		dbID, deployErr := tracking.DeployWorker(ctx, u.Err(), tracking.DeployOptions{
			WorkerDir:             "internal/tracking/worker",
			WorkerName:            cfg.WorkerName,
			DatabaseName:          dbName,
			TrackingKeys:          trackingKeys,
			TrackingCurrentVersion: nextVersion,
			AdminKey:              cfg.AdminKey,
		})
		if deployErr != nil {
			return deployErr
		}
		cfg.DatabaseID = dbID
	}

	if err := tracking.SaveTrackingKeys(account, trackingKeys, nextVersion, cfg.AdminKey); err != nil {
		return fmt.Errorf("save tracking keys: %w", err)
	}

	cfg.TrackingKeyVersions = versions
	cfg.TrackingCurrentKeyVersion = nextVersion
	cfg.TrackingKey = ""
	cfg.SecretsInKeyring = true

	if err := tracking.SaveConfig(account, cfg); err != nil {
		return fmt.Errorf("save tracking config: %w", err)
	}

	if c.NoDeploy {
		u.Out().Printf("tracking_key_rotated\t%t", true)
		u.Out().Printf("current_version\t%d", nextVersion)
		u.Out().Printf("tracking_keys\t%v", versions)
		u.Err().Println("No deploy selected; rotate was stored locally only.")
		return nil
	}

	u.Out().Printf("tracking_key_rotated\t%t", true)
	u.Out().Printf("current_version\t%d", nextVersion)
	u.Out().Printf("tracking_keys\t%v", versions)

	return nil
}
