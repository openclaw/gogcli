package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ResourcesBuildingsCmd struct {
	List   ResourcesBuildingsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List buildings"`
	Get    ResourcesBuildingsGetCmd    `cmd:"" name:"get" help:"Get building"`
	Create ResourcesBuildingsCreateCmd `cmd:"" name:"create" aliases:"add" help:"Create building"`
	Update ResourcesBuildingsUpdateCmd `cmd:"" name:"update" help:"Update building"`
	Delete ResourcesBuildingsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete building"`
}

type ResourcesBuildingsListCmd struct {
	Max  int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *ResourcesBuildingsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Resources.Buildings.List(adminCustomerID())
	if c.Max > 0 {
		call = call.MaxResults(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list buildings: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Buildings) == 0 {
		u.Err().Println("No buildings found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tFLOORS\tDESCRIPTION")
	for _, building := range resp.Buildings {
		if building == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(building.BuildingId),
			sanitizeTab(building.BuildingName),
			sanitizeTab(strings.Join(building.FloorNames, ", ")),
			sanitizeTab(building.Description),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ResourcesBuildingsGetCmd struct {
	BuildingID string `arg:"" name:"building-id" help:"Building ID"`
}

func (c *ResourcesBuildingsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	buildingID := strings.TrimSpace(c.BuildingID)
	if buildingID == "" {
		return usage("building ID is required")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	building, err := svc.Resources.Buildings.Get(adminCustomerID(), buildingID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get building %s: %w", buildingID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, building)
	}

	u.Out().Printf("ID:          %s\n", building.BuildingId)
	u.Out().Printf("Name:        %s\n", building.BuildingName)
	if building.Description != "" {
		u.Out().Printf("Description: %s\n", building.Description)
	}
	if len(building.FloorNames) > 0 {
		u.Out().Printf("Floors:      %s\n", strings.Join(building.FloorNames, ", "))
	}
	return nil
}

type ResourcesBuildingsCreateCmd struct {
	Name        string `name:"name" help:"Building name" required:""`
	Description string `name:"description" help:"Building description"`
	Floors      string `name:"floors" help:"Comma-separated floor names"`
}

func (c *ResourcesBuildingsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("--name is required")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	building := &admin.Building{
		BuildingName: name,
		Description:  c.Description,
	}
	if floors := splitCSV(c.Floors); len(floors) > 0 {
		building.FloorNames = floors
	}

	created, err := svc.Resources.Buildings.Insert(adminCustomerID(), building).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create building %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created building: %s (%s)\n", created.BuildingName, created.BuildingId)
	return nil
}

type ResourcesBuildingsUpdateCmd struct {
	BuildingID  string  `arg:"" name:"building-id" help:"Building ID"`
	Name        *string `name:"name" help:"Building name"`
	Description *string `name:"description" help:"Building description"`
}

func (c *ResourcesBuildingsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	buildingID := strings.TrimSpace(c.BuildingID)
	if buildingID == "" {
		return usage("building ID is required")
	}

	if c.Name == nil && c.Description == nil {
		return usage("no updates specified")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	patch := &admin.Building{}
	if c.Name != nil {
		patch.BuildingName = strings.TrimSpace(*c.Name)
	}
	if c.Description != nil {
		patch.Description = strings.TrimSpace(*c.Description)
	}

	updated, err := svc.Resources.Buildings.Patch(adminCustomerID(), buildingID, patch).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update building %s: %w", buildingID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u.Out().Printf("Updated building: %s (%s)\n", updated.BuildingName, updated.BuildingId)
	return nil
}

type ResourcesBuildingsDeleteCmd struct {
	BuildingID string `arg:"" name:"building-id" help:"Building ID"`
}

func (c *ResourcesBuildingsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	buildingID := strings.TrimSpace(c.BuildingID)
	if buildingID == "" {
		return usage("building ID is required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete building %s", buildingID)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Resources.Buildings.Delete(adminCustomerID(), buildingID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete building %s: %w", buildingID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"buildingId": buildingID, "deleted": true})
	}

	u.Out().Printf("Deleted building: %s\n", buildingID)
	return nil
}
