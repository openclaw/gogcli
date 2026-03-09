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

const driveFileListFields = "nextPageToken, files(id, name, mimeType, size, modifiedTime, parents, webViewLink)"

type driveFileListOptions struct {
	query     string
	max       int64
	page      string
	allDrives bool
}

func (c *DriveLsCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.All && strings.TrimSpace(c.Parent) != "" {
		return usage("--all cannot be combined with --parent")
	}

	folderID := strings.TrimSpace(c.Parent)
	if folderID == "" {
		folderID = "root"
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	query := buildDriveListQuery(folderID, c.Query)
	if c.All {
		query = buildDriveAllListQuery(c.Query)
	}

	resp, err := listDriveFiles(ctx, svc, driveFileListOptions{
		query:     query,
		max:       c.Max,
		page:      c.Page,
		allDrives: c.AllDrives,
	})
	if err != nil {
		return err
	}

	return writeDriveFileList(ctx, resp, "No files")
}

func (c *DriveSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	query := strings.TrimSpace(strings.Join(c.Query, " "))
	if query == "" {
		return usage("missing query")
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	resp, err := listDriveFiles(ctx, svc, driveFileListOptions{
		query:     buildDriveSearchQuery(query, c.RawQuery),
		max:       c.Max,
		page:      c.Page,
		allDrives: c.AllDrives,
	})
	if err != nil {
		return err
	}

	return writeDriveFileList(ctx, resp, "No results")
}

func listDriveFiles(ctx context.Context, svc *drive.Service, opts driveFileListOptions) (*drive.FileList, error) {
	call := svc.Files.List().
		Q(opts.query).
		PageSize(opts.max).
		PageToken(opts.page).
		OrderBy("modifiedTime desc")
	call = driveFilesListCallWithDriveSupport(call, opts.allDrives)
	return call.Fields(driveFileListFields).Context(ctx).Do()
}

func writeDriveFileList(ctx context.Context, resp *drive.FileList, emptyMessage string) error {
	u := ui.FromContext(ctx)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"files":         resp.Files,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Files) == 0 {
		u.Err().Println(emptyMessage)
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tTYPE\tSIZE\tMODIFIED")
	for _, f := range resp.Files {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\n",
			f.Id,
			f.Name,
			driveType(f.MimeType),
			formatDriveSize(f.Size),
			formatDateTime(f.ModifiedTime),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}
