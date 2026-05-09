package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"
)

type driveBackupCollaboration struct {
	Permissions []driveBackupPermission
	Comments    []driveBackupComment
	Revisions   []driveBackupRevision
}

type driveBackupPermission struct {
	FileID     string            `json:"fileId"`
	Permission *drive.Permission `json:"permission,omitempty"`
	Error      string            `json:"error,omitempty"`
}

type driveBackupComment struct {
	FileID  string         `json:"fileId"`
	Comment *drive.Comment `json:"comment,omitempty"`
	Error   string         `json:"error,omitempty"`
}

type driveBackupRevision struct {
	FileID   string          `json:"fileId"`
	Revision *drive.Revision `json:"revision,omitempty"`
	Error    string          `json:"error,omitempty"`
}

func fetchBackupDriveCollaboration(ctx context.Context, svc *drive.Service, files []driveBackupFile) (driveBackupCollaboration, map[string]int) {
	var out driveBackupCollaboration
	jobs := make(chan string)
	results := make(chan driveBackupCollaboration, len(files))
	workers := 8
	if len(files) < workers {
		workers = len(files)
	}
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fileID := range jobs {
				results <- fetchBackupDriveFileCollaboration(ctx, svc, fileID)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, row := range files {
			if row.File == nil || strings.TrimSpace(row.File.Id) == "" {
				continue
			}
			select {
			case jobs <- row.File.Id:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	errors := 0
	for result := range results {
		out.Permissions = append(out.Permissions, result.Permissions...)
		out.Comments = append(out.Comments, result.Comments...)
		out.Revisions = append(out.Revisions, result.Revisions...)
		for _, permission := range result.Permissions {
			if permission.Error != "" {
				errors++
			}
		}
		for _, comment := range result.Comments {
			if comment.Error != "" {
				errors++
			}
		}
		for _, revision := range result.Revisions {
			if revision.Error != "" {
				errors++
			}
		}
	}
	sort.Slice(out.Permissions, func(i, j int) bool {
		if out.Permissions[i].FileID == out.Permissions[j].FileID {
			return drivePermissionSortKey(out.Permissions[i].Permission) < drivePermissionSortKey(out.Permissions[j].Permission)
		}
		return out.Permissions[i].FileID < out.Permissions[j].FileID
	})
	sort.Slice(out.Comments, func(i, j int) bool {
		if out.Comments[i].FileID == out.Comments[j].FileID {
			return driveCommentSortKey(out.Comments[i].Comment) < driveCommentSortKey(out.Comments[j].Comment)
		}
		return out.Comments[i].FileID < out.Comments[j].FileID
	})
	sort.Slice(out.Revisions, func(i, j int) bool {
		if out.Revisions[i].FileID == out.Revisions[j].FileID {
			return driveRevisionSortKey(out.Revisions[i].Revision) < driveRevisionSortKey(out.Revisions[j].Revision)
		}
		return out.Revisions[i].FileID < out.Revisions[j].FileID
	})
	return out, map[string]int{
		"drive.permissions":   len(out.Permissions),
		"drive.comments":      len(out.Comments),
		"drive.revisions":     len(out.Revisions),
		"drive.collab.errors": errors,
	}
}

func fetchBackupDriveFileCollaboration(ctx context.Context, svc *drive.Service, fileID string) driveBackupCollaboration {
	var out driveBackupCollaboration
	permissions, err := fetchBackupDrivePermissions(ctx, svc, fileID)
	if err != nil {
		out.Permissions = append(out.Permissions, driveBackupPermission{FileID: fileID, Error: err.Error()})
	} else {
		for _, permission := range permissions {
			out.Permissions = append(out.Permissions, driveBackupPermission{FileID: fileID, Permission: permission})
		}
	}
	comments, err := fetchBackupDriveComments(ctx, svc, fileID)
	if err != nil {
		out.Comments = append(out.Comments, driveBackupComment{FileID: fileID, Error: err.Error()})
	} else {
		for _, comment := range comments {
			out.Comments = append(out.Comments, driveBackupComment{FileID: fileID, Comment: comment})
		}
	}
	revisions, err := fetchBackupDriveRevisions(ctx, svc, fileID)
	if err != nil {
		out.Revisions = append(out.Revisions, driveBackupRevision{FileID: fileID, Error: err.Error()})
	} else {
		for _, revision := range revisions {
			out.Revisions = append(out.Revisions, driveBackupRevision{FileID: fileID, Revision: revision})
		}
	}
	return out
}

func fetchBackupDrivePermissions(ctx context.Context, svc *drive.Service, fileID string) ([]*drive.Permission, error) {
	var out []*drive.Permission
	pageToken := ""
	for {
		call := svc.Permissions.List(fileID).
			PageSize(100).
			SupportsAllDrives(true).
			Fields(gapi.Field("nextPageToken, permissions(id,type,role,emailAddress,domain,displayName,allowFileDiscovery,deleted,expirationTime,inheritedPermissionsDisabled,pendingOwner,permissionDetails,photoLink,view)")).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("drive file %s permissions: %w", fileID, err)
		}
		out = append(out, resp.Permissions...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return out, nil
}

func fetchBackupDriveComments(ctx context.Context, svc *drive.Service, fileID string) ([]*drive.Comment, error) {
	var out []*drive.Comment
	pageToken := ""
	for {
		comments, nextPageToken, err := listDriveComments(ctx, svc, fileID, driveCommentListOptions{
			includeResolved: true,
			includeQuoted:   true,
			page:            pageToken,
			max:             100,
			mode:            driveCommentListModeExpanded,
		})
		if err != nil {
			return nil, fmt.Errorf("drive file %s comments: %w", fileID, err)
		}
		out = append(out, comments...)
		if nextPageToken == "" {
			break
		}
		pageToken = nextPageToken
	}
	return out, nil
}

func fetchBackupDriveRevisions(ctx context.Context, svc *drive.Service, fileID string) ([]*drive.Revision, error) {
	var out []*drive.Revision
	pageToken := ""
	for {
		call := svc.Revisions.List(fileID).
			PageSize(200).
			Fields(gapi.Field("nextPageToken, revisions(id,mimeType,modifiedTime,keepForever,published,publishAuto,publishedOutsideDomain,publishedLink,lastModifyingUser,md5Checksum,size,originalFilename,exportLinks)")).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("drive file %s revisions: %w", fileID, err)
		}
		out = append(out, resp.Revisions...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return out, nil
}

func drivePermissionSortKey(permission *drive.Permission) string {
	if permission == nil {
		return ""
	}
	return permission.Id + "\x00" + permission.Type + "\x00" + permission.Role
}

func driveCommentSortKey(comment *drive.Comment) string {
	if comment == nil {
		return ""
	}
	return comment.Id
}

func driveRevisionSortKey(revision *drive.Revision) string {
	if revision == nil {
		return ""
	}
	return revision.ModifiedTime + "\x00" + revision.Id
}
