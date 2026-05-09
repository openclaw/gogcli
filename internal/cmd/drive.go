package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newDriveService = googleapi.NewDrive

var (
	driveSearchFieldComparisonPattern = regexp.MustCompile(`(?i)\b(?:mimeType|name|fullText|trashed|starred|modifiedTime|createdTime|viewedByMeTime|visibility)\b\s*(?:!=|<=|>=|=|<|>)`)
	driveSearchContainsPattern        = regexp.MustCompile(`(?i)\b(?:name|fullText)\b\s+contains\s+'`)
	driveSearchMembershipPattern      = regexp.MustCompile(`(?i)'[^']+'\s+in\s+(?:parents|owners|writers|readers)`)
	driveSearchHasPattern             = regexp.MustCompile(`(?i)\b(?:properties|appProperties)\b\s+has\s+\{`)
	// Only treat as "already constrained" when the query contains a real trashed predicate,
	// not just the word inside a quoted literal (e.g. "name contains 'trashed'").
	driveTrashedPredicatePattern = regexp.MustCompile(`(?i)\btrashed\b\s*(?:=|!=)\s*(?:true|false)\b`)
)

const (
	driveRootID            = "root"
	driveMimeFolder        = "application/vnd.google-apps.folder"
	driveMimeGoogleDoc     = "application/vnd.google-apps.document"
	driveMimeGoogleSheet   = "application/vnd.google-apps.spreadsheet"
	driveMimeGoogleSlides  = "application/vnd.google-apps.presentation"
	driveMimeGoogleDrawing = "application/vnd.google-apps.drawing"
	mimePDF                = "application/pdf"
	mimeCSV                = "text/csv"
	mimeDocx               = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	mimeXlsx               = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	mimePptx               = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	mimePNG                = "image/png"
	mimeTextPlain          = "text/plain"
	mimeTextMarkdown       = "text/markdown"
	mimeHTML               = "text/html"
	extPDF                 = ".pdf"
	extCSV                 = ".csv"
	extXlsx                = ".xlsx"
	extDocx                = ".docx"
	extPptx                = ".pptx"
	extPNG                 = ".png"
	extTXT                 = ".txt"
	extMD                  = ".md"
	extHTML                = ".html"
	formatAuto             = literalAuto
)

type DriveCmd struct {
	Ls          DriveLsCmd          `cmd:"" name:"ls" help:"List files in a folder (default: root)"`
	Search      DriveSearchCmd      `cmd:"" name:"search" help:"Full-text search across Drive"`
	Tree        DriveTreeCmd        `cmd:"" name:"tree" help:"Print a read-only folder tree"`
	Du          DriveDuCmd          `cmd:"" name:"du" help:"Summarize Drive folder sizes"`
	Inventory   DriveInventoryCmd   `cmd:"" name:"inventory" help:"Export a read-only Drive inventory"`
	Get         DriveGetCmd         `cmd:"" name:"get" help:"Get file metadata"`
	Download    DriveDownloadCmd    `cmd:"" name:"download" help:"Download a file (exports Google Docs formats)"`
	Copy        DriveCopyCmd        `cmd:"" name:"copy" help:"Copy a file"`
	Upload      DriveUploadCmd      `cmd:"" name:"upload" help:"Upload a file"`
	Mkdir       DriveMkdirCmd       `cmd:"" name:"mkdir" help:"Create a folder"`
	Delete      DriveDeleteCmd      `cmd:"" name:"delete" help:"Move a file to trash (use --permanent to delete forever)" aliases:"rm,del"`
	Move        DriveMoveCmd        `cmd:"" name:"move" help:"Move a file to a different folder"`
	Rename      DriveRenameCmd      `cmd:"" name:"rename" help:"Rename a file or folder"`
	Share       DriveShareCmd       `cmd:"" name:"share" help:"Share a file or folder"`
	Unshare     DriveUnshareCmd     `cmd:"" name:"unshare" help:"Remove a permission from a file"`
	Permissions DrivePermissionsCmd `cmd:"" name:"permissions" help:"List permissions on a file"`
	URL         DriveURLCmd         `cmd:"" name:"url" help:"Print web URLs for files"`
	Comments    DriveCommentsCmd    `cmd:"" name:"comments" help:"Manage comments on files"`
	Drives      DriveDrivesCmd      `cmd:"" name:"drives" help:"List shared drives (Team Drives)"`
	Raw         DriveRawCmd         `cmd:"" name:"raw" help:"Dump raw Google Drive API response as JSON (Files.Get; lossless; for scripting and LLM consumption)"`
}

type DriveLsCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"20"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	Query     string `name:"query" help:"Drive query filter"`
	Parent    string `name:"parent" help:"Folder ID to list (default: root)"`
	All       bool   `name:"all" aliases:"global" help:"List all accessible files (mutually exclusive with --parent)"`
	AllDrives bool   `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
	Fields    string `name:"fields" help:"Drive API field mask (overrides the default set; e.g. 'files(id,name,thumbnailLink),nextPageToken')"`
}

type DriveSearchCmd struct {
	Query     []string `arg:"" name:"query" help:"Search query"`
	RawQuery  bool     `name:"raw-query" aliases:"raw" help:"Treat query as Drive query language (pass through; may error if invalid)"`
	Max       int64    `name:"max" aliases:"limit" help:"Max results" default:"20"`
	Page      string   `name:"page" aliases:"cursor" help:"Page token"`
	AllDrives bool     `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
	Drive     string   `name:"drive" aliases:"drive-id" help:"Scope search to a specific shared drive (uses corpora=drive with driveId). Mutually exclusive with --no-all-drives. Pass the driveId from 'gog drive drives'."`
	Parent    string   `name:"parent" help:"Scope search to direct children of a specific folder or shared drive. Wraps the query with \"'<parentId>' in parents\"."`
}

type DriveGetCmd struct {
	FileID string `arg:"" name:"fileId" help:"File ID"`
	Fields string `name:"fields" help:"Drive API field mask (overrides the default set; e.g. 'id,name,thumbnailLink')"`
}

func (c *DriveGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	mask := driveFileGetFields
	if strings.TrimSpace(c.Fields) != "" {
		mask = c.Fields
	}
	f, err := svc.Files.Get(fileID).
		SupportsAllDrives(true).
		Fields(gapi.Field(mask)).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{strFile: f})
	}

	u.Out().Printf("id\t%s", f.Id)
	u.Out().Printf("name\t%s", f.Name)
	u.Out().Printf("type\t%s", f.MimeType)
	u.Out().Printf("size\t%s", formatDriveSize(f.Size))
	u.Out().Printf("created\t%s", f.CreatedTime)
	u.Out().Printf("modified\t%s", f.ModifiedTime)
	if f.Description != "" {
		u.Out().Printf("description\t%s", f.Description)
	}
	u.Out().Printf("starred\t%t", f.Starred)
	if f.WebViewLink != "" {
		u.Out().Printf("link\t%s", f.WebViewLink)
	}
	return nil
}

type DriveCopyCmd struct {
	FileID string `arg:"" name:"fileId" help:"File ID"`
	Name   string `arg:"" name:"name" help:"New file name"`
	Parent string `name:"parent" help:"Destination folder ID"`
}

func (c *DriveCopyCmd) Run(ctx context.Context, flags *RootFlags) error {
	return copyViaDrive(ctx, flags, copyViaDriveOptions{
		ArgName: "fileId",
	}, c.FileID, c.Name, c.Parent)
}

type DriveMkdirCmd struct {
	Name   string `arg:"" name:"name" help:"Folder name"`
	Parent string `name:"parent" help:"Parent folder ID"`
}

func (c *DriveMkdirCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("empty name")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	f := &drive.File{
		Name:     name,
		MimeType: "application/vnd.google-apps.folder",
	}
	if strings.TrimSpace(c.Parent) != "" {
		f.Parents = []string{strings.TrimSpace(c.Parent)}
	}

	created, err := svc.Files.Create(f).
		SupportsAllDrives(true).
		Fields("id, name, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"folder": created})
	}

	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("name\t%s", created.Name)
	if created.WebViewLink != "" {
		u.Out().Printf("link\t%s", created.WebViewLink)
	}
	return nil
}

type DriveDeleteCmd struct {
	FileID    string `arg:"" name:"fileId" help:"File ID"`
	Permanent bool   `name:"permanent" help:"Permanently delete instead of moving to trash" default:"false"`
}

func (c *DriveDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}

	action := "trash drive file"
	if c.Permanent {
		action = "permanently delete drive file"
	}
	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "drive.delete", map[string]any{
		"file_id":   fileID,
		"permanent": c.Permanent,
	}, fmt.Sprintf("%s %s", action, fileID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	trashed := !c.Permanent
	deleted := c.Permanent

	if c.Permanent {
		if err := svc.Files.Delete(fileID).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
			return err
		}
	} else {
		_, err := svc.Files.Update(fileID, &drive.File{Trashed: true}).
			SupportsAllDrives(true).
			Fields("id, trashed").
			Context(ctx).
			Do()
		if err != nil {
			return err
		}
	}
	return writeResult(ctx, u,
		kv("trashed", trashed),
		kv("deleted", deleted),
		kv("id", fileID),
	)
}

type DriveMoveCmd struct {
	FileID string `arg:"" name:"fileId" help:"File ID"`
	Parent string `name:"parent" help:"New parent folder ID (required)"`
}

func (c *DriveMoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}
	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		return usage("missing --parent")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	meta, err := svc.Files.Get(fileID).
		SupportsAllDrives(true).
		Fields("id, name, parents").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	call := svc.Files.Update(fileID, &drive.File{}).
		SupportsAllDrives(true).
		AddParents(parent).
		Fields("id, name, parents, webViewLink")
	if len(meta.Parents) > 0 {
		call = call.RemoveParents(strings.Join(meta.Parents, ","))
	}

	updated, err := call.Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{strFile: updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("name\t%s", updated.Name)
	return nil
}

type DriveRenameCmd struct {
	FileID  string `arg:"" name:"fileId" help:"File ID"`
	NewName string `arg:"" name:"newName" help:"New name"`
}

func (c *DriveRenameCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	newName := strings.TrimSpace(c.NewName)
	if fileID == "" {
		return usage("empty fileId")
	}
	if newName == "" {
		return usage("empty newName")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	updated, err := svc.Files.Update(fileID, &drive.File{Name: newName}).
		SupportsAllDrives(true).
		Fields("id, name").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{strFile: updated})
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("name\t%s", updated.Name)
	return nil
}

func buildDriveListQuery(folderID string, userQuery string) string {
	q := strings.TrimSpace(userQuery)
	parent := fmt.Sprintf("'%s' in parents", folderID)
	if q != "" {
		q = q + " and " + parent
	} else {
		q = parent
	}
	if !hasDriveTrashedPredicate(q) {
		q += " and trashed = false"
	}
	return q
}

func buildDriveAllListQuery(userQuery string) string {
	q := strings.TrimSpace(userQuery)
	if q == "" {
		return "trashed = false"
	}
	if !hasDriveTrashedPredicate(q) {
		q += " and trashed = false"
	}
	return q
}

func buildDriveSearchQuery(text string, rawQuery bool) string {
	q := strings.TrimSpace(text)
	if q == "" {
		return "trashed = false"
	}
	if rawQuery {
		return buildDriveFilterQuery(q)
	}
	if !looksLikeDriveQueryLanguage(q) {
		return fmt.Sprintf("fullText contains '%s' and trashed = false", escapeDriveQueryString(q))
	}
	return buildDriveFilterQuery(q)
}

func buildDriveFilterQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return "trashed = false"
	}
	if !hasDriveTrashedPredicate(q) {
		q += " and trashed = false"
	}
	return q
}

// Heuristic detection for Drive query-language input.
//
// Motivation: keep `gog drive search foo bar` user-friendly (fullText search)
// while still allowing power-users to paste raw Drive filters.
func looksLikeDriveQueryLanguage(q string) bool {
	if strings.EqualFold(q, "sharedWithMe") {
		return true
	}
	return driveSearchFieldComparisonPattern.MatchString(q) ||
		driveSearchContainsPattern.MatchString(q) ||
		driveSearchMembershipPattern.MatchString(q) ||
		driveSearchHasPattern.MatchString(q)
}

func hasDriveTrashedPredicate(q string) bool {
	return driveTrashedPredicatePattern.MatchString(q)
}

func escapeDriveQueryString(s string) string {
	// Escape backslashes first, then single quotes
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

func driveType(mimeType string) string {
	if mimeType == driveMimeFolder {
		return "folder"
	}
	return strFile
}

func formatDateTime(iso string) string {
	if iso == "" {
		return "-"
	}
	if len(iso) >= 16 {
		return strings.ReplaceAll(iso[:16], "T", " ")
	}
	return iso
}

func formatDriveSize(bytes int64) string {
	if bytes <= 0 {
		return "-"
	}
	const unit = 1024.0
	b := float64(bytes)
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for b >= unit && i < len(units)-1 {
		b /= unit
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f %s", b, units[i])
}

func driveFilesListCallWithDriveSupport(call *drive.FilesListCall, allDrives bool, driveID string) *drive.FilesListCall {
	// SupportsAllDrives must be set for shared drive file IDs to behave correctly.
	call = call.SupportsAllDrives(true).IncludeItemsFromAllDrives(allDrives)
	if driveID != "" {
		// Scoped search within a specific shared drive. The Drive API requires
		// corpora=drive + driveId together, and includeItemsFromAllDrives=true —
		// which is why callers must guard against driveID!="" with allDrives=false.
		call = call.Corpora("drive").DriveId(driveID)
	} else if allDrives {
		call = call.Corpora("allDrives")
	}
	return call
}
