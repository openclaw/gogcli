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

// LookerStudioCmd manages Looker Studio assets via Drive permissions.
type LookerStudioCmd struct {
	Permissions LookerStudioPermissionsCmd `cmd:"" name:"permissions" help:"Manage Looker Studio permissions"`
}

type LookerStudioPermissionsCmd struct {
	List   LookerStudioPermissionsListCmd   `cmd:"" name:"list" help:"List permissions for a Looker Studio asset"`
	Add    LookerStudioPermissionsAddCmd    `cmd:"" name:"add" help:"Add a permission to a Looker Studio asset"`
	Remove LookerStudioPermissionsRemoveCmd `cmd:"" name:"remove" help:"Remove a permission from a Looker Studio asset"`
}

type LookerStudioPermissionsListCmd struct {
	AssetID string `arg:"" name:"asset-id" help:"Looker Studio asset ID (Drive file ID)"`
	User    string `name:"user" help:"User email to list permissions as"`
}

func (c *LookerStudioPermissionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	assetID := strings.TrimSpace(c.AssetID)
	if assetID == "" {
		return usage("asset-id is required")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Permissions.List(assetID).SupportsAllDrives(true).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list permissions: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Permissions) == 0 {
		u.Err().Println("No permissions found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTYPE\tROLE\tEMAIL")
	for _, perm := range resp.Permissions {
		if perm == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(perm.Id),
			sanitizeTab(perm.Type),
			sanitizeTab(perm.Role),
			sanitizeTab(perm.EmailAddress),
		)
	}
	return nil
}

type LookerStudioPermissionsAddCmd struct {
	AssetID string `arg:"" name:"asset-id" help:"Looker Studio asset ID (Drive file ID)"`
	User    string `name:"user" help:"User email to apply permission as"`
	Email   string `name:"email" help:"User email to grant access" required:""`
	Role    string `name:"role" help:"Permission role: VIEWER|EDITOR" default:"VIEWER"`
}

func (c *LookerStudioPermissionsAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	assetID := strings.TrimSpace(c.AssetID)
	if assetID == "" {
		return usage("asset-id is required")
	}
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("--email is required")
	}

	role := strings.ToUpper(strings.TrimSpace(c.Role))
	var driveRole string
	switch role {
	case "VIEWER":
		driveRole = "reader"
	case "EDITOR":
		driveRole = "writer"
	default:
		return usage("invalid --role (expected VIEWER|EDITOR)")
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	perm := &drive.Permission{Type: "user", Role: driveRole, EmailAddress: email}
	created, err := svc.Permissions.Create(assetID, perm).
		SupportsAllDrives(true).
		SendNotificationEmail(false).
		Fields("id, type, role, emailAddress").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("add permission: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Added permission: %s (%s)\n", created.Id, email)
	return nil
}

type LookerStudioPermissionsRemoveCmd struct {
	AssetID      string `arg:"" name:"asset-id" help:"Looker Studio asset ID (Drive file ID)"`
	PermissionID string `arg:"" name:"permission-id" help:"Permission ID"`
	User         string `name:"user" help:"User email to apply permission as"`
}

func (c *LookerStudioPermissionsRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	assetID := strings.TrimSpace(c.AssetID)
	permissionID := strings.TrimSpace(c.PermissionID)
	if assetID == "" || permissionID == "" {
		return usage("asset-id and permission-id are required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove permission %s from asset %s", permissionID, assetID)); err != nil {
		return err
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Permissions.Delete(assetID, permissionID).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
		return fmt.Errorf("remove permission: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"removed": true, "permissionId": permissionID})
	}

	u.Out().Printf("Removed permission: %s\n", permissionID)
	return nil
}
