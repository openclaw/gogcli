package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

// AdminOrgunitsCmd manages Workspace organizational units.
type AdminOrgunitsCmd struct {
	List   AdminOrgunitsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List organizational units"`
	Get    AdminOrgunitsGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get organizational unit details"`
	Create AdminOrgunitsCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create an organizational unit"`
	Update AdminOrgunitsUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update an organizational unit"`
	Delete AdminOrgunitsDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete an organizational unit"`
}

type AdminOrgunitsListCmd struct {
	Parent string `name:"parent" help:"Parent org unit path or ID" default:"/"`
	Type   string `name:"type" enum:"all,children,allIncludingParent" help:"Return all descendants, children, or all including parent" default:"children"`
}

func (c *AdminOrgunitsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	svc, err := adminOrgUnitDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminOrgUnitDirectoryError(err, account)
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		parent = "/"
	}
	resp, err := svc.Orgunits.List(adminCustomerID).OrgUnitPath(parent).Type(c.Type).Context(ctx).Do()
	if err != nil {
		return wrapAdminOrgUnitDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), resp)
	}
	if len(resp.OrganizationUnits) == 0 {
		u.Err().Println("No organizational units found")
		return nil
	}

	return outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		nonNilAdminRows(resp.OrganizationUnits),
		adminOrgUnitColumns(),
	)
}

type AdminOrgunitsGetCmd struct {
	Path string `arg:"" name:"path" help:"Org unit path or ID"`
}

func (c *AdminOrgunitsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	path := strings.TrimSpace(c.Path)
	if path == "" {
		return usage("org unit path required")
	}

	path = normalizeAdminOrgUnitPath(path)
	if path == "" {
		return usage("org unit path required")
	}

	svc, err := adminOrgUnitDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminOrgUnitDirectoryError(err, account)
	}

	ou, err := svc.Orgunits.Get(adminCustomerID, path).Context(ctx).Do()
	if err != nil {
		return wrapAdminOrgUnitDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), ou)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintf(w, "Name:\t%s\n", ou.Name)
	fmt.Fprintf(w, "Path:\t%s\n", ou.OrgUnitPath)
	fmt.Fprintf(w, "ID:\t%s\n", ou.OrgUnitId)
	fmt.Fprintf(w, "Parent Path:\t%s\n", ou.ParentOrgUnitPath)
	fmt.Fprintf(w, "Parent ID:\t%s\n", ou.ParentOrgUnitId)
	if ou.Description != "" {
		fmt.Fprintf(w, "Description:\t%s\n", ou.Description)
	}
	return nil
}

type AdminOrgunitsCreateCmd struct {
	Name        string `arg:"" name:"name" help:"Org unit name"`
	Parent      string `name:"parent" help:"Parent org unit path" default:"/"`
	Description string `name:"description" help:"Description"`
}

func (c *AdminOrgunitsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := newAdminOrgUnitCreatePlan(adminOrgUnitCreateInput{
		Name:        c.Name,
		Parent:      c.Parent,
		Description: c.Description,
	})
	if err != nil {
		return err
	}
	if dryRunErr := dryRunExit(ctx, flags, "admin.orgunits.create", plan.Request); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	svc, err := adminOrgUnitDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminOrgUnitDirectoryError(err, account)
	}

	created, err := svc.Orgunits.Insert(adminCustomerID, plan.Request).Context(ctx).Do()
	if err != nil {
		return wrapAdminOrgUnitDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), created)
	}
	u.Out().Linef("Created org unit: %s (%s)", created.Name, created.OrgUnitPath)
	return nil
}

type AdminOrgunitsUpdateCmd struct {
	Path        string  `arg:"" name:"path" help:"Org unit path or ID"`
	Name        *string `name:"name" help:"New org unit name"`
	Parent      *string `name:"parent" help:"New parent org unit path"`
	Description *string `name:"description" help:"Description"`
}

func (c *AdminOrgunitsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	plan, err := newAdminOrgUnitUpdatePlan(adminOrgUnitUpdateInput{
		Path:        c.Path,
		Name:        c.Name,
		Parent:      c.Parent,
		Description: c.Description,
	})
	if err != nil {
		return err
	}
	if dryRunErr := dryRunExit(ctx, flags, "admin.orgunits.update", plan.Request); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	if plan.Path == "" {
		return usage("org unit path required")
	}

	svc, err := adminOrgUnitDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminOrgUnitDirectoryError(err, account)
	}

	updated, err := svc.Orgunits.Patch(adminCustomerID, plan.Path, plan.Request).Context(ctx).Do()
	if err != nil {
		return wrapAdminOrgUnitDirectoryError(err, account)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), updated)
	}
	u.Out().Linef("Updated org unit: %s (%s)", updated.Name, updated.OrgUnitPath)
	return nil
}

type AdminOrgunitsDeleteCmd struct {
	Path string `arg:"" name:"path" help:"Org unit path or ID"`
}

func (c *AdminOrgunitsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := strings.TrimSpace(c.Path)
	if path == "" {
		return usage("org unit path required")
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "admin.orgunits.delete", map[string]any{
		"path": path,
	}, fmt.Sprintf("delete org unit %s", path)); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAdminAccount(flags)
	if err != nil {
		return err
	}

	path = normalizeAdminOrgUnitPath(path)
	if path == "" {
		return usage("org unit path required")
	}

	svc, err := adminOrgUnitDirectoryService(ctx, account)
	if err != nil {
		return wrapAdminOrgUnitDirectoryError(err, account)
	}

	if err := svc.Orgunits.Delete(adminCustomerID, path).Context(ctx).Do(); err != nil {
		return wrapAdminOrgUnitDirectoryError(err, account)
	}

	return writeResult(ctx, ui.FromContext(ctx),
		kv("path", path),
		kv("deleted", true),
	)
}

func normalizeAdminOrgUnitPath(path string) string {
	return strings.TrimLeft(strings.TrimSpace(path), "/")
}
