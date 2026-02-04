package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type RolesCmd struct {
	List       RolesListCmd       `cmd:"" name:"list" aliases:"ls" help:"List admin roles"`
	Get        RolesGetCmd        `cmd:"" name:"get" help:"Get role details"`
	Create     RolesCreateCmd     `cmd:"" name:"create" aliases:"add" help:"Create admin role"`
	Update     RolesUpdateCmd     `cmd:"" name:"update" help:"Update admin role"`
	Delete     RolesDeleteCmd     `cmd:"" name:"delete" aliases:"rm" help:"Delete admin role"`
	Privileges RolesPrivilegesCmd `cmd:"" name:"privileges" help:"List available privileges"`
}

type RolesListCmd struct {
	Max  int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *RolesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Roles.List(adminCustomerID()).MaxResults(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list roles: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No roles found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ROLE ID\tNAME\tSYSTEM\tSUPERADMIN\tPRIVILEGES")
	for _, role := range resp.Items {
		if role == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%t\t%t\t%d\n",
			sanitizeTab(strconv.FormatInt(role.RoleId, 10)),
			sanitizeTab(role.RoleName),
			role.IsSystemRole,
			role.IsSuperAdminRole,
			len(role.RolePrivileges),
		)
	}

	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type RolesGetCmd struct {
	Role string `arg:"" name:"role" help:"Role ID or name"`
}

func (c *RolesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	roleID, role, err := resolveRole(ctx, svc, c.Role)
	if err != nil {
		return err
	}
	if role == nil {
		role, err = svc.Roles.Get(adminCustomerID(), roleID).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("get role %s: %w", c.Role, err)
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, role)
	}

	u.Out().Printf("Role ID:      %s\n", strconv.FormatInt(role.RoleId, 10))
	u.Out().Printf("Name:         %s\n", role.RoleName)
	u.Out().Printf("System Role:  %v\n", role.IsSystemRole)
	u.Out().Printf("Super Admin:  %v\n", role.IsSuperAdminRole)
	if role.RoleDescription != "" {
		u.Out().Printf("Description:  %s\n", role.RoleDescription)
	}
	if len(role.RolePrivileges) > 0 {
		privs := make([]string, 0, len(role.RolePrivileges))
		for _, p := range role.RolePrivileges {
			if p == nil {
				continue
			}
			privs = append(privs, p.PrivilegeName)
		}
		sort.Strings(privs)
		u.Out().Printf("Privileges:   %s\n", strings.Join(privs, ", "))
	}
	return nil
}

type RolesCreateCmd struct {
	Name        string `arg:"" name:"name" help:"Role name"`
	Privileges  string `name:"privileges" required:"" help:"Comma-separated privilege names"`
	Description string `name:"description" help:"Role description"`
}

func (c *RolesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	privNames := splitCSV(c.Privileges)
	if len(privNames) == 0 {
		return usage("--privileges is required")
	}

	privs, err := buildRolePrivileges(ctx, svc, privNames)
	if err != nil {
		return err
	}

	role := &admin.Role{
		RoleName:        c.Name,
		RoleDescription: c.Description,
		RolePrivileges:  privs,
	}

	created, err := svc.Roles.Insert(adminCustomerID(), role).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create role %s: %w", c.Name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Created role: %s (%d)\n", created.RoleName, created.RoleId)
	return nil
}

type RolesUpdateCmd struct {
	Role             string  `arg:"" name:"role" help:"Role ID or name"`
	Name             *string `name:"name" help:"New role name"`
	Description      *string `name:"description" help:"Role description"`
	AddPrivileges    string  `name:"add-privileges" help:"Comma-separated privileges to add"`
	RemovePrivileges string  `name:"remove-privileges" help:"Comma-separated privileges to remove"`
}

func (c *RolesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	roleID, role, err := resolveRole(ctx, svc, c.Role)
	if err != nil {
		return err
	}
	if role == nil {
		role, err = svc.Roles.Get(adminCustomerID(), roleID).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("get role %s: %w", c.Role, err)
		}
	}

	hasUpdates := false
	if c.Name != nil {
		role.RoleName = *c.Name
		hasUpdates = true
	}
	if c.Description != nil {
		role.RoleDescription = *c.Description
		if *c.Description == "" {
			role.ForceSendFields = append(role.ForceSendFields, "RoleDescription")
		}
		hasUpdates = true
	}

	addNames := splitCSV(c.AddPrivileges)
	removeNames := splitCSV(c.RemovePrivileges)
	if len(addNames) > 0 || len(removeNames) > 0 {
		updatedPrivs, err := updateRolePrivileges(ctx, svc, role.RolePrivileges, addNames, removeNames)
		if err != nil {
			return err
		}
		role.RolePrivileges = updatedPrivs
		hasUpdates = true
	}

	if !hasUpdates {
		return usage("no updates specified")
	}

	updated, err := svc.Roles.Update(adminCustomerID(), roleID, role).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update role %s: %w", c.Role, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Updated role: %s (%d)\n", updated.RoleName, updated.RoleId)
	return nil
}

type RolesDeleteCmd struct {
	Role string `arg:"" name:"role" help:"Role ID or name"`
}

func (c *RolesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete role %s", c.Role)); err != nil {
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

	if err := svc.Roles.Delete(adminCustomerID(), roleID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete role %s: %w", c.Role, err)
	}

	u.Out().Printf("Deleted role: %s\n", c.Role)
	return nil
}

type RolesPrivilegesCmd struct{}

func (c *RolesPrivilegesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Privileges.List(adminCustomerID()).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list privileges: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No privileges found")
		return nil
	}

	flat := flattenPrivileges(resp.Items)
	sort.Slice(flat, func(i, j int) bool { return flat[i].PrivilegeName < flat[j].PrivilegeName })

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PRIVILEGE\tSERVICE ID\tOU SCOPABLE")
	for _, priv := range flat {
		fmt.Fprintf(w, "%s\t%s\t%t\n",
			sanitizeTab(priv.PrivilegeName),
			sanitizeTab(priv.ServiceId),
			priv.IsOuScopable,
		)
	}
	return nil
}

func resolveRole(ctx context.Context, svc *admin.Service, role string) (string, *admin.Role, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return "", nil, usage("role required")
	}
	if _, err := strconv.ParseInt(role, 10, 64); err == nil {
		return role, nil, nil
	}

	roles, err := listAllRoles(ctx, svc)
	if err != nil {
		return "", nil, err
	}
	for _, r := range roles {
		if r == nil {
			continue
		}
		if strings.EqualFold(r.RoleName, role) {
			return strconv.FormatInt(r.RoleId, 10), r, nil
		}
	}
	return "", nil, fmt.Errorf("role %q not found", role)
}

func listAllRoles(ctx context.Context, svc *admin.Service) ([]*admin.Role, error) {
	roles := make([]*admin.Role, 0)
	call := svc.Roles.List(adminCustomerID()).MaxResults(200)
	for {
		resp, err := call.Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("list roles: %w", err)
		}
		roles = append(roles, resp.Items...)
		if resp.NextPageToken == "" {
			break
		}
		call = call.PageToken(resp.NextPageToken)
	}
	return roles, nil
}

func buildRolePrivileges(ctx context.Context, svc *admin.Service, names []string) ([]*admin.RoleRolePrivileges, error) {
	privs, err := privilegeMap(ctx, svc)
	if err != nil {
		return nil, err
	}

	out := make([]*admin.RoleRolePrivileges, 0, len(names))
	for _, name := range names {
		key := strings.TrimSpace(name)
		if key == "" {
			continue
		}
		priv, ok := privs[strings.ToLower(key)]
		if !ok {
			return nil, fmt.Errorf("unknown privilege %q", key)
		}
		out = append(out, &admin.RoleRolePrivileges{PrivilegeName: priv.PrivilegeName, ServiceId: priv.ServiceId})
	}
	if len(out) == 0 {
		return nil, usage("no privileges specified")
	}
	return out, nil
}

func updateRolePrivileges(ctx context.Context, svc *admin.Service, existing []*admin.RoleRolePrivileges, add, remove []string) ([]*admin.RoleRolePrivileges, error) {
	set := make(map[string]*admin.RoleRolePrivileges)
	for _, p := range existing {
		if p == nil {
			continue
		}
		set[strings.ToLower(p.PrivilegeName)] = p
	}

	if len(remove) > 0 {
		for _, name := range remove {
			key := strings.ToLower(strings.TrimSpace(name))
			if key == "" {
				continue
			}
			delete(set, key)
		}
	}

	if len(add) > 0 {
		privs, err := privilegeMap(ctx, svc)
		if err != nil {
			return nil, err
		}
		for _, name := range add {
			key := strings.ToLower(strings.TrimSpace(name))
			if key == "" {
				continue
			}
			priv, ok := privs[key]
			if !ok {
				return nil, fmt.Errorf("unknown privilege %q", name)
			}
			set[key] = &admin.RoleRolePrivileges{PrivilegeName: priv.PrivilegeName, ServiceId: priv.ServiceId}
		}
	}

	out := make([]*admin.RoleRolePrivileges, 0, len(set))
	for _, p := range set {
		out = append(out, p)
	}
	return out, nil
}

func privilegeMap(ctx context.Context, svc *admin.Service) (map[string]*admin.Privilege, error) {
	resp, err := svc.Privileges.List(adminCustomerID()).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list privileges: %w", err)
	}
	flat := flattenPrivileges(resp.Items)
	out := make(map[string]*admin.Privilege, len(flat))
	for _, p := range flat {
		if p == nil {
			continue
		}
		out[strings.ToLower(p.PrivilegeName)] = p
	}
	return out, nil
}

func flattenPrivileges(items []*admin.Privilege) []*admin.Privilege {
	var out []*admin.Privilege
	var walk func(p *admin.Privilege)
	walk = func(p *admin.Privilege) {
		if p == nil {
			return
		}
		out = append(out, p)
		for _, child := range p.ChildPrivileges {
			walk(child)
		}
	}
	for _, p := range items {
		walk(p)
	}
	return out
}
