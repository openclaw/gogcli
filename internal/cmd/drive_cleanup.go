package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DriveCleanupCmd struct {
	EmptyFolders DriveCleanupEmptyFoldersCmd `cmd:"" name:"empty-folders" help:"Delete empty folders"`
}

type DriveCleanupEmptyFoldersCmd struct {
	User   string `name:"user" help:"User email to clean up"`
	Max    int64  `name:"max" aliases:"limit" default:"200" help:"Max folders to scan per page"`
	Parent string `name:"parent" help:"Only scan folders within this parent folder"`
}

func (c *DriveCleanupEmptyFoldersCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	if err = confirmDestructive(ctx, flags, "delete empty Drive folders"); err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	query := "mimeType='application/vnd.google-apps.folder' and trashed=false"
	if strings.TrimSpace(c.Parent) != "" {
		query = fmt.Sprintf("%s and '%s' in parents", query, googleapi.EscapeDriveQueryValue(strings.TrimSpace(c.Parent)))
	}

	deleted := 0
	pageToken := ""
	for {
		call := svc.Files.List().
			Q(query).
			Fields("nextPageToken, files(id, name)").
			SupportsAllDrives(true).
			IncludeItemsFromAllDrives(true)
		if c.Max > 0 {
			call = call.PageSize(c.Max)
		}
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("list folders: %w", err)
		}

		for _, folder := range resp.Files {
			if folder == nil {
				continue
			}
			hasChildren, err := driveFolderHasChildren(ctx, svc, folder.Id)
			if err != nil {
				return err
			}
			if hasChildren {
				continue
			}
			if err := svc.Files.Delete(folder.Id).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
				return fmt.Errorf("delete folder %s: %w", folder.Id, err)
			}
			deleted++
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"deleted": deleted})
	}

	u.Out().Printf("Deleted %d empty folders\n", deleted)
	return nil
}

func driveFolderHasChildren(ctx context.Context, svc *drive.Service, folderID string) (bool, error) {
	resp, err := svc.Files.List().
		Q(fmt.Sprintf("'%s' in parents and trashed=false", googleapi.EscapeDriveQueryValue(folderID))).
		PageSize(1).
		Fields("files(id)").
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return false, fmt.Errorf("check folder %s: %w", folderID, err)
	}

	return len(resp.Files) > 0, nil
}
