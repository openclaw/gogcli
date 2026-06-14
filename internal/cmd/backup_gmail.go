package cmd

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/steipete/gogcli/internal/backup"
	gmailbackup "github.com/steipete/gogcli/internal/backup/gmail"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/ui"
)

type gmailBackupOptions struct {
	Query            string
	Max              int64
	IncludeSpamTrash bool
	ShardMaxRows     int
	AccountHash      string
	CacheMessages    bool
	RefreshCache     bool
	Checkpoints      bool
	CheckpointRows   int
	CheckpointEvery  time.Duration
	BackupOptions    backup.Options
	Cache            gmailbackup.Cache
}

type gmailBackupMessage = gmailbackup.Message

type gmailBackupLabel = gmailbackup.Label

func buildGmailBackupSnapshot(ctx context.Context, flags *RootFlags, opts gmailBackupOptions) (backup.Snapshot, error) {
	if opts.ShardMaxRows <= 0 {
		opts.ShardMaxRows = 1000
	}
	account, err := requireAccount(flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	svc, err := gmailService(ctx, account)
	if err != nil {
		return backup.Snapshot{}, err
	}
	source, err := gmailbackup.NewServiceSource(svc)
	if err != nil {
		return backup.Snapshot{}, err
	}
	accountHash := backupAccountHash(account)
	opts.AccountHash = accountHash
	labels, err := source.Labels(ctx)
	if err != nil {
		return backup.Snapshot{}, err
	}
	if opts.CacheMessages {
		if cacheErr := configureGmailBackupCache(ctx, &opts); cacheErr != nil {
			return backup.Snapshot{}, cacheErr
		}
	}
	shards := make([]backup.PlainShard, 0, 1)
	labelShard, err := backup.NewJSONLShard(backupServiceGmail, "labels", accountHash, fmt.Sprintf("data/gmail/%s/labels.jsonl.gz.age", accountHash), labels)
	if err != nil {
		return backup.Snapshot{}, err
	}
	shards = append(shards, labelShard)
	var messageCount int
	ids, err := gmailbackup.ListMessageIDs(ctx, source, gmailbackup.ListOptions{
		Selection: gmailBackupSelection(opts),
		Cache:     opts.Cache,
		UseCache:  opts.CacheMessages,
		Refresh:   opts.RefreshCache,
		Progress:  gmailBackupFetchProgress(ctx),
	})
	if err != nil {
		return backup.Snapshot{}, err
	}
	if opts.CacheMessages {
		checkpointOpts := gmailBackupCheckpointOptions(ctx, opts)
		checkpointOpts.RunID = gmailbackup.ResolveCheckpointRunID(ctx, ids, checkpointOpts)
		checkpointer := gmailbackup.NewCheckpointer(checkpointOpts, len(ids))
		if _, cacheErr := gmailbackup.EnsureMessageCache(ctx, source, ids, gmailbackup.FetchOptions{
			AccountHash: opts.AccountHash,
			Cache:       opts.Cache,
			UseCache:    true,
			Refresh:     opts.RefreshCache,
			Progress:    gmailBackupFetchProgress(ctx),
			AfterMessage: func(ctx context.Context, messageID string, event gmailbackup.Event) error {
				return checkpointer.Record(ctx, messageID, event)
			},
			ReleaseMemory: debug.FreeOSMemory,
		}); cacheErr != nil {
			return backup.Snapshot{}, cacheErr
		}
		messageShards, promoted, shardErr := gmailbackup.PromoteCheckpoint(ctx, ids, checkpointOpts)
		if shardErr != nil {
			return backup.Snapshot{}, shardErr
		}
		if !promoted {
			messageShards, shardErr = gmailbackup.BuildMessageShards(ctx, opts.Cache, ids, gmailbackup.ShardOptions{
				AccountHash: opts.AccountHash,
				MaxRows:     opts.ShardMaxRows,
				Progress: func(event gmailbackup.ShardEvent) {
					switch event.Phase {
					case "index":
						gmailBackupProgressf(ctx, "backup gmail shard-index\t%d/%d", event.Done, event.Total)
					case "build":
						gmailBackupProgressf(ctx, "backup gmail shard-build\tshards=%d\tmessages=%d/%d", event.Shards, event.Done, event.Total)
					}
				},
			})
			if shardErr != nil {
				return backup.Snapshot{}, shardErr
			}
		}
		shards = append(shards, messageShards...)
		messageCount = len(ids)
	} else {
		messages, _, err := gmailbackup.FetchMessages(ctx, source, ids, gmailbackup.FetchOptions{
			Progress: gmailBackupFetchProgress(ctx),
		})
		if err != nil {
			return backup.Snapshot{}, err
		}
		messageShards, err := gmailbackup.BuildMessageShardsFromMessages(ctx, messages, gmailbackup.ShardOptions{
			AccountHash: accountHash,
			MaxRows:     opts.ShardMaxRows,
		})
		if err != nil {
			return backup.Snapshot{}, err
		}
		shards = append(shards, messageShards...)
		messageCount = len(messages)
	}
	return backup.Snapshot{
		Services: []string{backupServiceGmail},
		Accounts: []string{accountHash},
		Counts: map[string]int{
			"gmail.labels":   len(labels),
			"gmail.messages": messageCount,
		},
		Shards: shards,
	}, nil
}

func configureGmailBackupCache(ctx context.Context, opts *gmailBackupOptions) error {
	if opts == nil || !opts.CacheMessages || opts.Cache.Configured() {
		return nil
	}
	layout, err := commandLayout(ctx, config.PathKindCache)
	if err != nil {
		return err
	}
	cache, err := gmailbackup.NewCache(layout.CacheDir)
	if err != nil {
		return err
	}
	opts.Cache = cache
	return nil
}

func gmailBackupSelection(opts gmailBackupOptions) gmailbackup.Selection {
	return gmailbackup.Selection{
		AccountHash:      opts.AccountHash,
		Query:            opts.Query,
		Max:              opts.Max,
		IncludeSpamTrash: opts.IncludeSpamTrash,
	}
}

func gmailBackupCheckpointOptions(ctx context.Context, opts gmailBackupOptions) gmailbackup.CheckpointOptions {
	return gmailbackup.CheckpointOptions{
		Enabled:          opts.Checkpoints && opts.CacheMessages,
		AccountHash:      opts.AccountHash,
		Query:            opts.Query,
		Max:              opts.Max,
		IncludeSpamTrash: opts.IncludeSpamTrash,
		Rows:             opts.CheckpointRows,
		Every:            opts.CheckpointEvery,
		Cache:            opts.Cache,
		BackupOptions:    opts.BackupOptions,
		Progress:         gmailBackupCheckpointProgress(ctx),
	}
}

func gmailBackupCheckpointProgress(ctx context.Context) func(gmailbackup.CheckpointEvent) {
	return func(event gmailbackup.CheckpointEvent) {
		switch event.Phase {
		case gmailbackup.CheckpointPhaseConfigured:
			gmailBackupProgressf(ctx, "backup gmail checkpoint\trun=%s\trows=%d\tinterval=%s", event.RunID, event.Rows, event.Every)
		case gmailbackup.CheckpointPhaseFlushed:
			gmailBackupProgressf(ctx, "backup gmail checkpoint\t%d/%d\tparts=%d\trows=%d\tchanged=%t", event.Done, event.Total, event.Parts, event.Rows, event.Changed)
		case gmailbackup.CheckpointPhaseReused:
			gmailBackupProgressf(ctx, "backup gmail checkpoint\treuse=%s\tdone=%d/%d", event.RunID, event.Done, event.Total)
		case gmailbackup.CheckpointPhasePromotionSkipped:
			gmailBackupProgressf(ctx, "backup gmail checkpoint-promote\tskip=%s\trun=%s", event.Reason, event.RunID)
		case gmailbackup.CheckpointPhasePromoted:
			gmailBackupProgressf(ctx, "backup gmail checkpoint-promote\trun=%s\tshards=%d\tmessages=%d", event.RunID, event.Shards, event.Rows)
		}
	}
}

func gmailBackupFetchProgress(ctx context.Context) func(gmailbackup.Event) {
	return func(event gmailbackup.Event) {
		switch event.Phase {
		case gmailbackup.EventPhaseList:
			switch event.Resume {
			case "complete", "partial":
				gmailBackupProgressf(ctx, "backup gmail list\tresume=%s\tmessages=%d", event.Resume, event.Done)
			case "start":
				gmailBackupProgressf(ctx, "backup gmail list\tstart\tmessages=%d", event.Done)
			default:
				gmailBackupProgressf(ctx, "backup gmail list\tmessages=%d", event.Done)
			}
		case gmailbackup.EventPhaseFetch:
			if event.Done == 0 {
				gmailBackupProgressf(ctx, "backup gmail fetch\tqueued=%d", event.Total)
				return
			}
			gmailBackupProgressf(ctx, "backup gmail fetch\t%d/%d\tfetched=%d\tcache=%d", event.Done, event.Total, event.Fetched, event.CacheHits)
		}
	}
}

func gmailBackupProgressf(ctx context.Context, format string, args ...any) {
	u := ui.FromContext(ctx)
	if u == nil {
		return
	}
	u.Err().Linef(format, args...)
}
