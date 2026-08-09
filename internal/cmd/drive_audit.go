package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type DriveAuditCmd struct {
	Sharing DriveAuditSharingCmd `cmd:"" name:"sharing" aliases:"permissions,perms,public,external" help:"Find public or external Drive permissions"`
	User    DriveAuditUserCmd    `cmd:"" name:"user" help:"Find Drive permissions granted to a user"`
}

type DriveAuditSharingCmd struct {
	FileID         string   `name:"file" aliases:"file-id" help:"Audit one file ID instead of a folder tree"`
	Parent         string   `name:"parent" help:"Folder ID to scan (default: root)"`
	Depth          int      `name:"depth" help:"Max folder depth (0 = unlimited)" default:"2"`
	Max            int      `name:"max" help:"Max files/folders to scan (0 = unlimited)" default:"500"`
	InternalDomain []string `name:"internal-domain" help:"Domain treated as internal (can be repeated; defaults to account email domain)"`
	PublicOnly     bool     `name:"public-only" help:"Only report anyone-with-link/public permissions"`
	ExternalOnly   bool     `name:"external-only" help:"Only report external user/group/domain permissions"`
	AllDrives      bool     `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
	FailFound      bool     `name:"fail-found" help:"Exit with code 3 when findings are present"`
}

type DriveAuditUserCmd struct {
	User      string `arg:"" name:"user" help:"User email to audit"`
	FileID    string `name:"file" aliases:"file-id" help:"Audit one file ID instead of a folder tree"`
	Parent    string `name:"parent" help:"Folder ID to scan (default: root)"`
	Depth     int    `name:"depth" help:"Max folder depth (0 = unlimited)" default:"2"`
	Max       int    `name:"max" help:"Max files/folders to scan (0 = unlimited)" default:"500"`
	AllDrives bool   `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
	FailFound bool   `name:"fail-found" help:"Exit with code 3 when findings are present"`
}

type driveSharingAuditFinding struct {
	FileID             string            `json:"fileId"`
	FileName           string            `json:"fileName,omitempty"`
	Path               string            `json:"path,omitempty"`
	MimeType           string            `json:"mimeType,omitempty"`
	WebViewLink        string            `json:"webViewLink,omitempty"`
	OwnerEmails        []string          `json:"ownerEmails,omitempty"`
	PermissionID       string            `json:"permissionId"`
	PermissionType     string            `json:"permissionType"`
	Role               string            `json:"role"`
	Email              string            `json:"email,omitempty"`
	Domain             string            `json:"domain,omitempty"`
	DisplayName        string            `json:"displayName,omitempty"`
	AllowFileDiscovery bool              `json:"allowFileDiscovery,omitempty"`
	Deleted            bool              `json:"deleted,omitempty"`
	ExpirationTime     string            `json:"expirationTime,omitempty"`
	Reasons            []string          `json:"reasons"`
	Inherited          bool              `json:"inherited,omitempty"`
	PermissionDetails  map[string]string `json:"permissionDetails,omitempty"`
}

func (c *DriveAuditUserCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	targetUser := strings.ToLower(strings.TrimSpace(c.User))
	if targetUser == "" {
		return usage("empty user")
	}
	if err := validateDriveScanBounds(c.Depth, c.Max); err != nil {
		return err
	}
	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	items, truncated, err := driveAuditItems(ctx, svc, c.FileID, c.Parent, c.Depth, c.Max, c.AllDrives)
	if err != nil {
		return err
	}

	findings := make([]driveSharingAuditFinding, 0)
	for _, item := range items {
		perms, err := listDrivePermissionsForAudit(ctx, svc, item.ID)
		if err != nil {
			return fmt.Errorf("list permissions for %s: %w", item.ID, err)
		}
		for _, perm := range perms {
			if perm == nil || strings.ToLower(strings.TrimSpace(perm.EmailAddress)) != targetUser {
				continue
			}
			findings = append(findings, drivePermissionFinding(item, perm, []string{"user"}))
		}
	}
	sortDriveSharingFindings(findings)

	if outfmt.IsJSON(ctx) {
		err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"findings":         findings,
			"findingCount":     len(findings),
			"scannedFileCount": len(items),
			"user":             targetUser,
			"truncated":        truncated,
		})
		if err != nil {
			return err
		}
		return failEmptyExit(c.FailFound && len(findings) > 0)
	}
	if len(findings) == 0 {
		u.Err().Linef("No permissions found for %s", targetUser)
		if truncated {
			u.Err().Println("Results truncated; increase --max to scan more.")
		}
		return nil
	}
	writeDriveSharingFindingsTable(ctx, findings)
	if truncated {
		u.Err().Println("Results truncated; increase --max to scan more.")
	}
	return failEmptyExit(c.FailFound)
}

func (c *DriveAuditSharingCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if c.PublicOnly && c.ExternalOnly {
		return usage("--public-only cannot be combined with --external-only")
	}
	if err := validateDriveScanBounds(c.Depth, c.Max); err != nil {
		return err
	}
	account, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	internalDomains := normalizeInternalDomains(c.InternalDomain, account)

	items, truncated, err := c.auditItems(ctx, svc)
	if err != nil {
		return err
	}

	findings := make([]driveSharingAuditFinding, 0)
	for _, item := range items {
		perms, err := listDrivePermissionsForAudit(ctx, svc, item.ID)
		if err != nil {
			return fmt.Errorf("list permissions for %s: %w", item.ID, err)
		}
		for _, perm := range perms {
			finding, ok := driveSharingFinding(item, perm, internalDomains)
			if !ok {
				continue
			}
			if c.PublicOnly && !hasReason(finding.Reasons, "public") {
				continue
			}
			if c.ExternalOnly && !hasReason(finding.Reasons, "external") {
				continue
			}
			findings = append(findings, finding)
		}
	}

	sortDriveSharingFindings(findings)

	if outfmt.IsJSON(ctx) {
		err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"findings":         findings,
			"findingCount":     len(findings),
			"scannedFileCount": len(items),
			"internalDomains":  sortedKeys(internalDomains),
			"truncated":        truncated,
		})
		if err != nil {
			return err
		}
		return failEmptyExit(c.FailFound && len(findings) > 0)
	}

	if len(findings) == 0 {
		u.Err().Println("No public or external permissions found")
		if truncated {
			u.Err().Println("Results truncated; increase --max to scan more.")
		}
		return nil
	}

	writeDriveSharingFindingsTable(ctx, findings)
	if truncated {
		u.Err().Println("Results truncated; increase --max to scan more.")
	}
	return failEmptyExit(c.FailFound)
}

func (c *DriveAuditSharingCmd) auditItems(ctx context.Context, svc *drive.Service) ([]driveTreeItem, bool, error) {
	return driveAuditItems(ctx, svc, c.FileID, c.Parent, c.Depth, c.Max, c.AllDrives)
}

func driveAuditItems(ctx context.Context, svc *drive.Service, fileIDRaw, parentRaw string, depthRaw, maxRaw int, allDrives bool) ([]driveTreeItem, bool, error) {
	fileID := strings.TrimSpace(fileIDRaw)
	if fileID != "" {
		f, err := svc.Files.Get(fileID).
			SupportsAllDrives(true).
			Fields("id,name,mimeType,webViewLink,owners(emailAddress,displayName)").
			Context(ctx).
			Do()
		if err != nil {
			return nil, false, err
		}
		return []driveTreeItem{{
			ID:          f.Id,
			Name:        f.Name,
			Path:        f.Name,
			MimeType:    f.MimeType,
			WebViewLink: f.WebViewLink,
			Owners:      driveOwners(f),
		}}, false, nil
	}
	rootID := strings.TrimSpace(parentRaw)
	if rootID == "" {
		rootID = driveRootID
	}
	limit := maxRaw
	depth := depthRaw
	return listDriveTree(ctx, svc, driveTreeOptions{
		RootID:        rootID,
		MaxDepth:      depth,
		MaxItems:      limit,
		Fields:        "id,name,mimeType,owners(emailAddress,displayName),webViewLink",
		IncludeFiles:  true,
		IncludeFolder: true,
		AllDrives:     allDrives,
	})
}

func listDrivePermissionsForAudit(ctx context.Context, svc *drive.Service, fileID string) ([]*drive.Permission, error) {
	out := make([]*drive.Permission, 0, 8)
	var pageToken string
	for {
		call := svc.Permissions.List(fileID).
			SupportsAllDrives(true).
			PageSize(100).
			Fields("nextPageToken,permissions(id,type,role,emailAddress,domain,displayName,allowFileDiscovery,deleted,expirationTime,permissionDetails(permissionType,role,inherited,inheritedFrom))").
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Permissions...)
		if resp.NextPageToken == "" {
			return out, nil
		}
		pageToken = resp.NextPageToken
	}
}

func driveSharingFinding(item driveTreeItem, perm *drive.Permission, internalDomains map[string]struct{}) (driveSharingAuditFinding, bool) {
	if perm == nil {
		return driveSharingAuditFinding{}, false
	}
	reasons := make([]string, 0, 2)
	if perm.Type == driveShareToAnyone {
		reasons = append(reasons, "public")
	}
	if isExternalDrivePermission(perm, internalDomains) {
		reasons = append(reasons, "external")
	}
	if len(reasons) == 0 {
		return driveSharingAuditFinding{}, false
	}
	f := driveSharingAuditFinding{
		FileID:         item.ID,
		FileName:       item.Name,
		Path:           item.Path,
		MimeType:       item.MimeType,
		OwnerEmails:    item.Owners,
		PermissionID:   perm.Id,
		PermissionType: perm.Type,
		Role:           perm.Role,
		Reasons:        reasons,
	}
	fillDrivePermissionFinding(&f, perm)
	for _, detail := range perm.PermissionDetails {
		if detail == nil {
			continue
		}
		if detail.Inherited {
			f.Inherited = true
		}
		if f.PermissionDetails == nil {
			f.PermissionDetails = map[string]string{}
		}
		if detail.PermissionType != "" {
			f.PermissionDetails["permissionType"] = detail.PermissionType
		}
		if detail.Role != "" {
			f.PermissionDetails["role"] = detail.Role
		}
		if detail.InheritedFrom != "" {
			f.PermissionDetails["inheritedFrom"] = detail.InheritedFrom
		}
	}
	return f, true
}

func drivePermissionFinding(item driveTreeItem, perm *drive.Permission, reasons []string) driveSharingAuditFinding {
	f := driveSharingAuditFinding{
		FileID:         item.ID,
		FileName:       item.Name,
		Path:           item.Path,
		MimeType:       item.MimeType,
		OwnerEmails:    item.Owners,
		PermissionID:   perm.Id,
		PermissionType: perm.Type,
		Role:           perm.Role,
		Reasons:        reasons,
	}
	fillDrivePermissionFinding(&f, perm)
	return f
}

func fillDrivePermissionFinding(f *driveSharingAuditFinding, perm *drive.Permission) {
	f.Email = perm.EmailAddress
	f.Domain = perm.Domain
	f.DisplayName = perm.DisplayName
	f.AllowFileDiscovery = perm.AllowFileDiscovery
	f.Deleted = perm.Deleted
	f.ExpirationTime = perm.ExpirationTime
}

func sortDriveSharingFindings(findings []driveSharingAuditFinding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path == findings[j].Path {
			return findings[i].PermissionID < findings[j].PermissionID
		}
		return findings[i].Path < findings[j].Path
	})
}

func writeDriveSharingFindingsTable(ctx context.Context, findings []driveSharingAuditFinding) {
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PATH\tREASONS\tTYPE\tROLE\tTARGET\tPERMISSION_ID")
	for _, f := range findings {
		target := f.Email
		if target == "" {
			target = f.Domain
		}
		if target == "" {
			target = f.DisplayName
		}
		if target == "" {
			target = "-"
		}
		path := f.Path
		if path == "" {
			path = f.FileName
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			sanitizeTab(path),
			strings.Join(f.Reasons, ","),
			f.PermissionType,
			f.Role,
			target,
			f.PermissionID,
		)
	}
}

func normalizeInternalDomains(input []string, account string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, raw := range input {
		for _, part := range splitCSV(raw) {
			domain := normalizeDomain(part)
			if domain != "" {
				out[domain] = struct{}{}
			}
		}
	}
	if len(out) == 0 {
		if i := strings.LastIndex(account, "@"); i >= 0 {
			if domain := normalizeDomain(account[i+1:]); domain != "" {
				out[domain] = struct{}{}
			}
		}
	}
	return out
}

func isExternalDrivePermission(perm *drive.Permission, internalDomains map[string]struct{}) bool {
	if perm == nil {
		return false
	}
	switch perm.Type {
	case driveShareToUser, "group":
		domain := emailDomain(perm.EmailAddress)
		return domain != "" && !domainAllowed(domain, internalDomains)
	case driveShareToDomain:
		domain := normalizeDomain(perm.Domain)
		return domain != "" && !domainAllowed(domain, internalDomains)
	default:
		return false
	}
}

func emailDomain(email string) string {
	i := strings.LastIndex(strings.TrimSpace(email), "@")
	if i < 0 {
		return ""
	}
	return normalizeDomain(email[i+1:])
}

func normalizeDomain(domain string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
}

func domainAllowed(domain string, allowed map[string]struct{}) bool {
	domain = normalizeDomain(domain)
	if domain == "" {
		return false
	}
	_, ok := allowed[domain]
	return ok
}

func hasReason(reasons []string, reason string) bool {
	for _, r := range reasons {
		if r == reason {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
