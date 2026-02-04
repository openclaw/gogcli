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

type ResourcesCalendarsCmd struct {
	List   ResourcesCalendarsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List calendar resources"`
	Get    ResourcesCalendarsGetCmd    `cmd:"" name:"get" help:"Get calendar resource"`
	Create ResourcesCalendarsCreateCmd `cmd:"" name:"create" aliases:"add" help:"Create calendar resource"`
	Update ResourcesCalendarsUpdateCmd `cmd:"" name:"update" help:"Update calendar resource"`
	Delete ResourcesCalendarsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete calendar resource"`
}

type ResourcesCalendarsListCmd struct {
	Building string `name:"building" help:"Filter by building ID"`
	Max      int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page     string `name:"page" help:"Page token"`
}

func (c *ResourcesCalendarsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Resources.Calendars.List(adminCustomerID())
	if c.Max > 0 {
		call = call.MaxResults(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list calendar resources: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	items := resp.Items
	if building := strings.TrimSpace(c.Building); building != "" {
		filtered := make([]*admin.CalendarResource, 0, len(items))
		for _, item := range items {
			if item != nil && item.BuildingId == building {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	if len(items) == 0 {
		u.Err().Println("No calendar resources found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE ID\tNAME\tEMAIL\tCATEGORY\tBUILDING\tFLOOR\tCAPACITY")
	for _, resource := range items {
		if resource == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
			sanitizeTab(resource.ResourceId),
			sanitizeTab(resource.ResourceName),
			sanitizeTab(resource.ResourceEmail),
			sanitizeTab(resource.ResourceCategory),
			sanitizeTab(resource.BuildingId),
			sanitizeTab(resource.FloorName),
			resource.Capacity,
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ResourcesCalendarsGetCmd struct {
	ResourceID string `arg:"" name:"resource-id" help:"Resource ID"`
}

func (c *ResourcesCalendarsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	resourceID := strings.TrimSpace(c.ResourceID)
	if resourceID == "" {
		return usage("resource ID is required")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	resource, err := svc.Resources.Calendars.Get(adminCustomerID(), resourceID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get calendar resource %s: %w", resourceID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resource)
	}

	u.Out().Printf("ID:          %s\n", resource.ResourceId)
	u.Out().Printf("Name:        %s\n", resource.ResourceName)
	u.Out().Printf("Email:       %s\n", resource.ResourceEmail)
	u.Out().Printf("Category:    %s\n", resource.ResourceCategory)
	if resource.ResourceDescription != "" {
		u.Out().Printf("Description: %s\n", resource.ResourceDescription)
	}
	if resource.UserVisibleDescription != "" {
		u.Out().Printf("User Desc:   %s\n", resource.UserVisibleDescription)
	}
	if resource.BuildingId != "" {
		u.Out().Printf("Building:    %s\n", resource.BuildingId)
	}
	if resource.FloorName != "" {
		u.Out().Printf("Floor:       %s\n", resource.FloorName)
	}
	if resource.Capacity != 0 {
		u.Out().Printf("Capacity:    %d\n", resource.Capacity)
	}
	return nil
}

type ResourcesCalendarsCreateCmd struct {
	Name     string `name:"name" help:"Resource name" required:""`
	Type     string `name:"type" help:"Resource category: CONFERENCE_ROOM|OTHER" enum:"CONFERENCE_ROOM,OTHER" required:""`
	Building string `name:"building" help:"Building ID"`
	Floor    string `name:"floor" help:"Floor name"`
	Capacity int64  `name:"capacity" help:"Capacity"`
}

func (c *ResourcesCalendarsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	resource := &admin.CalendarResource{
		ResourceName:     name,
		ResourceCategory: c.Type,
		BuildingId:       strings.TrimSpace(c.Building),
		FloorName:        strings.TrimSpace(c.Floor),
		Capacity:         c.Capacity,
	}

	created, err := svc.Resources.Calendars.Insert(adminCustomerID(), resource).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create calendar resource %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created calendar resource: %s (%s)\n", created.ResourceName, created.ResourceId)
	return nil
}

type ResourcesCalendarsUpdateCmd struct {
	ResourceID string  `arg:"" name:"resource-id" help:"Resource ID"`
	Name       *string `name:"name" help:"Resource name"`
	Capacity   *int64  `name:"capacity" help:"Capacity"`
}

func (c *ResourcesCalendarsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	resourceID := strings.TrimSpace(c.ResourceID)
	if resourceID == "" {
		return usage("resource ID is required")
	}

	if c.Name == nil && c.Capacity == nil {
		return usage("no updates specified")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	patch := &admin.CalendarResource{}
	if c.Name != nil {
		patch.ResourceName = strings.TrimSpace(*c.Name)
	}
	if c.Capacity != nil {
		patch.Capacity = *c.Capacity
	}

	updated, err := svc.Resources.Calendars.Patch(adminCustomerID(), resourceID, patch).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update calendar resource %s: %w", resourceID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u.Out().Printf("Updated calendar resource: %s (%s)\n", updated.ResourceName, updated.ResourceId)
	return nil
}

type ResourcesCalendarsDeleteCmd struct {
	ResourceID string `arg:"" name:"resource-id" help:"Resource ID"`
}

func (c *ResourcesCalendarsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	resourceID := strings.TrimSpace(c.ResourceID)
	if resourceID == "" {
		return usage("resource ID is required")
	}

	if err = confirmDestructive(ctx, flags, fmt.Sprintf("delete calendar resource %s", resourceID)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Resources.Calendars.Delete(adminCustomerID(), resourceID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete calendar resource %s: %w", resourceID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"resourceId": resourceID, "deleted": true})
	}

	u.Out().Printf("Deleted calendar resource: %s\n", resourceID)
	return nil
}
