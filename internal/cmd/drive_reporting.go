package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const driveDefaultPageSize = 1000

type DriveTreeCmd struct {
	Parent    string `name:"parent" help:"Folder ID to start from (default: root)"`
	Depth     int    `name:"depth" help:"Max depth (0 = unlimited)" default:"2"`
	Max       int    `name:"max" help:"Max items to return (0 = unlimited)" default:"0"`
	AllDrives bool   `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
}

func (c *DriveTreeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	rootID := strings.TrimSpace(c.Parent)
	if rootID == "" {
		rootID = driveRootID
	}
	depth := c.Depth
	if depth < 0 {
		depth = 0
	}
	maxItems := c.Max
	if maxItems < 0 {
		maxItems = 0
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	items, truncated, err := listDriveTree(ctx, svc, driveTreeOptions{
		RootID:        rootID,
		MaxDepth:      depth,
		MaxItems:      maxItems,
		Fields:        driveTreeFields,
		IncludeFiles:  true,
		IncludeFolder: true,
		AllDrives:     c.AllDrives,
	})
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"items":     items,
			"truncated": truncated,
		})
	}

	if len(items) == 0 {
		u.Err().Println("No files")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PATH\tTYPE\tSIZE\tMODIFIED\tID")
	for _, it := range items {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\n",
			sanitizeTab(it.Path),
			driveType(it.MimeType),
			formatDriveSize(it.Size),
			formatDateTime(it.ModifiedTime),
			it.ID,
		)
	}
	if truncated {
		u.Err().Println("Results truncated; increase --max to see more.")
	}
	return nil
}

type DriveInventoryCmd struct {
	Parent    string `name:"parent" help:"Folder ID to start from (default: root)"`
	Depth     int    `name:"depth" help:"Max depth (0 = unlimited)" default:"0"`
	Max       int    `name:"max" help:"Max items to return (0 = unlimited)" default:"500"`
	Sort      string `name:"sort" help:"Sort by path|size|modified" enum:"path,size,modified" default:"path"`
	Order     string `name:"order" help:"Sort order" enum:"asc,desc" default:"asc"`
	AllDrives bool   `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
}

func (c *DriveInventoryCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	rootID := strings.TrimSpace(c.Parent)
	if rootID == "" {
		rootID = driveRootID
	}
	depth := c.Depth
	if depth < 0 {
		depth = 0
	}
	maxItems := c.Max
	if maxItems < 0 {
		maxItems = 0
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	items, truncated, err := listDriveTree(ctx, svc, driveTreeOptions{
		RootID:        rootID,
		MaxDepth:      depth,
		MaxItems:      maxItems,
		Fields:        driveInventoryFields,
		IncludeFiles:  true,
		IncludeFolder: true,
		AllDrives:     c.AllDrives,
	})
	if err != nil {
		return err
	}

	sortDriveInventory(items, c.Sort, c.Order)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"items":     items,
			"truncated": truncated,
		})
	}

	if len(items) == 0 {
		u.Err().Println("No files")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PATH\tTYPE\tSIZE\tMODIFIED\tOWNER\tID")
	for _, it := range items {
		owner := "-"
		if len(it.Owners) > 0 {
			owner = it.Owners[0]
		}
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			sanitizeTab(it.Path),
			driveType(it.MimeType),
			formatDriveSize(it.Size),
			formatDateTime(it.ModifiedTime),
			owner,
			it.ID,
		)
	}
	if truncated {
		u.Err().Println("Results truncated; increase --max to see more.")
	}
	return nil
}

type DriveDuCmd struct {
	Parent    string `name:"parent" help:"Folder ID to start from (default: root)"`
	Depth     int    `name:"depth" help:"Depth for folder totals" default:"1"`
	Max       int    `name:"max" help:"Max folders to return (0 = unlimited)" default:"50"`
	Sort      string `name:"sort" help:"Sort by size|path|files" enum:"size,path,files" default:"size"`
	Order     string `name:"order" help:"Sort order" enum:"asc,desc" default:"desc"`
	AllDrives bool   `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
}

func (c *DriveDuCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	rootID := strings.TrimSpace(c.Parent)
	if rootID == "" {
		rootID = driveRootID
	}
	depth := c.Depth
	if depth < 0 {
		depth = 0
	}
	maxItems := c.Max
	if maxItems < 0 {
		maxItems = 0
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	items, truncated, err := listDriveTree(ctx, svc, driveTreeOptions{
		RootID:        rootID,
		MaxDepth:      0,
		MaxItems:      0,
		Fields:        driveTreeFields,
		IncludeFiles:  true,
		IncludeFolder: true,
		AllDrives:     c.AllDrives,
	})
	if err != nil {
		return err
	}
	if truncated {
		return fmt.Errorf("drive du truncated unexpectedly")
	}

	summaries := summarizeDriveDu(items, rootID, depth)
	sortDriveDu(summaries, c.Sort, c.Order)

	if maxItems > 0 && len(summaries) > maxItems {
		summaries = summaries[:maxItems]
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"folders": summaries,
		})
	}

	if len(summaries) == 0 {
		u.Err().Println("No folders")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PATH\tSIZE\tFILES")
	for _, f := range summaries {
		fmt.Fprintf(w, "%s\t%s\t%d\n", sanitizeTab(f.Path), formatDriveSize(f.Size), f.Files)
	}
	return nil
}

type driveTreeItem struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	ParentID     string   `json:"parentId,omitempty"`
	MimeType     string   `json:"mimeType"`
	Size         int64    `json:"size,omitempty"`
	ModifiedTime string   `json:"modifiedTime,omitempty"`
	Owners       []string `json:"owners,omitempty"`
	MD5          string   `json:"md5,omitempty"`
	Depth        int      `json:"depth"`
}

func (d driveTreeItem) IsFolder() bool {
	return d.MimeType == driveMimeFolder
}

type driveTreeOptions struct {
	RootID        string
	MaxDepth      int
	MaxItems      int
	Fields        string
	IncludeFiles  bool
	IncludeFolder bool
	AllDrives     bool
}

type driveFolderQueueItem struct {
	ID    string
	Path  string
	Depth int
}

const (
	driveTreeFields      = "id,name,mimeType,size,modifiedTime"
	driveInventoryFields = "id,name,mimeType,size,modifiedTime,owners(emailAddress,displayName)"
)

func listDriveTree(ctx context.Context, svc *drive.Service, opts driveTreeOptions) ([]driveTreeItem, bool, error) {
	rootID := strings.TrimSpace(opts.RootID)
	if rootID == "" {
		rootID = driveRootID
	}
	fields := strings.TrimSpace(opts.Fields)
	if fields == "" {
		fields = driveTreeFields
	}

	queue := []driveFolderQueueItem{{ID: rootID, Path: "", Depth: 0}}
	out := make([]driveTreeItem, 0, 128)
	truncated := false

	for len(queue) > 0 {
		folder := queue[0]
		queue = queue[1:]

		children, err := listDriveChildren(ctx, svc, folder.ID, fields, opts.AllDrives)
		if err != nil {
			return nil, false, err
		}
		for _, child := range children {
			if child == nil {
				continue
			}
			depth := folder.Depth + 1
			item := driveTreeItem{
				ID:           child.Id,
				Name:         child.Name,
				Path:         joinDrivePath(folder.Path, child.Name),
				ParentID:     folder.ID,
				MimeType:     child.MimeType,
				Size:         child.Size,
				ModifiedTime: child.ModifiedTime,
				Owners:       driveOwners(child),
				MD5:          child.Md5Checksum,
				Depth:        depth,
			}

			if item.IsFolder() {
				if opts.IncludeFolder {
					out = append(out, item)
				}
				if opts.MaxDepth <= 0 || depth < opts.MaxDepth {
					queue = append(queue, driveFolderQueueItem{ID: child.Id, Path: item.Path, Depth: depth})
				}
			} else if opts.IncludeFiles {
				out = append(out, item)
			}

			if opts.MaxItems > 0 && len(out) >= opts.MaxItems {
				truncated = true
				return out, truncated, nil
			}
		}
	}

	return out, truncated, nil
}

func listDriveChildren(ctx context.Context, svc *drive.Service, parentID string, fields string, allDrives bool) ([]*drive.File, error) {
	if parentID == "" {
		parentID = driveRootID
	}
	q := buildDriveListQuery(parentID, "")
	out := make([]*drive.File, 0, 64)
	var pageToken string

	for {
		call := svc.Files.List().
			Q(q).
			PageSize(driveDefaultPageSize).
			PageToken(pageToken).
			OrderBy("folder,name")
		call = driveFilesListCallWithDriveSupport(call, allDrives, "")
		call = call.Fields(
			gapi.Field("nextPageToken"),
			gapi.Field("files("+fields+")"),
		).Context(ctx)
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Files...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return out, nil
}

func joinDrivePath(parent string, name string) string {
	name = sanitizeDriveName(name)
	if parent == "" {
		return name
	}
	return path.Join(parent, name)
}

func sanitizeDriveName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "_"
	}
	return name
}

func driveOwners(f *drive.File) []string {
	if f == nil || len(f.Owners) == 0 {
		return nil
	}
	out := make([]string, 0, len(f.Owners))
	for _, owner := range f.Owners {
		if owner == nil {
			continue
		}
		if owner.EmailAddress != "" {
			out = append(out, owner.EmailAddress)
		} else if owner.DisplayName != "" {
			out = append(out, owner.DisplayName)
		}
	}
	return out
}
