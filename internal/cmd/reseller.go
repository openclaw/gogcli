package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/api/reseller/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newResellerService = googleapi.NewReseller

type ResellerCmd struct {
	Customers     ResellerCustomersCmd     `cmd:"" name:"customers" help:"Reseller customers"`
	Subscriptions ResellerSubscriptionsCmd `cmd:"" name:"subscriptions" help:"Reseller subscriptions"`
}

type ResellerCustomersCmd struct {
	List   ResellerCustomersListCmd   `cmd:"" name:"list" aliases:"ls" help:"List reseller customers"`
	Get    ResellerCustomersGetCmd    `cmd:"" name:"get" help:"Get reseller customer"`
	Create ResellerCustomersCreateCmd `cmd:"" name:"create" help:"Create reseller customer"`
}

type ResellerCustomersListCmd struct {
	Max    int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page   string `name:"page" help:"Page token"`
	Prefix string `name:"prefix" help:"Customer name prefix filter"`
}

func (c *ResellerCustomersListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newResellerService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Subscriptions.List().MaxResults(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	if strings.TrimSpace(c.Prefix) != "" {
		call = call.CustomerNamePrefix(strings.TrimSpace(c.Prefix))
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list customers: %w", err)
	}

	type customer struct {
		ID     string `json:"id"`
		Domain string `json:"domain"`
	}
	seen := make(map[string]customer)
	for _, sub := range resp.Subscriptions {
		if sub == nil || sub.CustomerId == "" {
			continue
		}
		if _, ok := seen[sub.CustomerId]; ok {
			continue
		}
		seen[sub.CustomerId] = customer{ID: sub.CustomerId, Domain: sub.CustomerDomain}
	}
	customers := make([]customer, 0, len(seen))
	for _, cust := range seen {
		customers = append(customers, cust)
	}
	sort.Slice(customers, func(i, j int) bool {
		if customers[i].Domain == customers[j].Domain {
			return customers[i].ID < customers[j].ID
		}
		return customers[i].Domain < customers[j].Domain
	})

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"customers":     customers,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(customers) == 0 {
		u.Err().Println("No customers found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "CUSTOMER ID\tDOMAIN")
	for _, cust := range customers {
		fmt.Fprintf(w, "%s\t%s\n", sanitizeTab(cust.ID), sanitizeTab(cust.Domain))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ResellerCustomersGetCmd struct {
	Customer string `arg:"" name:"customer" help:"Customer ID or domain"`
}

func (c *ResellerCustomersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	customer := strings.TrimSpace(c.Customer)
	if customer == "" {
		return usage("customer is required")
	}

	svc, err := newResellerService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Customers.Get(customer).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get customer %s: %w", customer, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	u.Out().Printf("Customer ID:  %s\n", resp.CustomerId)
	u.Out().Printf("Domain:       %s\n", resp.CustomerDomain)
	if resp.CustomerType != "" {
		u.Out().Printf("Type:         %s\n", resp.CustomerType)
	}
	if resp.PrimaryAdmin != nil && resp.PrimaryAdmin.PrimaryEmail != "" {
		u.Out().Printf("Primary Admin: %s\n", resp.PrimaryAdmin.PrimaryEmail)
	}
	return nil
}

type ResellerCustomersCreateCmd struct {
	Domain         string `name:"domain" help:"Customer domain" required:""`
	AdminEmail     string `name:"admin-email" help:"Primary admin email" required:""`
	AlternateEmail string `name:"alternate-email" help:"Alternate email"`
	Type           string `name:"type" default:"domain" enum:"domain,team" help:"Customer type"`
	Phone          string `name:"phone" help:"Phone number"`
}

func (c *ResellerCustomersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	domain := strings.TrimSpace(c.Domain)
	adminEmail := strings.TrimSpace(c.AdminEmail)
	if domain == "" || adminEmail == "" {
		return usage("--domain and --admin-email are required")
	}

	svc, err := newResellerService(ctx, account)
	if err != nil {
		return err
	}

	customer := &reseller.Customer{
		CustomerDomain: domain,
		CustomerType:   c.Type,
		PrimaryAdmin:   &reseller.PrimaryAdmin{PrimaryEmail: adminEmail},
	}
	if strings.TrimSpace(c.AlternateEmail) != "" {
		customer.AlternateEmail = strings.TrimSpace(c.AlternateEmail)
	}
	if strings.TrimSpace(c.Phone) != "" {
		customer.PhoneNumber = strings.TrimSpace(c.Phone)
	}

	resp, err := svc.Customers.Insert(customer).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create customer: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	u.Out().Printf("Created customer %s (%s)\n", resp.CustomerId, resp.CustomerDomain)
	return nil
}

type ResellerSubscriptionsCmd struct {
	List   ResellerSubscriptionsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List reseller subscriptions"`
	Get    ResellerSubscriptionsGetCmd    `cmd:"" name:"get" help:"Get reseller subscription"`
	Create ResellerSubscriptionsCreateCmd `cmd:"" name:"create" help:"Create reseller subscription"`
}

type ResellerSubscriptionsListCmd struct {
	Customer string `name:"customer" help:"Customer ID"`
	Max      int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page     string `name:"page" help:"Page token"`
	Prefix   string `name:"prefix" help:"Customer name prefix filter"`
}

func (c *ResellerSubscriptionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newResellerService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Subscriptions.List().MaxResults(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	if strings.TrimSpace(c.Customer) != "" {
		call = call.CustomerId(strings.TrimSpace(c.Customer))
	}
	if strings.TrimSpace(c.Prefix) != "" {
		call = call.CustomerNamePrefix(strings.TrimSpace(c.Prefix))
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list subscriptions: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Subscriptions) == 0 {
		u.Err().Println("No subscriptions found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "CUSTOMER\tSUBSCRIPTION\tSKU\tPLAN\tSTATUS")
	for _, sub := range resp.Subscriptions {
		if sub == nil {
			continue
		}
		plan := ""
		if sub.Plan != nil {
			plan = sub.Plan.PlanName
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			sanitizeTab(sub.CustomerId),
			sanitizeTab(sub.SubscriptionId),
			sanitizeTab(sub.SkuId),
			sanitizeTab(plan),
			sanitizeTab(sub.Status),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ResellerSubscriptionsGetCmd struct {
	Customer     string `arg:"" name:"customer" help:"Customer ID"`
	Subscription string `arg:"" name:"subscription" help:"Subscription ID"`
}

func (c *ResellerSubscriptionsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	customer := strings.TrimSpace(c.Customer)
	subscription := strings.TrimSpace(c.Subscription)
	if customer == "" || subscription == "" {
		return usage("customer and subscription are required")
	}

	svc, err := newResellerService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Subscriptions.Get(customer, subscription).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get subscription %s: %w", subscription, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	u.Out().Printf("Customer:      %s\n", resp.CustomerId)
	u.Out().Printf("Subscription:  %s\n", resp.SubscriptionId)
	u.Out().Printf("SKU:           %s\n", resp.SkuId)
	if resp.Plan != nil {
		u.Out().Printf("Plan:          %s\n", resp.Plan.PlanName)
	}
	if resp.Status != "" {
		u.Out().Printf("Status:        %s\n", resp.Status)
	}
	return nil
}

type ResellerSubscriptionsCreateCmd struct {
	Customer string `name:"customer" help:"Customer ID" required:""`
	Plan     string `name:"plan" help:"Plan name (FLEXIBLE, ANNUAL_MONTHLY_PAY, ANNUAL_YEARLY_PAY, TRIAL, FREE)" required:""`
	SKU      string `name:"sku" help:"SKU ID" required:""`
	Seats    int64  `name:"seats" help:"Number of seats for annual plans"`
	MaxSeats int64  `name:"max-seats" help:"Maximum seats for flexible/trial plans"`
}

func (c *ResellerSubscriptionsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	customer := strings.TrimSpace(c.Customer)
	plan := strings.ToUpper(strings.TrimSpace(c.Plan))
	sku := strings.TrimSpace(c.SKU)
	if customer == "" || plan == "" || sku == "" {
		return usage("--customer, --plan, and --sku are required")
	}

	svc, err := newResellerService(ctx, account)
	if err != nil {
		return err
	}

	seats := &reseller.Seats{}
	if plan == "FLEXIBLE" || plan == "TRIAL" || plan == "FREE" {
		if c.MaxSeats > 0 {
			seats.MaximumNumberOfSeats = c.MaxSeats
		} else if c.Seats > 0 {
			seats.MaximumNumberOfSeats = c.Seats
		}
	} else {
		if c.Seats > 0 {
			seats.NumberOfSeats = c.Seats
		} else if c.MaxSeats > 0 {
			seats.NumberOfSeats = c.MaxSeats
		}
	}
	if seats.NumberOfSeats == 0 && seats.MaximumNumberOfSeats == 0 {
		return usage("--seats or --max-seats is required")
	}

	subscription := &reseller.Subscription{
		SkuId: sku,
		Plan:  &reseller.SubscriptionPlan{PlanName: plan},
		Seats: seats,
	}

	resp, err := svc.Subscriptions.Insert(customer, subscription).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	u.Out().Printf("Created subscription %s for %s\n", resp.SubscriptionId, resp.CustomerId)
	return nil
}
