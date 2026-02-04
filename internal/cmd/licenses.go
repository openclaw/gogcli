package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/licensing/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newLicensingService = googleapi.NewLicensing

type LicensesCmd struct {
	List     LicensesListCmd     `cmd:"" name:"list" aliases:"ls" help:"List license assignments"`
	Get      LicensesGetCmd      `cmd:"" name:"get" help:"Get a license assignment"`
	Assign   LicensesAssignCmd   `cmd:"" name:"assign" help:"Assign a license"`
	Revoke   LicensesRevokeCmd   `cmd:"" name:"revoke" help:"Revoke a license"`
	Products LicensesProductsCmd `cmd:"" name:"products" help:"List available products and SKUs"`
}

type LicensesListCmd struct {
	Product string `name:"product" help:"Product ID"`
	SKU     string `name:"sku" help:"SKU ID"`
	Max     int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page    string `name:"page" help:"Page token"`
}

func (c *LicensesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	product := strings.TrimSpace(c.Product)
	if product == "" {
		return usage("--product is required")
	}

	svc, err := newLicensingService(ctx, account)
	if err != nil {
		return err
	}

	customer := adminCustomerID()
	var resp *licensing.LicenseAssignmentList
	if strings.TrimSpace(c.SKU) != "" {
		call := svc.LicenseAssignments.ListForProductAndSku(product, c.SKU, customer)
		if c.Max > 0 {
			call = call.MaxResults(c.Max)
		}
		if c.Page != "" {
			call = call.PageToken(c.Page)
		}
		resp, err = call.Context(ctx).Do()
	} else {
		call := svc.LicenseAssignments.ListForProduct(product, customer)
		if c.Max > 0 {
			call = call.MaxResults(c.Max)
		}
		if c.Page != "" {
			call = call.PageToken(c.Page)
		}
		resp, err = call.Context(ctx).Do()
	}
	if err != nil {
		return fmt.Errorf("list licenses: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No licenses found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "USER\tPRODUCT\tSKU")
	for _, item := range resp.Items {
		if item == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(item.UserId),
			sanitizeTab(item.ProductId),
			sanitizeTab(item.SkuId),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type LicensesGetCmd struct {
	User    string `arg:"" name:"user" help:"User email or ID"`
	Product string `name:"product" help:"Product ID" required:""`
	SKU     string `name:"sku" help:"SKU ID" required:""`
}

func (c *LicensesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	user := strings.TrimSpace(c.User)
	if user == "" {
		return usage("user is required")
	}

	svc, err := newLicensingService(ctx, account)
	if err != nil {
		return err
	}

	assignment, err := svc.LicenseAssignments.Get(c.Product, c.SKU, user).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get license for %s: %w", user, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, assignment)
	}

	u.Out().Printf("User:        %s\n", assignment.UserId)
	u.Out().Printf("Product:     %s\n", assignment.ProductId)
	if assignment.ProductName != "" {
		u.Out().Printf("Product Name: %s\n", assignment.ProductName)
	}
	u.Out().Printf("SKU:         %s\n", assignment.SkuId)
	if assignment.SkuName != "" {
		u.Out().Printf("SKU Name:    %s\n", assignment.SkuName)
	}
	return nil
}

type LicensesAssignCmd struct {
	User    string `arg:"" name:"user" help:"User email or ID"`
	Product string `name:"product" help:"Product ID" required:""`
	SKU     string `name:"sku" help:"SKU ID" required:""`
}

func (c *LicensesAssignCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	user := strings.TrimSpace(c.User)
	if user == "" {
		return usage("user is required")
	}

	svc, err := newLicensingService(ctx, account)
	if err != nil {
		return err
	}

	insert := &licensing.LicenseAssignmentInsert{UserId: user}
	assignment, err := svc.LicenseAssignments.Insert(c.Product, c.SKU, insert).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("assign license to %s: %w", user, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, assignment)
	}

	u.Out().Printf("Assigned license %s/%s to %s\n", assignment.ProductId, assignment.SkuId, assignment.UserId)
	return nil
}

type LicensesRevokeCmd struct {
	User    string `arg:"" name:"user" help:"User email or ID"`
	Product string `name:"product" help:"Product ID" required:""`
	SKU     string `name:"sku" help:"SKU ID" required:""`
}

func (c *LicensesRevokeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	user := strings.TrimSpace(c.User)
	if user == "" {
		return usage("user is required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("revoke license %s/%s for %s", c.Product, c.SKU, user)); err != nil {
		return err
	}

	svc, err := newLicensingService(ctx, account)
	if err != nil {
		return err
	}

	if _, err := svc.LicenseAssignments.Delete(c.Product, c.SKU, user).Context(ctx).Do(); err != nil {
		return fmt.Errorf("revoke license for %s: %w", user, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"user": user, "product": c.Product, "sku": c.SKU, "revoked": true})
	}

	u.Out().Printf("Revoked license %s/%s for %s\n", c.Product, c.SKU, user)
	return nil
}

type LicensesProductsCmd struct{}

func (c *LicensesProductsCmd) Run(ctx context.Context, _ *RootFlags) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, licenseProducts)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PRODUCT ID\tPRODUCT NAME\tSKU ID\tSKU NAME")
	for _, product := range licenseProducts {
		for _, sku := range product.SKUs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				sanitizeTab(product.ID),
				sanitizeTab(product.Name),
				sanitizeTab(sku.ID),
				sanitizeTab(sku.Name),
			)
		}
	}
	return nil
}

type licenseSKU struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	UnarchivalProduct  string `json:"unarchivalProduct,omitempty"`
	UnarchivalSKU      string `json:"unarchivalSku,omitempty"`
	UnarchivalSKUName  string `json:"unarchivalSkuName,omitempty"`
	UnarchivalProdName string `json:"unarchivalProductName,omitempty"`
}

type licenseProduct struct {
	ID   string       `json:"id"`
	Name string       `json:"name"`
	SKUs []licenseSKU `json:"skus"`
}

var licenseProducts = []licenseProduct{
	{
		ID:   "Google-Apps",
		Name: "Google Workspace",
		SKUs: []licenseSKU{
			{ID: "1010020027", Name: "Google Workspace Business Starter"},
			{ID: "1010020028", Name: "Google Workspace Business Standard"},
			{ID: "1010020025", Name: "Google Workspace Business Plus"},
			{ID: "1010060003", Name: "Google Workspace Enterprise Essentials"},
			{ID: "1010020029", Name: "Google Workspace Enterprise Starter"},
			{ID: "1010020026", Name: "Google Workspace Enterprise Standard"},
			{ID: "1010020020", Name: "Google Workspace Enterprise Plus (formerly G Suite Enterprise)"},
			{ID: "1010060001", Name: "Google Workspace Essentials (formerly G Suite Essentials)"},
			{ID: "1010060005", Name: "Google Workspace Enterprise Essentials Plus"},
			{ID: "1010020030", Name: "Google Workspace Frontline Starter"},
			{ID: "1010020031", Name: "Google Workspace Frontline Standard"},
			{ID: "1010020034", Name: "Google Workspace Frontline Plus"},
			{ID: "1010020035", Name: "Google Workspace Business Continuity"},
			{ID: "1010020036", Name: "Google Workspace Business Continuity Plus"},
			{ID: "Google-Apps-Unlimited", Name: "G Suite Business"},
			{ID: "Google-Apps-For-Business", Name: "G Suite Basic"},
			{ID: "Google-Apps-Lite", Name: "G Suite Lite"},
			{ID: "Google-Apps-For-Postini", Name: "Google Apps Message Security"},
			{ID: "Google-Apps-For-Education", Name: "Google Workspace for Education - Fundamentals"},
			{ID: "1010070001", Name: "Google Workspace for Education Fundamentals"},
			{ID: "1010070004", Name: "Google Workspace for Education Gmail Only"},
		},
	},
	{
		ID:   "101047",
		Name: "Google AI",
		SKUs: []licenseSKU{
			{ID: "1010470008", Name: "Google AI Ultra for Business"},
			{ID: "1010470004", Name: "Google AI Pro for Education"},
			{ID: "1010470005", Name: "Gemini Education Premium"},
		},
	},
	{
		ID:   "101031",
		Name: "Google Workspace for Education",
		SKUs: []licenseSKU{
			{ID: "1010310005", Name: "Google Workspace for Education Standard"},
			{ID: "1010310006", Name: "Google Workspace for Education Standard (Staff)"},
			{ID: "1010310007", Name: "Google Workspace for Education Standard (Extra Student)"},
			{ID: "1010310008", Name: "Google Workspace for Education Plus"},
			{ID: "1010310009", Name: "Google Workspace for Education Plus (Staff)"},
			{ID: "1010310010", Name: "Google Workspace for Education Plus (Extra Student)"},
			{ID: "1010310002", Name: "Google Workspace for Education Plus - Legacy"},
			{ID: "1010310003", Name: "Google Workspace for Education Plus - Legacy (Student)"},
		},
	},
	{
		ID:   "101037",
		Name: "Google Workspace for Education: Teaching and Learning Upgrade",
		SKUs: []licenseSKU{
			{ID: "1010370001", Name: "Google Workspace for Education: Teaching and Learning Upgrade"},
		},
	},
	{
		ID:   "101038",
		Name: "AppSheet",
		SKUs: []licenseSKU{
			{ID: "1010380001", Name: "AppSheet Core"},
			{ID: "1010380002", Name: "AppSheet Enterprise Standard"},
			{ID: "1010380003", Name: "AppSheet Enterprise Plus"},
		},
	},
	{
		ID:   "Google-Vault",
		Name: "Google Vault",
		SKUs: []licenseSKU{
			{ID: "Google-Vault", Name: "Google Vault"},
			{ID: "Google-Vault-Former-Employee", Name: "Google Vault - Former Employee"},
		},
	},
	{
		ID:   "101001",
		Name: "Cloud Identity",
		SKUs: []licenseSKU{
			{ID: "1010010001", Name: "Cloud Identity"},
		},
	},
	{
		ID:   "101005",
		Name: "Cloud Identity Premium",
		SKUs: []licenseSKU{
			{ID: "1010050001", Name: "Cloud Identity Premium"},
		},
	},
	{
		ID:   "101033",
		Name: "Google Voice",
		SKUs: []licenseSKU{
			{ID: "1010330003", Name: "Google Voice Starter"},
			{ID: "1010330004", Name: "Google Voice Standard"},
			{ID: "1010330002", Name: "Google Voice Premier"},
		},
	},
	{
		ID:   "101034",
		Name: "Google Workspace Archived User",
		SKUs: []licenseSKU{
			{ID: "1010340007", Name: "Google Workspace for Education Fundamentals - Archived User"},
			{ID: "1010340004", Name: "Google Workspace Enterprise Standard - Archived User", UnarchivalProduct: "Google-Apps", UnarchivalSKU: "1010020026"},
			{ID: "1010340001", Name: "Google Workspace Enterprise Plus - Archived User", UnarchivalProduct: "Google-Apps", UnarchivalSKU: "1010020020"},
			{ID: "1010340005", Name: "Google Workspace Business Starter - Archived User", UnarchivalProduct: "Google-Apps", UnarchivalSKU: "1010020027"},
			{ID: "1010340006", Name: "Google Workspace Business Standard - Archived User", UnarchivalProduct: "Google-Apps", UnarchivalSKU: "1010020028"},
			{ID: "1010340003", Name: "Google Workspace Business Plus - Archived User", UnarchivalProduct: "Google-Apps", UnarchivalSKU: "1010020025"},
			{ID: "1010340002", Name: "G Suite Business - Archived User", UnarchivalProduct: "Google-Apps", UnarchivalSKU: "Google-Apps-Unlimited"},
		},
	},
}
