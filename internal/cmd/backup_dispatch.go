package cmd

import (
	"context"
	"strings"

	"github.com/openclaw/gogcli/internal/backup"
)

const backupSupportedServicesHelp = "all, admin, appscript, calendar, chat, classroom, contacts, drive, gmail, gmail-settings, groups, keep, tasks, workspace"

type backupSnapshotBuilder func() (backup.Snapshot, error)

func (c *BackupPushCmd) snapshotBuilders(ctx context.Context, flags *RootFlags, backupOpts backup.Options) map[string]backupSnapshotBuilder {
	optional := func(service string, build backupSnapshotBuilder) backupSnapshotBuilder {
		return func() (backup.Snapshot, error) {
			return c.buildOptionalSnapshot(flags, service, build)
		}
	}

	return map[string]backupSnapshotBuilder{
		backupServiceAppScript: optional(backupServiceAppScript, func() (backup.Snapshot, error) {
			return buildAppScriptBackupSnapshot(ctx, flags, c.ShardMaxRows)
		}),
		backupServiceCalendar: func() (backup.Snapshot, error) {
			return buildCalendarBackupSnapshot(ctx, flags, c.ShardMaxRows)
		},
		backupServiceChat: optional(backupServiceChat, func() (backup.Snapshot, error) {
			return buildChatBackupSnapshot(ctx, flags, c.ShardMaxRows)
		}),
		backupServiceClassroom: optional(backupServiceClassroom, func() (backup.Snapshot, error) {
			return buildClassroomBackupSnapshot(ctx, flags, c.ShardMaxRows)
		}),
		backupServiceContacts: func() (backup.Snapshot, error) {
			return buildContactsBackupSnapshot(ctx, flags, c.ShardMaxRows)
		},
		backupServiceDrive: func() (backup.Snapshot, error) {
			return buildDriveBackupSnapshot(ctx, flags, driveBackupOptions{
				ShardMaxRows:    c.ShardMaxRows,
				IncludeContents: c.DriveContents,
				IncludeBinary:   c.DriveBinaryContents,
				MaxContentBytes: c.DriveContentMaxBytes,
				IncludeCollab:   c.DriveCollaboration,
				ContentTimeout:  c.DriveContentTimeout,
			})
		},
		backupServiceGmail: func() (backup.Snapshot, error) {
			return buildGmailBackupSnapshot(ctx, flags, gmailBackupOptions{
				Query:            c.Query,
				Max:              c.Max,
				IncludeSpamTrash: c.IncludeSpamTrash,
				ShardMaxRows:     c.ShardMaxRows,
				CacheMessages:    c.GmailCache,
				RefreshCache:     c.GmailRefreshCache,
				Checkpoints:      c.GmailCheckpoints,
				CheckpointRows:   c.GmailCheckpointRows,
				CheckpointEvery:  c.GmailCheckpointEvery,
				BackupOptions:    backupOpts,
			})
		},
		backupServiceGmailSettings: func() (backup.Snapshot, error) {
			return buildGmailSettingsBackupSnapshot(ctx, flags, c.ShardMaxRows)
		},
		backupServiceGroups: optional(backupServiceGroups, func() (backup.Snapshot, error) {
			return buildGroupsBackupSnapshot(ctx, flags, c.ShardMaxRows)
		}),
		backupServiceAdmin: optional(backupServiceAdmin, func() (backup.Snapshot, error) {
			return buildAdminBackupSnapshot(ctx, flags, c.ShardMaxRows)
		}),
		backupServiceKeep: optional(backupServiceKeep, func() (backup.Snapshot, error) {
			return buildKeepBackupSnapshot(ctx, flags, c.ShardMaxRows)
		}),
		backupServiceTasks: func() (backup.Snapshot, error) {
			return buildTasksBackupSnapshot(ctx, flags, c.ShardMaxRows)
		},
		backupServiceWorkspace: optional(backupServiceWorkspace, func() (backup.Snapshot, error) {
			return buildWorkspaceBackupSnapshot(ctx, flags, workspaceBackupOptions{
				ShardMaxRows: c.ShardMaxRows,
				Native:       c.WorkspaceNative,
				MaxFiles:     c.WorkspaceMaxFiles,
			})
		}),
	}
}

func buildBackupSnapshots(services []string, builders map[string]backupSnapshotBuilder) ([]backup.Snapshot, error) {
	snapshots := make([]backup.Snapshot, 0, len(services))
	for _, rawService := range services {
		service := strings.ToLower(strings.TrimSpace(rawService))
		build, ok := builders[service]
		if !ok {
			return nil, usagef("unsupported backup service %q (supported: %s)", rawService, backupSupportedServicesHelp)
		}
		snapshot, err := build()
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}
