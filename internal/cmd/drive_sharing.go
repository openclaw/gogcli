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

const (
	driveShareToAnyone = "anyone"
	driveShareToUser   = "user"
	driveShareToDomain = "domain"

	// Drive sharing permission roles matching the Google Drive API roles.
	// "commenter" allows view + comment access without edit rights.
	drivePermRoleReader    = "reader"
	drivePermRoleWriter    = "writer"
	drivePermRoleCommenter = "commenter"
)

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
