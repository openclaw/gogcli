package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DriveOrphansCmd struct {
	List    DriveOrphansListCmd    `cmd:"" name:"list" aliases:"ls" help:"List orphaned files. This finds files owned by the user that are not in root. Note: files whose parent folders were deleted may not be detected; use Drive's 'Organize files' UI for comprehensive orphan recovery."`
	Collect DriveOrphansCollectCmd `cmd:"" name:"collect" help:"Move orphaned files into a folder. This finds files owned by the user that are not in root. Note: files whose parent folders were deleted may not be detected; use Drive's 'Organize files' UI for comprehensive orphan recovery."`
}

type DriveOrphansListCmd struct {
	User string `name:"user" help:"User email to list files for"`
	Max  int64  `name:"max" aliases:"limit" default:"50" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *DriveOrphansListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Files.List().
		Q(driveOrphansQuery()).
		Fields("nextPageToken, files(id, name, mimeType, owners(emailAddress), parents)").
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true)
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list orphaned files: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Files) == 0 {
		u.Err().Println("No orphaned files found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tTYPE\tOWNER")
	for _, f := range resp.Files {
		if f == nil {
			continue
		}
		owner := ""
		if len(f.Owners) > 0 {
			owner = f.Owners[0].EmailAddress
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(f.Id),
			sanitizeTab(f.Name),
			sanitizeTab(f.MimeType),
			sanitizeTab(owner),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type DriveOrphansCollectCmd struct {
	User   string `name:"user" help:"User email to collect files for"`
	Folder string `name:"folder" help:"Destination folder ID (default: create 'Orphans')"`
}

func (c *DriveOrphansCollectCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	if err := confirmDestructive(ctx, flags, "collect orphaned files into a folder"); err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	folderID := strings.TrimSpace(c.Folder)
	if folderID == "" {
		folder, err := svc.Files.Create(&drive.File{
			Name:     "Orphans",
			MimeType: "application/vnd.google-apps.folder",
		}).
			SupportsAllDrives(true).
			Context(ctx).
			Do()
		if err != nil {
			return fmt.Errorf("create orphans folder: %w", err)
		}
		folderID = folder.Id
	}

	moved := 0
	pageToken := ""
	for {
		call := svc.Files.List().
			Q(driveOrphansQuery()).
			Fields("nextPageToken, files(id, parents)").
			SupportsAllDrives(true).
			IncludeItemsFromAllDrives(true)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("list orphaned files: %w", err)
		}

		for _, f := range resp.Files {
			if f == nil {
				continue
			}
			update := svc.Files.Update(f.Id, &drive.File{}).
				AddParents(folderID).
				SupportsAllDrives(true)
			if len(f.Parents) > 0 {
				update = update.RemoveParents(strings.Join(f.Parents, ","))
			}
			if _, err := update.Context(ctx).Do(); err != nil {
				return fmt.Errorf("move file %s: %w", f.Id, err)
			}
			moved++
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"moved":  moved,
			"folder": folderID,
		})
	}

	u.Out().Printf("Moved %d orphaned files to %s\n", moved, folderID)
	return nil
}

func driveOrphansQuery() string {
	return "trashed=false and 'me' in owners and not 'root' in parents"
}
