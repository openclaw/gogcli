package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/vault/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type VaultMattersCmd struct {
	List   VaultMattersListCmd   `cmd:"" name:"list" aliases:"ls" help:"List Vault matters"`
	Get    VaultMattersGetCmd    `cmd:"" name:"get" help:"Get Vault matter"`
	Create VaultMattersCreateCmd `cmd:"" name:"create" aliases:"add" help:"Create Vault matter"`
	Update VaultMattersUpdateCmd `cmd:"" name:"update" help:"Update Vault matter"`
	Close  VaultMattersCloseCmd  `cmd:"" name:"close" help:"Close Vault matter"`
	Reopen VaultMattersReopenCmd `cmd:"" name:"reopen" help:"Reopen Vault matter"`
	Delete VaultMattersDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete Vault matter"`
}

type VaultMattersListCmd struct {
	State string `name:"state" help:"Filter by state (OPEN, CLOSED, DELETED)"`
	Max   int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page  string `name:"page" help:"Page token"`
}

func (c *VaultMattersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Matters.List().View("BASIC")
	if state := strings.ToUpper(strings.TrimSpace(c.State)); state != "" {
		switch state {
		case "OPEN", "CLOSED", "DELETED":
			call = call.State(state)
		default:
			return usage("invalid --state (expected OPEN, CLOSED, DELETED)")
		}
	}
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list matters: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Matters) == 0 {
		u.Err().Println("No matters found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "MATTER ID\tNAME\tSTATE")
	for _, matter := range resp.Matters {
		if matter == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(matter.MatterId),
			sanitizeTab(matter.Name),
			sanitizeTab(matter.State),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type VaultMattersGetCmd struct {
	MatterID string `arg:"" name:"matter-id" help:"Matter ID"`
}

func (c *VaultMattersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	matter, err := svc.Matters.Get(c.MatterID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get matter %s: %w", c.MatterID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, matter)
	}

	u.Out().Printf("Matter ID:   %s\n", matter.MatterId)
	u.Out().Printf("Name:        %s\n", matter.Name)
	u.Out().Printf("State:       %s\n", matter.State)
	if matter.Description != "" {
		u.Out().Printf("Description: %s\n", matter.Description)
	}
	return nil
}

type VaultMattersCreateCmd struct {
	Name        string `arg:"" name:"name" help:"Matter name"`
	Description string `name:"description" help:"Description"`
}

func (c *VaultMattersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	matter := &vault.Matter{Name: c.Name, Description: c.Description}
	created, err := svc.Matters.Create(matter).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create matter %s: %w", c.Name, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Created matter: %s (%s)\n", created.Name, created.MatterId)
	return nil
}

type VaultMattersUpdateCmd struct {
	MatterID    string  `arg:"" name:"matter-id" help:"Matter ID"`
	Name        *string `name:"name" help:"Matter name"`
	Description *string `name:"description" help:"Description"`
}

func (c *VaultMattersUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	matter := &vault.Matter{}
	hasUpdates := false
	if c.Name != nil {
		matter.Name = *c.Name
		hasUpdates = true
	}
	if c.Description != nil {
		matter.Description = *c.Description
		if *c.Description == "" {
			matter.ForceSendFields = append(matter.ForceSendFields, "Description")
		}
		hasUpdates = true
	}
	if !hasUpdates {
		return usage("no updates specified")
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	updated, err := svc.Matters.Update(c.MatterID, matter).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update matter %s: %w", c.MatterID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Updated matter: %s (%s)\n", updated.Name, updated.MatterId)
	return nil
}

type VaultMattersCloseCmd struct {
	MatterID string `arg:"" name:"matter-id" help:"Matter ID"`
}

func (c *VaultMattersCloseCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("close matter %s", c.MatterID)); err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Matters.Close(c.MatterID, &vault.CloseMatterRequest{}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("close matter %s: %w", c.MatterID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	matterID := c.MatterID
	if resp != nil && resp.Matter != nil && resp.Matter.MatterId != "" {
		matterID = resp.Matter.MatterId
	}
	u.Out().Printf("Closed matter: %s\n", matterID)
	return nil
}

type VaultMattersReopenCmd struct {
	MatterID string `arg:"" name:"matter-id" help:"Matter ID"`
}

func (c *VaultMattersReopenCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Matters.Reopen(c.MatterID, &vault.ReopenMatterRequest{}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("reopen matter %s: %w", c.MatterID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	matterID := c.MatterID
	if resp != nil && resp.Matter != nil && resp.Matter.MatterId != "" {
		matterID = resp.Matter.MatterId
	}
	u := ui.FromContext(ctx)
	u.Out().Printf("Reopened matter: %s\n", matterID)
	return nil
}

type VaultMattersDeleteCmd struct {
	MatterID string `arg:"" name:"matter-id" help:"Matter ID"`
}

func (c *VaultMattersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete matter %s", c.MatterID)); err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	if _, err := svc.Matters.Delete(c.MatterID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete matter %s: %w", c.MatterID, err)
	}

	u.Out().Printf("Deleted matter: %s\n", c.MatterID)
	return nil
}
