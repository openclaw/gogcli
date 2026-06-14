package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/backup"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type BackupCmd struct {
	Init   BackupInitCmd   `cmd:"" name:"init" help:"Initialize encrypted backup config and repository"`
	Push   BackupPushCmd   `cmd:"" name:"push" help:"Export services into encrypted backup shards"`
	Status BackupStatusCmd `cmd:"" name:"status" help:"Inspect backup manifest without decrypting shards"`
	Verify BackupVerifyCmd `cmd:"" name:"verify" help:"Decrypt and verify all backup shards"`
	Cat    BackupCatCmd    `cmd:"" name:"cat" help:"Decrypt one backup shard to stdout"`
	Export BackupExportCmd `cmd:"" name:"export" help:"Write a local plaintext export"`
	Gmail  BackupGmailCmd  `cmd:"" name:"gmail" help:"Gmail backup operations"`
}

type BackupGmailCmd struct {
	Push BackupGmailPushCmd `cmd:"" name:"push" help:"Export Gmail into encrypted backup shards"`
}

const (
	backupServiceAppScript     = "appscript"
	backupServiceCalendar      = "calendar"
	backupServiceChat          = "chat"
	backupServiceClassroom     = "classroom"
	backupServiceContacts      = "contacts"
	backupServiceDrive         = "drive"
	backupServiceGmail         = "gmail"
	backupServiceGmailSettings = "gmail-settings"
	backupServiceGroups        = "groups"
	backupServiceAdmin         = "admin"
	backupServiceKeep          = "keep"
	backupServiceTasks         = "tasks"
	backupServiceWorkspace     = "workspace"
)

type backupFlags struct {
	Config     string   `name:"config" help:"Backup config path" default:""`
	Repo       string   `name:"repo" help:"Local backup repository path"`
	Remote     string   `name:"remote" help:"Backup Git remote URL"`
	Identity   string   `name:"identity" help:"Local age identity path"`
	Recipients []string `name:"recipient" help:"Public age recipient (repeatable)"`
	NoPush     bool     `name:"no-push" help:"Commit locally but do not push to the remote"`
}

func (f backupFlags) options() backup.Options {
	return backup.Options{
		ConfigPath: f.Config,
		Repo:       f.Repo,
		Remote:     f.Remote,
		Identity:   f.Identity,
		Recipients: f.Recipients,
		Push:       !f.NoPush,
	}
}

type backupReadFlags struct {
	Config   string `name:"config" help:"Backup config path" default:""`
	Repo     string `name:"repo" help:"Local backup repository path"`
	Remote   string `name:"remote" help:"Backup Git remote URL"`
	Identity string `name:"identity" help:"Local age identity path"`
	NoPull   bool   `name:"no-pull" help:"Use local backup repository state without pulling first"`
}

type backupReadCompatFlags struct {
	NoPush     bool     `name:"no-push" hidden:"" help:"(deprecated) Ignored for read commands"`
	Recipients []string `name:"recipient" hidden:"" help:"(deprecated) Ignored for read commands"`
}

func (f backupReadFlags) options() backup.Options {
	return backup.Options{
		ConfigPath: f.Config,
		Repo:       f.Repo,
		Remote:     f.Remote,
		Identity:   f.Identity,
		Push:       false,
		SkipPull:   f.NoPull,
	}
}

func bindBackupConfigStore(ctx context.Context, opts *backup.Options) error {
	if opts == nil || opts.ConfigStore != nil {
		return nil
	}
	layout, err := commandLayout(ctx, config.PathKindConfig)
	if err != nil {
		return err
	}
	store, err := backup.NewConfigStore(layout, os.UserHomeDir)
	if err != nil {
		return err
	}
	opts.ConfigStore = store
	return nil
}

func backupCommandContext(ctx context.Context, flags *RootFlags) context.Context {
	if flags != nil && flags.NoInput {
		return backup.WithNoInput(ctx)
	}
	return ctx
}

type BackupInitCmd struct {
	backupFlags
}

func (c *BackupInitCmd) Run(ctx context.Context, flags *RootFlags) error {
	ctx = backupCommandContext(ctx, flags)
	opts := c.options()
	if err := bindBackupConfigStore(ctx, &opts); err != nil {
		return err
	}
	if c.NoPush && strings.TrimSpace(c.Remote) == "" {
		opts.SuppressDefaultRemote = true
	}
	if flags != nil && flags.DryRun {
		cfg, err := backup.ResolveOptions(opts)
		if err != nil {
			return err
		}
		return dryRunExit(ctx, flags, "backup.init", map[string]any{
			"repo":       cfg.Repo,
			"remote":     cfg.Remote,
			"identity":   cfg.Identity,
			"recipients": cfg.Recipients,
			"push":       opts.Push,
		})
	}

	cfg, recipient, err := backup.Init(ctx, opts)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"repo":      cfg.Repo,
			"remote":    cfg.Remote,
			"identity":  cfg.Identity,
			"recipient": recipient,
		})
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("repo\t%s", cfg.Repo)
	u.Out().Linef("remote\t%s", cfg.Remote)
	u.Out().Linef("identity\t%s", cfg.Identity)
	u.Out().Linef("recipient\t%s", recipient)
	return nil
}

type BackupPushCmd struct {
	backupFlags
	Services             string        `name:"services" help:"Comma-separated services to back up" default:"gmail"`
	Query                string        `name:"query" help:"Gmail query for bounded/test backups"`
	Max                  int64         `name:"max" aliases:"limit" help:"Max Gmail messages to export; 0 means all" default:"0"`
	IncludeSpamTrash     bool          `name:"include-spam-trash" help:"Include Gmail spam and trash" default:"true"`
	ShardMaxRows         int           `name:"shard-max-rows" help:"Max rows per encrypted shard" default:"1000"`
	DriveContents        bool          `name:"drive-contents" help:"Download/export Drive file contents into encrypted shards" default:"true" negatable:""`
	DriveBinaryContents  bool          `name:"drive-binary-contents" help:"Include non-Google Drive binary file bytes in encrypted shards"`
	DriveContentMaxBytes int64         `name:"drive-content-max-bytes" help:"Skip individual Drive content exports larger than this many bytes; 0 means unlimited" default:"0"`
	DriveCollaboration   bool          `name:"drive-collaboration" help:"Back up Drive permissions, comments, and revision metadata" default:"true" negatable:""`
	DriveContentTimeout  time.Duration `name:"drive-content-timeout" help:"Per-file Drive content export/download timeout" default:"2m"`
	WorkspaceNative      bool          `name:"workspace-native" help:"Fetch full native Docs/Sheets/Slides API JSON in addition to Drive exports"`
	WorkspaceMaxFiles    int           `name:"workspace-max-files" help:"Max Docs/Sheets/Slides files per type for native Workspace metadata; 0 means all" default:"0"`
	GmailCache           bool          `name:"gmail-cache" help:"Cache fetched Gmail raw messages locally so interrupted full backups can resume" default:"true" negatable:""`
	GmailRefreshCache    bool          `name:"gmail-refresh-cache" help:"Refetch Gmail messages even when a local backup cache entry exists"`
	GmailCheckpoints     bool          `name:"gmail-checkpoints" help:"Commit and push incomplete encrypted Gmail checkpoints during long cached fetches" default:"true" negatable:""`
	GmailCheckpointRows  int           `name:"gmail-checkpoint-rows" help:"Gmail messages per encrypted checkpoint chunk; 0 disables row-triggered checkpoints" default:"10000"`
	GmailCheckpointEvery time.Duration `name:"gmail-checkpoint-interval" help:"Max time between Gmail checkpoint pushes during fetch; 0 disables time-triggered checkpoints" default:"30m"`
	BestEffort           bool          `name:"best-effort" help:"Record optional service errors as backup rows and continue" default:"true" negatable:""`
}

func (c *BackupPushCmd) Run(ctx context.Context, flags *RootFlags) error {
	ctx = backupCommandContext(ctx, flags)
	services := expandBackupServices(splitCSV(c.Services))
	if len(services) == 0 {
		return usage("at least one --services value is required")
	}
	if err := c.validate(); err != nil {
		return err
	}
	backupOpts := c.options()
	if err := bindBackupConfigStore(ctx, &backupOpts); err != nil {
		return err
	}
	backupOpts.AsyncPush = c.GmailCheckpoints
	backupOpts.Progress = func(format string, args ...any) { gmailBackupProgressf(ctx, format, args...) }
	snapshots, err := buildBackupSnapshots(services, c.snapshotBuilders(ctx, flags, backupOpts))
	if err != nil {
		return err
	}
	result, err := backup.PushSnapshot(ctx, mergeBackupSnapshots(snapshots...), backupOpts)
	if err != nil {
		return err
	}
	return writeBackupResult(ctx, result)
}

func (c *BackupPushCmd) validate() error {
	if c.Max < 0 {
		return usage("--max must be >= 0")
	}
	if c.ShardMaxRows <= 0 {
		return usage("--shard-max-rows must be > 0")
	}
	if c.DriveContentMaxBytes < 0 {
		return usage("--drive-content-max-bytes must be >= 0")
	}
	if c.DriveContentTimeout <= 0 {
		return usage("--drive-content-timeout must be > 0")
	}
	if c.WorkspaceMaxFiles < 0 {
		return usage("--workspace-max-files must be >= 0")
	}
	if c.GmailCheckpointRows < 0 {
		return usage("--gmail-checkpoint-rows must be >= 0")
	}
	if c.GmailCheckpointEvery < 0 {
		return usage("--gmail-checkpoint-interval must be >= 0")
	}
	return nil
}

func (c *BackupPushCmd) buildOptionalSnapshot(flags *RootFlags, service string, build func() (backup.Snapshot, error)) (backup.Snapshot, error) {
	snapshot, err := build()
	if err == nil || !c.BestEffort {
		return snapshot, err
	}
	account, accountErr := requireAccount(flags)
	if accountErr != nil {
		return backup.Snapshot{}, err
	}
	return buildBackupServiceErrorSnapshot(service, backupAccountHash(account), err)
}

type BackupGmailPushCmd struct {
	backupFlags
	Query            string        `name:"query" help:"Gmail query for bounded/test backups"`
	Max              int64         `name:"max" aliases:"limit" help:"Max Gmail messages to export; 0 means all" default:"0"`
	IncludeSpamTrash bool          `name:"include-spam-trash" help:"Include spam and trash" default:"true"`
	ShardMaxRows     int           `name:"shard-max-rows" help:"Max messages per encrypted shard" default:"1000"`
	CacheMessages    bool          `name:"gmail-cache" help:"Cache fetched raw messages locally so interrupted full backups can resume" default:"true" negatable:""`
	RefreshCache     bool          `name:"gmail-refresh-cache" help:"Refetch messages even when a local backup cache entry exists"`
	Checkpoints      bool          `name:"checkpoints" help:"Commit and push incomplete encrypted checkpoints during long cached fetches" default:"true" negatable:""`
	CheckpointRows   int           `name:"checkpoint-rows" help:"Gmail messages per encrypted checkpoint chunk; 0 disables row-triggered checkpoints" default:"10000"`
	CheckpointEvery  time.Duration `name:"checkpoint-interval" help:"Max time between checkpoint pushes during fetch; 0 disables time-triggered checkpoints" default:"30m"`
}

func (c *BackupGmailPushCmd) Run(ctx context.Context, flags *RootFlags) error {
	ctx = backupCommandContext(ctx, flags)
	if err := c.validate(); err != nil {
		return err
	}
	backupOpts := c.options()
	if err := bindBackupConfigStore(ctx, &backupOpts); err != nil {
		return err
	}
	backupOpts.AsyncPush = c.Checkpoints
	backupOpts.Progress = func(format string, args ...any) { gmailBackupProgressf(ctx, format, args...) }
	snapshot, err := buildGmailBackupSnapshot(ctx, flags, gmailBackupOptions{
		Query:            c.Query,
		Max:              c.Max,
		IncludeSpamTrash: c.IncludeSpamTrash,
		ShardMaxRows:     c.ShardMaxRows,
		CacheMessages:    c.CacheMessages,
		RefreshCache:     c.RefreshCache,
		Checkpoints:      c.Checkpoints,
		CheckpointRows:   c.CheckpointRows,
		CheckpointEvery:  c.CheckpointEvery,
		BackupOptions:    backupOpts,
	})
	if err != nil {
		return err
	}
	result, err := backup.PushSnapshot(ctx, snapshot, backupOpts)
	if err != nil {
		return err
	}
	return writeBackupResult(ctx, result)
}

func (c *BackupGmailPushCmd) validate() error {
	if c.Max < 0 {
		return usage("--max must be >= 0")
	}
	if c.ShardMaxRows <= 0 {
		return usage("--shard-max-rows must be > 0")
	}
	if c.CheckpointRows < 0 {
		return usage("--checkpoint-rows must be >= 0")
	}
	if c.CheckpointEvery < 0 {
		return usage("--checkpoint-interval must be >= 0")
	}
	return nil
}

type BackupStatusCmd struct {
	backupReadFlags
	backupReadCompatFlags
}

func (c *BackupStatusCmd) Run(ctx context.Context, flags *RootFlags) error {
	ctx = backupCommandContext(ctx, flags)
	opts := c.options()
	if err := bindBackupConfigStore(ctx, &opts); err != nil {
		return err
	}
	if flags != nil && flags.DryRun {
		cfg, err := backup.ResolveOptions(opts)
		if err != nil {
			return err
		}
		return dryRunExit(ctx, flags, "backup.status", map[string]any{
			"repo": cfg.Repo,
			"pull": !c.NoPull,
		})
	}
	manifest, repo, err := backup.Status(ctx, opts)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"repo": repo, "manifest": manifest})
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("repo\t%s", repo)
	u.Out().Linef("encrypted\t%t", manifest.Encrypted)
	u.Out().Linef("exported\t%s", manifest.Exported.Format(time.RFC3339))
	u.Out().Linef("services\t%s", strings.Join(manifest.Services, ","))
	u.Out().Linef("accounts\t%s", strings.Join(manifest.Accounts, ","))
	u.Out().Linef("shards\t%d", len(manifest.Shards))
	keys := make([]string, 0, len(manifest.Counts))
	for key := range manifest.Counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		u.Out().Linef("count.%s\t%d", key, manifest.Counts[key])
	}
	return nil
}

type BackupVerifyCmd struct {
	backupReadFlags
	backupReadCompatFlags
}

func (c *BackupVerifyCmd) Run(ctx context.Context, flags *RootFlags) error {
	ctx = backupCommandContext(ctx, flags)
	opts := c.options()
	if err := bindBackupConfigStore(ctx, &opts); err != nil {
		return err
	}
	if flags != nil && flags.DryRun {
		cfg, err := backup.ResolveOptions(opts)
		if err != nil {
			return err
		}
		return dryRunExit(ctx, flags, "backup.verify", map[string]any{
			"repo": cfg.Repo,
			"pull": !c.NoPull,
		})
	}
	result, err := backup.Verify(ctx, opts)
	if err != nil {
		return err
	}
	return writeBackupResult(ctx, result)
}

func backupAccountHash(account string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(account))))
	return hex.EncodeToString(sum[:12])
}

func mergeBackupSnapshots(snapshots ...backup.Snapshot) backup.Snapshot {
	out := backup.Snapshot{Counts: map[string]int{}}
	for _, snapshot := range snapshots {
		out.Services = append(out.Services, snapshot.Services...)
		out.Accounts = append(out.Accounts, snapshot.Accounts...)
		out.Shards = append(out.Shards, snapshot.Shards...)
		for key, value := range snapshot.Counts {
			out.Counts[key] += value
		}
	}
	return out
}

func writeBackupResult(ctx context.Context, result backup.Result) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}
	u := ui.FromContext(ctx)
	u.Out().Linef("repo\t%s", result.Repo)
	u.Out().Linef("changed\t%t", result.Changed)
	u.Out().Linef("encrypted\t%t", result.Encrypted)
	u.Out().Linef("shards\t%d", result.Shards)
	keys := make([]string, 0, len(result.Counts))
	for key := range result.Counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		u.Out().Linef("count.%s\t%s", key, strconv.Itoa(result.Counts[key]))
	}
	return nil
}
