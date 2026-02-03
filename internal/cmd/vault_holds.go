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

type VaultHoldsCmd struct {
	List   VaultHoldsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List holds"`
	Get    VaultHoldsGetCmd    `cmd:"" name:"get" help:"Get hold"`
	Create VaultHoldsCreateCmd `cmd:"" name:"create" aliases:"add" help:"Create hold"`
	Delete VaultHoldsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete hold"`
}

type VaultHoldsListCmd struct {
	MatterID string `name:"matter" required:"" help:"Matter ID"`
	Max      int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page     string `name:"page" help:"Page token"`
}

func (c *VaultHoldsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Matters.Holds.List(c.MatterID).PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list holds: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Holds) == 0 {
		u.Err().Println("No holds found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "HOLD ID\tNAME\tCORPUS\tSCOPE")
	for _, hold := range resp.Holds {
		if hold == nil {
			continue
		}
		scope := holdScope(hold)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(hold.HoldId),
			sanitizeTab(hold.Name),
			sanitizeTab(hold.Corpus),
			sanitizeTab(scope),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type VaultHoldsGetCmd struct {
	MatterID string `name:"matter" required:"" help:"Matter ID"`
	HoldID   string `arg:"" name:"hold-id" help:"Hold ID"`
}

func (c *VaultHoldsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	hold, err := svc.Matters.Holds.Get(c.MatterID, c.HoldID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get hold %s: %w", c.HoldID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, hold)
	}

	u.Out().Printf("Hold ID:  %s\n", hold.HoldId)
	u.Out().Printf("Name:     %s\n", hold.Name)
	u.Out().Printf("Corpus:   %s\n", hold.Corpus)
	u.Out().Printf("Scope:    %s\n", holdScope(hold))
	return nil
}

type VaultHoldsCreateCmd struct {
	MatterID string `name:"matter" required:"" help:"Matter ID"`
	Name     string `name:"name" required:"" help:"Hold name"`
	Corpus   string `name:"corpus" required:"" enum:"MAIL,DRIVE,GROUPS" help:"Corpus to hold"`
	Accounts string `name:"accounts" help:"Comma-separated account emails"`
	OrgUnit  string `name:"org-unit" aliases:"ou" help:"Org unit path"`
	Query    string `name:"query" help:"Search query"`
}

func (c *VaultHoldsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	hold := &vault.Hold{
		Name:   c.Name,
		Corpus: strings.ToUpper(c.Corpus),
	}

	accounts := splitCSV(c.Accounts)
	orgUnit := strings.TrimSpace(c.OrgUnit)
	if len(accounts) == 0 && orgUnit == "" {
		return usage("--accounts or --org-unit required")
	}
	if len(accounts) > 0 && orgUnit != "" {
		return usage("use only one of --accounts or --org-unit")
	}

	if len(accounts) > 0 {
		hold.Accounts = make([]*vault.HeldAccount, 0, len(accounts))
		for _, email := range accounts {
			email = strings.TrimSpace(email)
			if email == "" {
				continue
			}
			hold.Accounts = append(hold.Accounts, &vault.HeldAccount{Email: email})
		}
	}

	if orgUnit != "" {
		adminSvc, err := newAdminDirectory(ctx, account)
		if err != nil {
			return err
		}
		orgID, err := resolveOrgUnitID(ctx, adminSvc, orgUnit)
		if err != nil {
			return err
		}
		hold.OrgUnit = &vault.HeldOrgUnit{OrgUnitId: orgID}
	}

	if strings.TrimSpace(c.Query) != "" {
		if hold.Corpus == "DRIVE" {
			return usage("drive holds do not support --query")
		}
		if hold.Corpus == "MAIL" {
			hold.Query = &vault.CorpusQuery{MailQuery: &vault.HeldMailQuery{Terms: c.Query}}
		} else if hold.Corpus == "GROUPS" {
			hold.Query = &vault.CorpusQuery{GroupsQuery: &vault.HeldGroupsQuery{Terms: c.Query}}
		}
	}

	created, err := svc.Matters.Holds.Create(c.MatterID, hold).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create hold: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("Created hold: %s (%s)\n", created.Name, created.HoldId)
	return nil
}

type VaultHoldsDeleteCmd struct {
	MatterID string `name:"matter" required:"" help:"Matter ID"`
	HoldID   string `arg:"" name:"hold-id" help:"Hold ID"`
}

func (c *VaultHoldsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete hold %s", c.HoldID)); err != nil {
		return err
	}

	svc, err := newVaultService(ctx, account)
	if err != nil {
		return err
	}

	if _, err := svc.Matters.Holds.Delete(c.MatterID, c.HoldID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete hold %s: %w", c.HoldID, err)
	}

	u.Out().Printf("Deleted hold: %s\n", c.HoldID)
	return nil
}

func holdScope(hold *vault.Hold) string {
	if hold == nil {
		return ""
	}
	if hold.OrgUnit != nil && hold.OrgUnit.OrgUnitId != "" {
		return "org-unit"
	}
	if len(hold.Accounts) > 0 {
		return fmt.Sprintf("accounts:%d", len(hold.Accounts))
	}
	return ""
}
