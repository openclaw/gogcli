package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type DriveBulkCmd struct {
	RemovePublic DriveBulkRemovePublicCmd `cmd:"" name:"remove-public" help:"Remove anyone/public permissions across files"`
	UpdateRole   DriveBulkUpdateRoleCmd   `cmd:"" name:"update-role" help:"Change matching Drive permission roles across files"`
}

type DriveBulkRemovePublicCmd struct {
	FileID    string `name:"file" aliases:"file-id" help:"Update one file ID instead of a folder tree"`
	Parent    string `name:"parent" help:"Folder ID to scan (default: root)"`
	Depth     int    `name:"depth" help:"Max folder depth (0 = unlimited)" default:"2"`
	Max       int    `name:"max" help:"Max files/folders to scan (0 = unlimited)" default:"500"`
	AllDrives bool   `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
}

type DriveBulkUpdateRoleCmd struct {
	FileID    string `name:"file" aliases:"file-id" help:"Update one file ID instead of a folder tree"`
	Parent    string `name:"parent" help:"Folder ID to scan (default: root)"`
	Depth     int    `name:"depth" help:"Max folder depth (0 = unlimited)" default:"2"`
	Max       int    `name:"max" help:"Max files/folders to scan (0 = unlimited)" default:"500"`
	AllDrives bool   `name:"all-drives" help:"Include shared drives (default: true; use --no-all-drives for My Drive only)" default:"true" negatable:"_"`
	From      string `name:"from" help:"Current role to match (reader|commenter|writer)"`
	To        string `name:"to" help:"New role (reader|commenter|writer)"`
	Type      string `name:"type" help:"Optional permission type filter (user|group|domain|anyone)"`
	Target    string `name:"target" help:"Optional target email/domain filter"`
}

type driveBulkPermissionPlan struct {
	FileID       string `json:"fileId"`
	FileName     string `json:"fileName,omitempty"`
	Path         string `json:"path,omitempty"`
	PermissionID string `json:"permissionId"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	NewRole      string `json:"newRole,omitempty"`
	Target       string `json:"target,omitempty"`
	Inherited    bool   `json:"inherited,omitempty"`
	Action       string `json:"action"`
}

func (c *DriveBulkRemovePublicCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if boundsErr := validateDriveScanBounds(c.Depth, c.Max); boundsErr != nil {
		return boundsErr
	}
	request := driveBulkScanRequest(c.FileID, c.Parent, c.Depth, c.Max, c.AllDrives)
	if err := dryRunExit(ctx, flags, "drive.bulk.remove-public", request); err != nil {
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
	plans, err := collectDriveBulkPlans(ctx, svc, items, func(perm *drive.Permission) (string, bool) {
		return "", perm != nil && perm.Type == driveShareToAnyone
	})
	if err != nil {
		return err
	}
	for i := range plans {
		plans[i].Action = "remove"
	}
	if err := driveBulkConfirm(ctx, flags, plans, "remove public Drive permissions"); err != nil {
		return err
	}
	for _, plan := range plans {
		if plan.Inherited {
			continue
		}
		if err := svc.Permissions.Delete(plan.FileID, plan.PermissionID).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
			return fmt.Errorf("remove permission %s from %s: %w", plan.PermissionID, plan.FileID, err)
		}
	}
	return writeDriveBulkResult(ctx, u, plans, truncated)
}

func (c *DriveBulkUpdateRoleCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	from, err := normalizeDriveBulkRole(c.From, "--from")
	if err != nil {
		return err
	}
	to, err := normalizeDriveBulkRole(c.To, "--to")
	if err != nil {
		return err
	}
	if from == to {
		return usage("--from and --to must differ")
	}
	if boundsErr := validateDriveScanBounds(c.Depth, c.Max); boundsErr != nil {
		return boundsErr
	}
	typeFilter := strings.ToLower(strings.TrimSpace(c.Type))
	targetFilter := strings.ToLower(strings.TrimSpace(c.Target))
	request := driveBulkScanRequest(c.FileID, c.Parent, c.Depth, c.Max, c.AllDrives)
	request["from"] = from
	request["to"] = to
	request["type"] = typeFilter
	request["target"] = targetFilter
	if dryRunErr := dryRunExit(ctx, flags, "drive.bulk.update-role", request); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	items, truncated, err := driveAuditItems(ctx, svc, c.FileID, c.Parent, c.Depth, c.Max, c.AllDrives)
	if err != nil {
		return err
	}
	plans, err := collectDriveBulkPlans(ctx, svc, items, func(perm *drive.Permission) (string, bool) {
		if perm == nil || perm.Role != from {
			return "", false
		}
		if typeFilter != "" && perm.Type != typeFilter {
			return "", false
		}
		if targetFilter != "" && !drivePermissionTargetMatches(perm, targetFilter) {
			return "", false
		}
		return to, true
	})
	if err != nil {
		return err
	}
	for i := range plans {
		plans[i].Action = "updateRole"
		plans[i].NewRole = to
	}
	if err := driveBulkConfirm(ctx, flags, plans, fmt.Sprintf("update %d Drive permission roles from %s to %s", len(plans), from, to)); err != nil {
		return err
	}
	for _, plan := range plans {
		if plan.Inherited {
			continue
		}
		if _, err := svc.Permissions.Update(plan.FileID, plan.PermissionID, &drive.Permission{Role: plan.NewRole}).
			SupportsAllDrives(true).
			Fields("id,role").
			Context(ctx).
			Do(); err != nil {
			return fmt.Errorf("update permission %s on %s: %w", plan.PermissionID, plan.FileID, err)
		}
	}
	return writeDriveBulkResult(ctx, u, plans, truncated)
}

func collectDriveBulkPlans(ctx context.Context, svc *drive.Service, items []driveTreeItem, include func(*drive.Permission) (string, bool)) ([]driveBulkPermissionPlan, error) {
	plans := make([]driveBulkPermissionPlan, 0)
	for _, item := range items {
		perms, err := listDrivePermissionsForAudit(ctx, svc, item.ID)
		if err != nil {
			return nil, fmt.Errorf("list permissions for %s: %w", item.ID, err)
		}
		for _, perm := range perms {
			newRole, ok := include(perm)
			if !ok {
				continue
			}
			plans = append(plans, driveBulkPermissionPlan{
				FileID:       item.ID,
				FileName:     item.Name,
				Path:         item.Path,
				PermissionID: perm.Id,
				Type:         perm.Type,
				Role:         perm.Role,
				NewRole:      newRole,
				Target:       drivePermissionTarget(perm),
				Inherited:    drivePermissionInherited(perm),
			})
		}
	}
	return plans, nil
}

func driveBulkConfirm(ctx context.Context, flags *RootFlags, plans []driveBulkPermissionPlan, action string) error {
	if len(plans) == 0 {
		return nil
	}
	return confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), action)
}

func driveBulkScanRequest(fileID, parent string, depth, maxItems int, allDrives bool) map[string]any {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		parent = "root"
	}
	return map[string]any{
		"file_id":    strings.TrimSpace(fileID),
		"parent":     parent,
		"depth":      depth,
		"max":        maxItems,
		"all_drives": allDrives,
	}
}

func writeDriveBulkResult(ctx context.Context, u *ui.UI, plans []driveBulkPermissionPlan, truncated bool) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"items":     plans,
			"count":     len(plans),
			"truncated": truncated,
		})
	}
	if len(plans) == 0 {
		u.Err().Println("No matching permissions")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PATH\tACTION\tTYPE\tROLE\tNEW_ROLE\tTARGET\tPERMISSION_ID")
	for _, p := range plans {
		path := p.Path
		if path == "" {
			path = p.FileName
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", sanitizeTab(path), p.Action, p.Type, p.Role, p.NewRole, p.Target, p.PermissionID)
	}
	if truncated {
		u.Err().Println("Results truncated; increase --max to scan more.")
	}
	return nil
}

func normalizeDriveBulkRole(raw string, flag string) (string, error) {
	role := strings.ToLower(strings.TrimSpace(raw))
	switch role {
	case drivePermRoleReader, drivePermRoleCommenter, drivePermRoleWriter:
		return role, nil
	case "":
		return "", usagef("%s is required", flag)
	default:
		return "", usagef("invalid %s (expected reader|commenter|writer)", flag)
	}
}

func drivePermissionTarget(perm *drive.Permission) string {
	if perm == nil {
		return ""
	}
	if strings.TrimSpace(perm.EmailAddress) != "" {
		return perm.EmailAddress
	}
	if strings.TrimSpace(perm.Domain) != "" {
		return perm.Domain
	}
	if strings.TrimSpace(perm.DisplayName) != "" {
		return perm.DisplayName
	}
	return ""
}

func drivePermissionTargetMatches(perm *drive.Permission, target string) bool {
	return strings.EqualFold(strings.TrimSpace(drivePermissionTarget(perm)), strings.TrimSpace(target))
}

func drivePermissionInherited(perm *drive.Permission) bool {
	if perm == nil {
		return false
	}
	for _, detail := range perm.PermissionDetails {
		if detail != nil && detail.Inherited {
			return true
		}
	}
	return false
}
