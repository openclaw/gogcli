package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/drivereport"
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
	if err := validateDriveScanBounds(c.Depth, c.Max); err != nil {
		return err
	}

	rootID := strings.TrimSpace(c.Parent)
	if rootID == "" {
		rootID = driveRootID
	}
	depth := c.Depth
	maxItems := c.Max

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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":     items,
			"truncated": truncated,
		})
	}

	if len(items) == 0 {
		u.Err().Println("No files")
		return nil
	}

	if err := outfmt.WriteTable(ctx, stdoutWriter(ctx), items, driveTreeColumns(outfmt.IsPlain(ctx))); err != nil {
		return err
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
	if err := validateDriveScanBounds(c.Depth, c.Max); err != nil {
		return err
	}

	rootID := strings.TrimSpace(c.Parent)
	if rootID == "" {
		rootID = driveRootID
	}
	depth := c.Depth
	maxItems := c.Max

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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":     items,
			"truncated": truncated,
		})
	}

	if len(items) == 0 {
		u.Err().Println("No files")
		return nil
	}

	if err := outfmt.WriteTable(ctx, stdoutWriter(ctx), items, driveInventoryColumns(outfmt.IsPlain(ctx))); err != nil {
		return err
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
	if err := validateDriveScanBounds(c.Depth, c.Max); err != nil {
		return err
	}

	rootID := strings.TrimSpace(c.Parent)
	if rootID == "" {
		rootID = driveRootID
	}
	depth := c.Depth
	maxItems := c.Max

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	placements, truncated, err := listDrivePlacements(ctx, svc, driveTreeOptions{
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

	summaries, err := drivereport.Summarize(placements, rootID, depth)
	if err != nil {
		return err
	}
	drivereport.SortSummaries(summaries, c.Sort, c.Order)

	if maxItems > 0 && len(summaries) > maxItems {
		summaries = summaries[:maxItems]
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"folders": summaries,
		})
	}

	if len(summaries) == 0 {
		u.Err().Println("No folders")
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), summaries, driveDuColumns())
}

type driveTreeItem struct {
	ID                string                     `json:"id"`
	Name              string                     `json:"name"`
	Path              string                     `json:"path"`
	ParentID          string                     `json:"parentId,omitempty"`
	MimeType          string                     `json:"mimeType"`
	Size              int64                      `json:"size,omitempty"`
	ModifiedTime      string                     `json:"modifiedTime,omitempty"`
	WebViewLink       string                     `json:"webViewLink,omitempty"`
	Owners            []string                   `json:"owners,omitempty"`
	MD5               string                     `json:"md5,omitempty"`
	ShortcutDetails   *drive.FileShortcutDetails `json:"shortcutDetails,omitempty"`
	Depth             int                        `json:"depth"`
	placementID       drivePlacementID
	parentPlacementID drivePlacementID
}

func (d driveTreeItem) IsFolder() bool {
	return d.MimeType == driveMimeFolder
}

func validateDriveScanBounds(depth, maxItems int) error {
	if depth < 0 {
		return usage("--depth must be >= 0")
	}
	if maxItems < 0 {
		return usage("--max must be >= 0")
	}
	return nil
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

type drivePlacementID = drivereport.PlacementID

const (
	driveRootPlacementID = drivereport.RootPlacementID
	driveTreeFields      = "id,name,mimeType,size,modifiedTime,shortcutDetails(targetId,targetMimeType,targetResourceKey)"
	driveInventoryFields = "id,name,mimeType,size,modifiedTime,owners(emailAddress,displayName),shortcutDetails(targetId,targetMimeType,targetResourceKey)"
)

func listDriveTree(ctx context.Context, svc *drive.Service, opts driveTreeOptions) ([]driveTreeItem, bool, error) {
	placements, truncated, err := listDrivePlacements(ctx, svc, opts)
	if err != nil {
		return nil, false, err
	}
	items := make([]driveTreeItem, 0, len(placements))
	for _, placement := range placements {
		items = append(items, driveTreeItemFromPlacement(placement))
	}
	return items, truncated, nil
}

func listDrivePlacements(ctx context.Context, svc *drive.Service, opts driveTreeOptions) ([]drivereport.Placement, bool, error) {
	fields := strings.TrimSpace(opts.Fields)
	if fields == "" {
		fields = driveTreeFields
	}
	placements, truncated, err := drivereport.Traverse(ctx, driveTreeSource{
		service:   svc,
		fields:    fields,
		allDrives: opts.AllDrives,
	}, drivereport.Options{
		RootID:         opts.RootID,
		MaxDepth:       opts.MaxDepth,
		MaxItems:       opts.MaxItems,
		IncludeFiles:   opts.IncludeFiles,
		IncludeFolders: opts.IncludeFolder,
	})
	if err != nil {
		return nil, false, err
	}
	return placements, truncated, nil
}

type driveTreeSource struct {
	service   *drive.Service
	fields    string
	allDrives bool
}

func (s driveTreeSource) Children(ctx context.Context, parentID string) ([]drivereport.File, error) {
	children, err := listDriveChildren(ctx, s.service, parentID, s.fields, s.allDrives)
	if err != nil {
		return nil, err
	}
	files := make([]drivereport.File, 0, len(children))
	for _, child := range children {
		if child == nil {
			continue
		}
		files = append(files, drivereport.File{
			ID:              child.Id,
			Name:            child.Name,
			MimeType:        child.MimeType,
			Size:            child.Size,
			ModifiedTime:    child.ModifiedTime,
			WebViewLink:     child.WebViewLink,
			Owners:          driveOwners(child),
			MD5:             child.Md5Checksum,
			ShortcutDetails: driveReportShortcutDetails(child.ShortcutDetails),
		})
	}
	return files, nil
}

func driveTreeItemFromPlacement(placement drivereport.Placement) driveTreeItem {
	return driveTreeItem{
		ID:                placement.ID,
		Name:              placement.Name,
		Path:              placement.Path,
		ParentID:          placement.ParentID,
		MimeType:          placement.MimeType,
		Size:              placement.Size,
		ModifiedTime:      placement.ModifiedTime,
		WebViewLink:       placement.WebViewLink,
		Owners:            placement.Owners,
		MD5:               placement.MD5,
		ShortcutDetails:   driveAPIShortcutDetails(placement.ShortcutDetails),
		Depth:             placement.Depth,
		placementID:       placement.PlacementID,
		parentPlacementID: placement.ParentPlacementID,
	}
}

func driveReportShortcutDetails(details *drive.FileShortcutDetails) *drivereport.ShortcutDetails {
	if details == nil {
		return nil
	}
	return &drivereport.ShortcutDetails{
		TargetID:          details.TargetId,
		TargetMimeType:    details.TargetMimeType,
		TargetResourceKey: details.TargetResourceKey,
	}
}

func driveAPIShortcutDetails(details *drivereport.ShortcutDetails) *drive.FileShortcutDetails {
	if details == nil {
		return nil
	}
	return &drive.FileShortcutDetails{
		TargetId:          details.TargetID,
		TargetMimeType:    details.TargetMimeType,
		TargetResourceKey: details.TargetResourceKey,
	}
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
