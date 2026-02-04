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

type PrintersCmd struct {
	List   PrintersListCmd   `cmd:"" name:"list" aliases:"ls" help:"List printers"`
	Get    PrintersGetCmd    `cmd:"" name:"get" help:"Get printer"`
	Create PrintersCreateCmd `cmd:"" name:"create" aliases:"add" help:"Create printer"`
	Update PrintersUpdateCmd `cmd:"" name:"update" help:"Update printer"`
	Delete PrintersDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete printer"`
}

type PrintersListCmd struct {
	OrgUnit string `name:"org-unit" aliases:"ou" help:"Filter by org unit path or ID"`
	Max     int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page    string `name:"page" help:"Page token"`
}

func (c *PrintersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	parent := printerParent()
	call := svc.Customers.Chrome.Printers.List(parent)
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	if strings.TrimSpace(c.OrgUnit) != "" {
		orgUnit := strings.TrimSpace(c.OrgUnit)
		orgUnit = strings.TrimPrefix(orgUnit, "orgUnits/")
		orgUnitID, err := resolveOrgUnitID(ctx, svc, orgUnit)
		if err != nil {
			return err
		}
		call = call.OrgUnitId(orgUnitID)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list printers: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Printers) == 0 {
		u.Err().Println("No printers found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tURI\tORG UNIT")
	for _, printer := range resp.Printers {
		if printer == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(printer.Id),
			sanitizeTab(printer.DisplayName),
			sanitizeTab(printer.Uri),
			sanitizeTab(printer.OrgUnitId),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type PrintersGetCmd struct {
	PrinterID string `arg:"" name:"printer-id" help:"Printer ID"`
}

func (c *PrintersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	printerID := strings.TrimSpace(c.PrinterID)
	if printerID == "" {
		return usage("printer ID is required")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	name := printerResourceName(printerID)
	printer, err := svc.Customers.Chrome.Printers.Get(name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get printer %s: %w", printerID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, printer)
	}

	u.Out().Printf("ID:        %s\n", printer.Id)
	u.Out().Printf("Name:      %s\n", printer.DisplayName)
	u.Out().Printf("URI:       %s\n", printer.Uri)
	if printer.OrgUnitId != "" {
		u.Out().Printf("Org Unit:  %s\n", printer.OrgUnitId)
	}
	if printer.Description != "" {
		u.Out().Printf("Desc:      %s\n", printer.Description)
	}
	return nil
}

type PrintersCreateCmd struct {
	Name    string `name:"name" help:"Printer name" required:""`
	URI     string `name:"uri" help:"Printer URI" required:""`
	OrgUnit string `name:"org-unit" aliases:"ou" help:"Org unit path or ID"`
}

func (c *PrintersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	name := strings.TrimSpace(c.Name)
	uri := strings.TrimSpace(c.URI)
	if name == "" || uri == "" {
		return usage("--name and --uri are required")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	printer := &admin.Printer{
		DisplayName: name,
		Uri:         uri,
	}
	if strings.TrimSpace(c.OrgUnit) != "" {
		orgUnit := strings.TrimSpace(c.OrgUnit)
		orgUnit = strings.TrimPrefix(orgUnit, "orgUnits/")
		orgUnitID, err := resolveOrgUnitID(ctx, svc, orgUnit)
		if err != nil {
			return err
		}
		printer.OrgUnitId = orgUnitID
	}

	created, err := svc.Customers.Chrome.Printers.Create(printerParent(), printer).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create printer %s: %w", name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created printer: %s (%s)\n", created.DisplayName, created.Id)
	return nil
}

type PrintersUpdateCmd struct {
	PrinterID string  `arg:"" name:"printer-id" help:"Printer ID"`
	Name      *string `name:"name" help:"Printer name"`
	URI       *string `name:"uri" help:"Printer URI"`
}

func (c *PrintersUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	printerID := strings.TrimSpace(c.PrinterID)
	if printerID == "" {
		return usage("printer ID is required")
	}
	if c.Name == nil && c.URI == nil {
		return usage("no updates specified")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	patch := &admin.Printer{}
	updateMask := make([]string, 0, 2)
	if c.Name != nil {
		patch.DisplayName = strings.TrimSpace(*c.Name)
		updateMask = append(updateMask, "displayName")
	}
	if c.URI != nil {
		patch.Uri = strings.TrimSpace(*c.URI)
		updateMask = append(updateMask, "uri")
	}

	updated, err := svc.Customers.Chrome.Printers.Patch(printerResourceName(printerID), patch).
		UpdateMask(strings.Join(updateMask, ",")).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("update printer %s: %w", printerID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u.Out().Printf("Updated printer: %s (%s)\n", updated.DisplayName, updated.Id)
	return nil
}

type PrintersDeleteCmd struct {
	PrinterID string `arg:"" name:"printer-id" help:"Printer ID"`
}

func (c *PrintersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	printerID := strings.TrimSpace(c.PrinterID)
	if printerID == "" {
		return usage("printer ID is required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete printer %s", printerID)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if _, err := svc.Customers.Chrome.Printers.Delete(printerResourceName(printerID)).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete printer %s: %w", printerID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"printerId": printerID, "deleted": true})
	}

	u.Out().Printf("Deleted printer: %s\n", printerID)
	return nil
}

func printerParent() string {
	return fmt.Sprintf("customers/%s", adminCustomerID())
}

func printerResourceName(id string) string {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, "customers/") {
		return id
	}
	return fmt.Sprintf("customers/%s/chrome/printers/%s", adminCustomerID(), id)
}
