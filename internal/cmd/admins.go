package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type AdminsCmd struct {
	List   AdminsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List admin role assignments"`
	Create AdminsCreateCmd `cmd:"" name:"create" aliases:"add" help:"Assign admin role"`
	Delete AdminsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete admin assignment"`
}

type AdminsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *AdminsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	call := svc.RoleAssignments.List(adminCustomerID()).MaxResults(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list admin assignments: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No admin assignments found")
		return nil
	}

	roleNames, _ := roleIDNameMap(ctx, svc)

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ASSIGNMENT ID\tROLE\tASSIGNED TO\tSCOPE\tORG UNIT")
	for _, assignment := range resp.Items {
		if assignment == nil {
			continue
		}
		roleID := strconv.FormatInt(assignment.RoleId, 10)
		roleName := roleNames[roleID]
		if roleName == "" {
			roleName = roleID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			sanitizeTab(strconv.FormatInt(assignment.RoleAssignmentId, 10)),
			sanitizeTab(roleName),
			sanitizeTab(assignment.AssignedTo),
			sanitizeTab(assignment.ScopeType),
			sanitizeTab(assignment.OrgUnitId),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type AdminsCreateCmd struct {
	User    string `arg:"" name:"user" help:"User email or ID"`
	Role    string `name:"role" required:"" help:"Role ID or name"`
	OrgUnit string `name:"org-unit" aliases:"ou" help:"Org unit path (scope)"`
}

func (c *AdminsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	roleID, _, err := resolveRole(ctx, svc, c.Role)
	if err != nil {
		return err
	}
	roleIDNum, err := strconv.ParseInt(roleID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid role id %q: %w", roleID, err)
	}

	assignedTo, err := resolveUserID(ctx, svc, c.User)
	if err != nil {
		return err
	}

	assignment := &admin.RoleAssignment{
		RoleId:     roleIDNum,
		AssignedTo: assignedTo,
		ScopeType:  "CUSTOMER",
	}
	if strings.TrimSpace(c.OrgUnit) != "" {
		orgID, err := resolveOrgUnitID(ctx, svc, c.OrgUnit)
		if err != nil {
			return err
		}
		assignment.ScopeType = "ORG_UNIT"
		assignment.OrgUnitId = orgID
	}

	created, err := svc.RoleAssignments.Insert(adminCustomerID(), assignment).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("assign role %s to %s: %w", c.Role, c.User, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Assigned role %s to %s (assignment %d)\n", c.Role, c.User, created.RoleAssignmentId)
	return nil
}

type AdminsDeleteCmd struct {
	AssignmentID string `arg:"" name:"assignment-id" help:"Role assignment ID"`
}

func (c *AdminsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete admin assignment %s", c.AssignmentID)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.RoleAssignments.Delete(adminCustomerID(), c.AssignmentID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete admin assignment %s: %w", c.AssignmentID, err)
	}

	u.Out().Printf("Deleted admin assignment: %s\n", c.AssignmentID)
	return nil
}

func roleIDNameMap(ctx context.Context, svc *admin.Service) (map[string]string, error) {
	roles, err := listAllRoles(ctx, svc)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(roles))
	for _, role := range roles {
		if role == nil {
			continue
		}
		out[strconv.FormatInt(role.RoleId, 10)] = role.RoleName
	}
	return out, nil
}

func resolveUserID(ctx context.Context, svc *admin.Service, user string) (string, error) {
	user = strings.TrimSpace(user)
	if user == "" {
		return "", usage("user required")
	}
	resp, err := svc.Users.Get(user).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("resolve user %s: %w", user, err)
	}
	if resp.Id == "" {
		return "", fmt.Errorf("user %s has no ID", user)
	}
	return resp.Id, nil
}

func resolveOrgUnitID(ctx context.Context, svc *admin.Service, path string) (string, error) {
	ou, err := svc.Orgunits.Get(adminCustomerID(), path).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("resolve org unit %s: %w", path, err)
	}
	if ou.OrgUnitId == "" {
		return "", fmt.Errorf("org unit %s has no ID", path)
	}
	return ou.OrgUnitId, nil
}
