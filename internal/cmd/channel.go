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

var newCloudChannelService = googleapi.NewCloudChannel

type ChannelCmd struct {
	Customers    ChannelCustomersCmd    `cmd:"" name:"customers" help:"Cloud Channel customers"`
	Offers       ChannelOffersCmd       `cmd:"" name:"offers" help:"Cloud Channel offers"`
	Entitlements ChannelEntitlementsCmd `cmd:"" name:"entitlements" help:"Cloud Channel entitlements"`
}

type ChannelCustomersCmd struct {
	List ChannelCustomersListCmd `cmd:"" name:"list" aliases:"ls" help:"List Cloud Channel customers"`
}

type ChannelCustomersListCmd struct {
	ChannelAccount string `name:"channel-account" help:"Channel account ID or resource (accounts/...)" required:""`
	Max            int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page           string `name:"page" help:"Page token"`
}

func (c *ChannelCustomersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	channelAccount := normalizeChannelAccount(c.ChannelAccount)
	if channelAccount == "" {
		return usage("--channel-account is required")
	}

	svc, err := newCloudChannelService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Accounts.Customers.List(channelAccount).PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list channel customers: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Customers) == 0 {
		u.Err().Println("No customers found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESOURCE\tDOMAIN\tCLOUD IDENTITY")
	for _, cust := range resp.Customers {
		if cust == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(cust.Name),
			sanitizeTab(cust.Domain),
			sanitizeTab(cust.CloudIdentityId),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ChannelOffersCmd struct {
	List ChannelOffersListCmd `cmd:"" name:"list" aliases:"ls" help:"List Cloud Channel offers"`
}

type ChannelOffersListCmd struct {
	ChannelAccount string `name:"channel-account" help:"Channel account ID or resource (accounts/...)" required:""`
	Filter         string `name:"filter" help:"Filter expression"`
	Language       string `name:"language" help:"Language code (default en-US)"`
	Future         bool   `name:"future" help:"Show future offers"`
	Max            int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page           string `name:"page" help:"Page token"`
}

func (c *ChannelOffersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	channelAccount := normalizeChannelAccount(c.ChannelAccount)
	if channelAccount == "" {
		return usage("--channel-account is required")
	}

	svc, err := newCloudChannelService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Accounts.Offers.List(channelAccount).PageSize(c.Max)
	if strings.TrimSpace(c.Filter) != "" {
		call = call.Filter(strings.TrimSpace(c.Filter))
	}
	if strings.TrimSpace(c.Language) != "" {
		call = call.LanguageCode(strings.TrimSpace(c.Language))
	}
	if c.Future {
		call = call.ShowFutureOffers(true)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list offers: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Offers) == 0 {
		u.Err().Println("No offers found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "OFFER\tSKU\tPRODUCT")
	for _, offer := range resp.Offers {
		if offer == nil {
			continue
		}
		sku := ""
		product := ""
		if offer.Sku != nil {
			sku = offer.Sku.Name
			if offer.Sku.Product != nil {
				product = offer.Sku.Product.Name
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(offer.Name),
			sanitizeTab(sku),
			sanitizeTab(product),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ChannelEntitlementsCmd struct {
	List ChannelEntitlementsListCmd `cmd:"" name:"list" aliases:"ls" help:"List Cloud Channel entitlements"`
}

type ChannelEntitlementsListCmd struct {
	ChannelAccount string `name:"channel-account" help:"Channel account ID or resource (accounts/...)" required:""`
	Customer       string `name:"customer" help:"Customer ID or resource" required:""`
	Max            int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page           string `name:"page" help:"Page token"`
}

func (c *ChannelEntitlementsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	channelAccount := normalizeChannelAccount(c.ChannelAccount)
	if channelAccount == "" {
		return usage("--channel-account is required")
	}
	customer := strings.TrimSpace(c.Customer)
	if customer == "" {
		return usage("--customer is required")
	}

	parent := customer
	if !strings.HasPrefix(customer, "accounts/") {
		parent = fmt.Sprintf("%s/customers/%s", channelAccount, customer)
	}

	svc, err := newCloudChannelService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Accounts.Customers.Entitlements.List(parent).PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list entitlements: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Entitlements) == 0 {
		u.Err().Println("No entitlements found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ENTITLEMENT\tOFFER\tSTATE")
	for _, ent := range resp.Entitlements {
		if ent == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(ent.Name),
			sanitizeTab(ent.Offer),
			sanitizeTab(ent.ProvisioningState),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

func normalizeChannelAccount(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "accounts/") {
		return trimmed
	}
	return "accounts/" + trimmed
}
