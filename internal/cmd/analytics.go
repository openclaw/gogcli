package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newAnalyticsAdminService = googleapi.NewAnalyticsAdmin

type AnalyticsCmd struct {
	Accounts    AnalyticsAccountsCmd    `cmd:"" name:"accounts" help:"List Analytics accounts"`
	Properties  AnalyticsPropertiesCmd  `cmd:"" name:"properties" help:"List Analytics properties for an account"`
	DataStreams AnalyticsDataStreamsCmd `cmd:"" name:"datastreams" help:"List Analytics data streams for a property"`
}

type AnalyticsAccountsCmd struct {
	Max  int64  `name:"max" aliases:"limit" default:"50" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *AnalyticsAccountsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Accounts.List()
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list analytics accounts: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Accounts) == 0 {
		u.Err().Println("No analytics accounts found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tDISPLAY NAME")
	for _, acc := range resp.Accounts {
		if acc == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\n", sanitizeTab(acc.Name), sanitizeTab(acc.DisplayName))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type AnalyticsPropertiesCmd struct {
	AccountID string `name:"account-id" help:"Account ID" required:""`
	Max       int64  `name:"max" aliases:"limit" default:"50" help:"Max results"`
	Page      string `name:"page" help:"Page token"`
}

func (c *AnalyticsPropertiesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	accountID := strings.TrimSpace(c.AccountID)
	if accountID == "" {
		return usage("--account-id is required")
	}
	if !strings.HasPrefix(accountID, "accounts/") {
		accountID = "accounts/" + accountID
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Properties.List().Filter(fmt.Sprintf("parent:%s", accountID))
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list analytics properties: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Properties) == 0 {
		u.Err().Println("No properties found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tDISPLAY NAME\tTIME ZONE")
	for _, prop := range resp.Properties {
		if prop == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(prop.Name),
			sanitizeTab(prop.DisplayName),
			sanitizeTab(prop.TimeZone),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type AnalyticsDataStreamsCmd struct {
	Property string `name:"property" help:"Property ID" required:""`
	Max      int64  `name:"max" aliases:"limit" default:"50" help:"Max results"`
	Page     string `name:"page" help:"Page token"`
}

func (c *AnalyticsDataStreamsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	propertyID := strings.TrimSpace(c.Property)
	if propertyID == "" {
		return usage("--property is required")
	}
	if !strings.HasPrefix(propertyID, "properties/") {
		propertyID = "properties/" + propertyID
	}

	svc, err := newAnalyticsAdminService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Properties.DataStreams.List(propertyID)
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list analytics datastreams: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.DataStreams) == 0 {
		u.Err().Println("No data streams found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tDISPLAY NAME\tTYPE")
	for _, ds := range resp.DataStreams {
		if ds == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(ds.Name),
			sanitizeTab(ds.DisplayName),
			sanitizeTab(ds.Type),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}
