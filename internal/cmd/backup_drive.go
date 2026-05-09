package cmd

import (
	"context"
	"fmt"
	"sort"
	"time"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/backup"
)

type driveBackupFile struct {
	File *drive.File `json:"file"`
}

type driveBackupOptions struct {
	ShardMaxRows    int
	IncludeContents bool
	IncludeBinary   bool
	MaxContentBytes int64
	IncludeCollab   bool
	ContentTimeout  time.Duration
}

func buildDriveBackupSnapshot(ctx context.Context, flags *RootFlags, opts driveBackupOptions) (backup.Snapshot, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	svc, err := newDriveService(ctx, account)
	if err != nil {
		return backup.Snapshot{}, err
	}
	accountHash := backupAccountHash(account)
	drives, err := fetchBackupSharedDrives(ctx, svc)
	if err != nil {
		return backup.Snapshot{}, err
	}
	files, err := fetchBackupDriveFiles(ctx, svc)
	if err != nil {
		return backup.Snapshot{}, err
	}
	shards := make([]backup.PlainShard, 0, 2)
	driveShard, err := backup.NewJSONLShard(backupServiceDrive, "drives", accountHash, fmt.Sprintf("data/drive/%s/drives.jsonl.gz.age", accountHash), drives)
	if err != nil {
		return backup.Snapshot{}, err
	}
	shards = append(shards, driveShard)
	fileShards, err := buildBackupShards(backupServiceDrive, "files", accountHash, fmt.Sprintf("data/drive/%s/files", accountHash), files, opts.ShardMaxRows)
	if err != nil {
		return backup.Snapshot{}, err
	}
	shards = append(shards, fileShards...)
	counts := map[string]int{
		"drive.drives": len(drives),
		"drive.files":  len(files),
	}
	if opts.IncludeContents {
		contents, contentCounts := fetchBackupDriveContents(ctx, svc, files, opts)
		contentShards, shardErr := buildBackupShards(backupServiceDrive, "contents", accountHash, fmt.Sprintf("data/drive/%s/contents", accountHash), contents, opts.ShardMaxRows)
		if shardErr != nil {
			return backup.Snapshot{}, shardErr
		}
		shards = append(shards, contentShards...)
		for key, value := range contentCounts {
			counts[key] = value
		}
	}
	if opts.IncludeCollab {
		collab, collabCounts := fetchBackupDriveCollaboration(ctx, svc, files)
		permissionShards, shardErr := buildBackupShards(backupServiceDrive, "permissions", accountHash, fmt.Sprintf("data/drive/%s/permissions", accountHash), collab.Permissions, opts.ShardMaxRows)
		if shardErr != nil {
			return backup.Snapshot{}, shardErr
		}
		commentShards, shardErr := buildBackupShards(backupServiceDrive, "comments", accountHash, fmt.Sprintf("data/drive/%s/comments", accountHash), collab.Comments, opts.ShardMaxRows)
		if shardErr != nil {
			return backup.Snapshot{}, shardErr
		}
		revisionShards, shardErr := buildBackupShards(backupServiceDrive, "revisions", accountHash, fmt.Sprintf("data/drive/%s/revisions", accountHash), collab.Revisions, opts.ShardMaxRows)
		if shardErr != nil {
			return backup.Snapshot{}, shardErr
		}
		shards = append(shards, permissionShards...)
		shards = append(shards, commentShards...)
		shards = append(shards, revisionShards...)
		for key, value := range collabCounts {
			counts[key] = value
		}
	}
	return backup.Snapshot{
		Services: []string{backupServiceDrive},
		Accounts: []string{accountHash},
		Counts:   counts,
		Shards:   shards,
	}, nil
}

func fetchBackupSharedDrives(ctx context.Context, svc *drive.Service) ([]*drive.Drive, error) {
	var out []*drive.Drive
	pageToken := ""
	for {
		call := svc.Drives.List().
			PageSize(100).
			Fields("nextPageToken, drives(id, name, createdTime, hidden, restrictions)").
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Drives...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out, nil
}

func fetchBackupDriveFiles(ctx context.Context, svc *drive.Service) ([]driveBackupFile, error) {
	var out []driveBackupFile
	pageToken := ""
	for {
		call := svc.Files.List().
			Q("trashed = false").
			PageSize(1000).
			OrderBy("modifiedTime desc").
			Fields(gapi.Field("nextPageToken, files(id, name, mimeType, size, createdTime, modifiedTime, viewedByMeTime, parents, owners, lastModifyingUser, webViewLink, webContentLink, description, starred, trashed, explicitlyTrashed, shared, ownedByMe, driveId, md5Checksum, sha1Checksum, sha256Checksum, originalFilename, fileExtension, exportLinks, appProperties, properties)")).
			Context(ctx)
		call = driveFilesListCallWithDriveSupport(call, true, "")
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		for _, file := range resp.Files {
			out = append(out, driveBackupFile{File: file})
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return out, nil
}
