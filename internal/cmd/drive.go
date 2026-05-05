package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	driveShareToAnyone     = "anyone"
	driveShareToUser       = "user"
	driveShareToDomain     = "domain"

	// Drive sharing permission roles matching the Google Drive API roles.
	// "commenter" allows view + comment access without edit rights.
	drivePermRoleReader    = "reader"
	drivePermRoleWriter    = "writer"
	drivePermRoleCommenter = "commenter"
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

// driveRawSensitiveFields is the set of top-level File fields redacted from
// `gog drive raw` output when the user did not name them via --fields. See
// docs/raw-audit.md for the rationale per field.
var driveRawSensitiveFields = []string{
	"thumbnailLink",
	"webContentLink",
	"exportLinks",
	"resourceKey",
	"appProperties",
	"properties",
}

// DriveRawCmd dumps the full Files.Get response as JSON. Uses fields=* by
// default to expose the entire File resource. When --fields is absent the
// command redacts a small set of capability/token-shaped fields (see
// driveRawSensitiveFields); when --fields is explicitly set the response is
// returned verbatim, honoring exactly what the user asked for. This means
// passing `--fields "id,name,thumbnailLink"` returns thumbnailLink as
// requested.
//
// REST reference: https://developers.google.com/drive/api/reference/rest/v3/files/get
// Go type: https://pkg.go.dev/google.golang.org/api/drive/v3#File
type DriveRawCmd struct {
	FileID string `arg:"" name:"fileId" help:"File ID"`
	Fields string `name:"fields" help:"Drive API field mask (default: * with sensitive fields redacted client-side). Set explicitly to disable redaction."`
	Pretty bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *DriveRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	userSetFields := strings.TrimSpace(c.Fields) != ""
	mask := "*"
	if userSetFields {
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
	if f == nil {
		return errors.New("file not found")
	}

	// Round-trip through JSON so we can redact by key when needed.
	raw, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal drive file: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("unmarshal drive file: %w", err)
	}

	// Redact only when the user did not explicitly request fields.
	if !userSetFields {
		for _, key := range driveRawSensitiveFields {
			delete(m, key)
		}
		// contentHints.thumbnail.image is the one nested leak.
		if hints, ok := m["contentHints"].(map[string]any); ok {
			if thumb, ok := hints["thumbnail"].(map[string]any); ok {
				delete(thumb, "image")
			}
		}
	}

	return outfmt.WriteRaw(ctx, os.Stdout, m, outfmt.RawOptions{Pretty: c.Pretty})
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

type DriveDownloadCmd struct {
	FileID string         `arg:"" name:"fileId" help:"File ID"`
	Output OutputPathFlag `embed:""`
	Format string         `name:"format" help:"Export format for Google Docs files: pdf|csv|xlsx|pptx|txt|png|docx|md (default: inferred)"`
	Tab    string         `name:"tab" help:"(experimental) Export a specific tab by title or ID (Google Docs only; see 'gog docs list-tabs')"`
}

func (c *DriveDownloadCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	fileID := normalizeGoogleID(strings.TrimSpace(c.FileID))
	if fileID == "" {
		return usage("empty fileId")
	}

	if tab := strings.TrimSpace(c.Tab); tab != "" {
		if f := c.Format; f != "" && f != formatAuto {
			if _, fmtErr := tabExportFormatParam(f); fmtErr != nil {
				return fmt.Errorf("--tab limits export formats (pdf|docx|txt|md|html); %q is not supported with --tab", f)
			}
		}
		return runDocsTabExport(ctx, flags, tabExportParams{
			DocID:    fileID,
			OutFlag:  c.Output.Path,
			Format:   c.Format,
			TabQuery: tab,
		})
	}

	u := ui.FromContext(ctx)
	if formatErr := validateDriveDownloadFormatFlag(c.Format); formatErr != nil {
		return formatErr
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	meta, err := svc.Files.Get(fileID).
		SupportsAllDrives(true).
		Fields("id, name, mimeType").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if meta.Name == "" {
		return errors.New("file has no name")
	}
	if fileFormatErr := validateDriveDownloadFormatForFile(meta, c.Format); fileFormatErr != nil {
		return fileFormatErr
	}

	destPath, err := resolveDriveDownloadDestPath(meta, c.Output.Path)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) && isStdoutPath(destPath) {
		return usage("can't combine --json with --out -")
	}

	downloadedPath, size, err := downloadDriveFile(ctx, svc, meta, destPath, c.Format)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"path": downloadedPath,
			"size": size,
		})
	}
	if isStdoutPath(downloadedPath) {
		return nil
	}

	u.Out().Printf("path\t%s", downloadedPath)
	u.Out().Printf("size\t%s", formatDriveSize(size))
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

type DriveUploadCmd struct {
	LocalPath           string `arg:"" name:"localPath" help:"Path to local file"`
	Name                string `name:"name" help:"Override filename (create) or rename target (replace)"`
	Parent              string `name:"parent" help:"Destination folder ID (create only)"`
	ReplaceFileID       string `name:"replace" help:"Replace the content of an existing Drive file ID (preserves shared link/permissions)"`
	MimeType            string `name:"mime-type" help:"Override MIME type inference"`
	KeepRevisionForever bool   `name:"keep-revision-forever" help:"Keep the new head revision forever (binary files only)"`
	Convert             bool   `name:"convert" help:"Auto-convert to native Google format based on file extension (create only)"`
	ConvertTo           string `name:"convert-to" help:"Convert to a specific Google format: doc|sheet|slides (create only)"`
	KeepFrontmatter     bool   `name:"keep-frontmatter" help:"Keep YAML frontmatter (---) in Markdown when converting to a Google Doc (--convert or --convert-to doc; default: strip)"`
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

type DriveShareCmd struct {
	FileID       string `arg:"" name:"fileId" help:"File ID"`
	To           string `name:"to" help:"Share target: anyone|user|domain"`
	Anyone       bool   `name:"anyone" hidden:"" help:"(deprecated) Use --to=anyone"`
	Email        string `name:"email" help:"User email (for --to=user)"`
	Domain       string `name:"domain" help:"Domain (for --to=domain; e.g. example.com)"`
	Role         string `name:"role" help:"Permission: reader|writer|commenter" default:"reader"`
	Discoverable bool   `name:"discoverable" help:"Allow file discovery in search (anyone/domain only)"`
}

type driveShareTarget struct {
	to     string
	email  string
	domain string
}

func (c *DriveShareCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}

	target, err := c.normalizeTarget()
	if err != nil {
		return err
	}
	role, err := normalizeDrivePermissionRole(c.Role)
	if err != nil {
		return err
	}
	if target.to == driveShareToAnyone {
		if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("share drive file %s with anyone (public)", fileID)); confirmErr != nil {
			return confirmErr
		}
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	perm := target.permission(role, c.Discoverable)

	created, err := svc.Permissions.Create(fileID, perm).
		SupportsAllDrives(true).
		SendNotificationEmail(false).
		Fields("id, type, role, emailAddress, domain, allowFileDiscovery").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	link, err := driveWebLink(ctx, svc, fileID)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"link":         link,
			"permissionId": created.Id,
			"permission":   created,
		})
	}

	u.Out().Printf("link\t%s", link)
	u.Out().Printf("permission_id\t%s", created.Id)
	return nil
}

func (c *DriveShareCmd) normalizeTarget() (driveShareTarget, error) {
	to := strings.TrimSpace(c.To)
	email := strings.TrimSpace(c.Email)
	domain := strings.TrimSpace(c.Domain)

	// Back-compat: allow legacy target flags without --to, but keep it unambiguous.
	// New UX: prefer explicit --to + matching parameter.
	if to == "" {
		switch {
		case c.Anyone && email == "" && domain == "":
			to = driveShareToAnyone
		case !c.Anyone && email != "" && domain == "":
			to = driveShareToUser
		case !c.Anyone && email == "" && domain != "":
			to = driveShareToDomain
		case !c.Anyone && email == "" && domain == "":
			return driveShareTarget{}, usage("must specify --to (anyone|user|domain)")
		default:
			return driveShareTarget{}, usage("ambiguous share target (use --to=anyone|user|domain)")
		}
	}

	switch to {
	case driveShareToAnyone:
		if email != "" || domain != "" {
			return driveShareTarget{}, usage("--to=anyone cannot be combined with --email or --domain")
		}
	case driveShareToUser:
		if email == "" {
			return driveShareTarget{}, usage("missing --email for --to=user")
		}
		if domain != "" || c.Anyone {
			return driveShareTarget{}, usage("--to=user cannot be combined with --anyone or --domain")
		}
		if c.Discoverable {
			return driveShareTarget{}, usage("--discoverable is only valid for --to=anyone or --to=domain")
		}
	case driveShareToDomain:
		if domain == "" {
			return driveShareTarget{}, usage("missing --domain for --to=domain")
		}
		if email != "" || c.Anyone {
			return driveShareTarget{}, usage("--to=domain cannot be combined with --anyone or --email")
		}
	default:
		// Should be guarded by enum, but keep a friendly message for future changes.
		return driveShareTarget{}, usage("invalid --to (expected anyone|user|domain)")
	}

	return driveShareTarget{to: to, email: email, domain: domain}, nil
}

func (target driveShareTarget) permission(role string, discoverable bool) *drive.Permission {
	perm := &drive.Permission{Role: role}
	switch target.to {
	case driveShareToAnyone:
		perm.Type = "anyone"
		perm.AllowFileDiscovery = discoverable
	case driveShareToDomain:
		perm.Type = "domain"
		perm.Domain = target.domain
		perm.AllowFileDiscovery = discoverable
	default:
		perm.Type = "user"
		perm.EmailAddress = target.email
	}
	return perm
}

func normalizeDrivePermissionRole(role string) (string, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return drivePermRoleReader, nil
	}
	switch role {
	case drivePermRoleReader, drivePermRoleWriter, drivePermRoleCommenter:
		return role, nil
	default:
		return "", usage("invalid --role (expected reader|writer|commenter)")
	}
}

type DriveUnshareCmd struct {
	FileID       string `arg:"" name:"fileId" help:"File ID"`
	PermissionID string `arg:"" name:"permissionId" help:"Permission ID"`
}

func (c *DriveUnshareCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	fileID := strings.TrimSpace(c.FileID)
	permissionID := strings.TrimSpace(c.PermissionID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if permissionID == "" {
		return usage("empty permissionId")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("remove permission %s from drive file %s", permissionID, fileID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Permissions.Delete(fileID, permissionID).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
		return err
	}

	return writeResult(ctx, u,
		kv("removed", true),
		kv("fileId", fileID),
		kv("permissionId", permissionID),
	)
}

type DrivePermissionsCmd struct {
	FileID string `arg:"" name:"fileId" help:"File ID"`
	Max    int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page   string `name:"page" aliases:"cursor" help:"Page token"`
}

func (c *DrivePermissionsCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	call := svc.Permissions.List(fileID).
		SupportsAllDrives(true).
		Fields("nextPageToken, permissions(id, type, role, emailAddress, domain)").
		Context(ctx)
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if strings.TrimSpace(c.Page) != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"fileId":          fileID,
			"permissions":     resp.Permissions,
			"permissionCount": len(resp.Permissions),
			"nextPageToken":   resp.NextPageToken,
		})
	}
	if len(resp.Permissions) == 0 {
		u.Err().Println("No permissions")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTYPE\tROLE\tEMAIL")
	for _, p := range resp.Permissions {
		email := p.EmailAddress
		if email == "" && p.Domain != "" {
			email = p.Domain
		}
		if email == "" {
			email = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Id, p.Type, p.Role, email)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type DriveURLCmd struct {
	FileIDs []string `arg:"" name:"fileId" help:"File IDs"`
}

func (c *DriveURLCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	for _, id := range c.FileIDs {
		link, err := driveWebLink(ctx, svc, id)
		if err != nil {
			return err
		}
		if outfmt.IsJSON(ctx) {
			// collected below
		} else {
			u.Out().Printf("%s\t%s", id, link)
		}
	}
	if outfmt.IsJSON(ctx) {
		urls := make([]map[string]string, 0, len(c.FileIDs))
		for _, id := range c.FileIDs {
			link, err := driveWebLink(ctx, svc, id)
			if err != nil {
				return err
			}
			urls = append(urls, map[string]string{"id": id, "url": link})
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"urls": urls})
	}
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

func guessMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case extPDF:
		return mimePDF
	case ".doc":
		return "application/msword"
	case extDocx:
		return mimeDocx
	case ".xls":
		return "application/vnd.ms-excel"
	case extXlsx:
		return mimeXlsx
	case ".ppt":
		return "application/vnd.ms-powerpoint"
	case extPptx:
		return mimePptx
	case extPNG:
		return mimePNG
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case extTXT:
		return mimeTextPlain
	case ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".zip":
		return "application/zip"
	case ".csv":
		return "text/csv"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}

func downloadDriveFile(ctx context.Context, svc *drive.Service, meta *drive.File, destPath string, format string) (string, int64, error) {
	isGoogleDoc := strings.HasPrefix(meta.MimeType, "application/vnd.google-apps.")
	normalizedFormat := strings.ToLower(strings.TrimSpace(format))
	if normalizedFormat == formatAuto {
		normalizedFormat = ""
	}

	if !isGoogleDoc && normalizedFormat != "" {
		return "", 0, fmt.Errorf("--format %q not supported for non-Google Workspace files (mimeType=%q); file can only be downloaded as-is", format, meta.MimeType)
	}
	if fileFormatErr := validateDriveDownloadFormatForFile(meta, format); fileFormatErr != nil {
		return "", 0, fileFormatErr
	}

	var (
		resp    *http.Response
		outPath string
		err     error
	)

	if isGoogleDoc {
		var exportMimeType string
		if normalizedFormat == "" {
			exportMimeType = driveExportMimeType(meta.MimeType)
		} else {
			var mimeErr error
			exportMimeType, mimeErr = driveExportMimeTypeForFormat(meta.MimeType, normalizedFormat)
			if mimeErr != nil {
				return "", 0, mimeErr
			}
		}
		if isStdoutPath(destPath) {
			outPath = stdoutPath
		} else {
			outPath = replaceExt(destPath, driveExportExtension(exportMimeType))
		}
		resp, err = driveExportDownload(ctx, svc, meta.Id, exportMimeType)
	} else {
		outPath = destPath
		resp, err = driveDownload(ctx, svc, meta.Id)
	}
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("download failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	if isStdoutPath(outPath) {
		n, copyErr := io.Copy(os.Stdout, resp.Body)
		return stdoutPath, n, copyErr
	}

	f, outPath, err := createUserOutputFile(outPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return "", 0, err
	}
	return outPath, n, nil
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

func validateDriveDownloadFormatFlag(format string) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return nil
	}
	switch format {
	case "pdf", "csv", "xlsx", "pptx", "txt", "png", "docx", "md", "html":
		return nil
	default:
		return usagef("invalid --format %q (use pdf|csv|xlsx|pptx|txt|png|docx|md|html)", format)
	}
}

func validateDriveDownloadFormatForFile(meta *drive.File, format string) error {
	if meta == nil {
		return errors.New("missing file metadata")
	}
	isGoogleDoc := strings.HasPrefix(meta.MimeType, "application/vnd.google-apps.")
	if isGoogleDoc {
		return nil
	}
	if strings.TrimSpace(format) == "" {
		return nil
	}
	return fmt.Errorf("--format %q not supported for non-Google Workspace files (mimeType=%q); file can only be downloaded as-is", format, meta.MimeType)
}

var driveDownload = func(ctx context.Context, svc *drive.Service, fileID string) (*http.Response, error) {
	return svc.Files.Get(fileID).SupportsAllDrives(true).Context(ctx).Download()
}

var driveExportDownload = func(ctx context.Context, svc *drive.Service, fileID string, mimeType string) (*http.Response, error) {
	return svc.Files.Export(fileID, mimeType).Context(ctx).Download()
}

func replaceExt(path string, ext string) string {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	return base + ext
}

func driveExportMimeType(googleMimeType string) string {
	switch googleMimeType {
	case driveMimeGoogleDoc:
		return mimePDF
	case driveMimeGoogleSheet:
		return mimeCSV
	case driveMimeGoogleSlides:
		return mimePDF
	case driveMimeGoogleDrawing:
		return mimePNG
	default:
		return mimePDF
	}
}

func driveExportMimeTypeForFormat(googleMimeType string, format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" || format == formatAuto {
		return driveExportMimeType(googleMimeType), nil
	}

	switch googleMimeType {
	case driveMimeGoogleDoc:
		switch format {
		case defaultExportFormat:
			return mimePDF, nil
		case "docx":
			return mimeDocx, nil
		case "txt":
			return mimeTextPlain, nil
		case "md":
			return mimeTextMarkdown, nil
		case "html":
			return mimeHTML, nil
		default:
			return "", fmt.Errorf("invalid --format %q for Google Doc (use pdf|docx|txt|md|html)", format)
		}
	case driveMimeGoogleSheet:
		switch format {
		case defaultExportFormat:
			return mimePDF, nil
		case "csv":
			return mimeCSV, nil
		case "xlsx":
			return mimeXlsx, nil
		default:
			return "", fmt.Errorf("invalid --format %q for Google Sheet (use pdf|csv|xlsx)", format)
		}
	case driveMimeGoogleSlides:
		switch format {
		case defaultExportFormat:
			return mimePDF, nil
		case "pptx":
			return mimePptx, nil
		default:
			return "", fmt.Errorf("invalid --format %q for Google Slides (use pdf|pptx)", format)
		}
	case driveMimeGoogleDrawing:
		switch format {
		case "png":
			return mimePNG, nil
		case defaultExportFormat:
			return mimePDF, nil
		default:
			return "", fmt.Errorf("invalid --format %q for Google Drawing (use png|pdf)", format)
		}
	default:
		if format == defaultExportFormat {
			return mimePDF, nil
		}
		return "", fmt.Errorf("invalid --format %q for file type %q (use pdf)", format, googleMimeType)
	}
}

func driveExportExtension(mimeType string) string {
	switch mimeType {
	case mimePDF:
		return extPDF
	case mimeCSV:
		return extCSV
	case mimeXlsx:
		return extXlsx
	case mimeDocx:
		return extDocx
	case mimePptx:
		return extPptx
	case mimePNG:
		return extPNG
	case mimeTextPlain:
		return extTXT
	case mimeTextMarkdown:
		return extMD
	case mimeHTML:
		return extHTML
	default:
		return extPDF
	}
}

// googleConvertMimeType returns the Google-native MIME type for convertible
// Office/text formats. The boolean indicates whether the extension is supported.
func googleConvertMimeType(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case extDocx, ".doc":
		return driveMimeGoogleDoc, true
	case extXlsx, ".xls", extCSV:
		return driveMimeGoogleSheet, true
	case extPptx, ".ppt":
		return driveMimeGoogleSlides, true
	case extTXT, ".html", extMD:
		return driveMimeGoogleDoc, true
	default:
		return "", false
	}
}

func googleConvertTargetMimeType(target string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "doc":
		return driveMimeGoogleDoc, true
	case "sheet":
		return driveMimeGoogleSheet, true
	case "slides":
		return driveMimeGoogleSlides, true
	default:
		return "", false
	}
}

func driveUploadConvertMimeType(path string, auto bool, target string) (string, bool, error) {
	target = strings.TrimSpace(target)
	if target != "" {
		mimeType, ok := googleConvertTargetMimeType(target)
		if !ok {
			return "", false, fmt.Errorf("--convert-to: invalid value %q (use doc|sheet|slides)", target)
		}
		return mimeType, true, nil
	}
	if !auto {
		return "", false, nil
	}

	mimeType, ok := googleConvertMimeType(path)
	if !ok {
		return "", false, fmt.Errorf("--convert: unsupported file type %q (supported: docx, xlsx, pptx, doc, xls, ppt, csv, txt, html, md)", filepath.Ext(path))
	}
	return mimeType, true, nil
}

// stripOfficeExt removes common Office extensions from a filename so
// the resulting Google Doc/Sheet/Slides has a clean name.
func stripOfficeExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case extDocx, ".doc", extXlsx, ".xls", extPptx, ".ppt", extMD:
		return strings.TrimSuffix(name, filepath.Ext(name))
	default:
		return name
	}
}

func driveWebLink(ctx context.Context, svc *drive.Service, fileID string) (string, error) {
	f, err := svc.Files.Get(fileID).SupportsAllDrives(true).Fields("webViewLink").Context(ctx).Do()
	if err != nil {
		return "", err
	}
	if f.WebViewLink != "" {
		return f.WebViewLink, nil
	}
	return fmt.Sprintf("https://drive.google.com/file/d/%s/view", fileID), nil
}
