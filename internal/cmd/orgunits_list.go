package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type OrgunitsListCmd struct {
	Parent  string       `name:"parent" help:"Parent org unit path (default: /)"`
	Type    string       `name:"type" default:"children" enum:"all,children,allIncludingParent" help:"Whether to return all descendants or immediate children"`
	ToDrive ToDriveFlags `embed:""`
}

func (c *OrgunitsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		parent = "/"
	}

	resp, err := svc.Orgunits.List(adminCustomerID()).
		OrgUnitPath(parent).
		Type(c.Type).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("list org units: %w", err)
	}

	if len(resp.OrganizationUnits) == 0 {
		u.Err().Println("No organizational units found")
		return nil
	}

	rows := make([][]string, 0, len(resp.OrganizationUnits))
	for _, ou := range resp.OrganizationUnits {
		if ou == nil {
			continue
		}
		rows = append(rows, toDriveRow(
			ou.OrgUnitPath,
			ou.Name,
			ou.OrgUnitId,
			ou.Description,
		))
	}

	if ok, err := writeToDrive(ctx, flags, toDriveTitle("Org Units", c.ToDrive), []string{"PATH", "NAME", "ID", "DESCRIPTION"}, rows, c.ToDrive); ok {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PATH\tNAME\tID\tDESCRIPTION")
	for _, row := range rows {
		if len(row) < 4 {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(row[0]),
			sanitizeTab(row[1]),
			sanitizeTab(row[2]),
			sanitizeTab(row[3]),
		)
	}
	return nil
}
