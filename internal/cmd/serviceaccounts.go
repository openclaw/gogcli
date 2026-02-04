package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/iam/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newIAMService = googleapi.NewIAM

type ServiceAccountsCmd struct {
	List   ServiceAccountsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List service accounts"`
	Create ServiceAccountsCreateCmd `cmd:"" name:"create" help:"Create a service account"`
	Delete ServiceAccountsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete a service account"`
	Keys   ServiceAccountsKeysCmd   `cmd:"" name:"keys" help:"Manage service account keys"`
}

type ServiceAccountsListCmd struct {
	Project string `name:"project" help:"GCP project ID" required:""`
	Max     int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page    string `name:"page" help:"Page token"`
}

func (c *ServiceAccountsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	project := strings.TrimSpace(c.Project)
	if project == "" {
		return usage("--project is required")
	}

	svc, err := newIAMService(ctx, account)
	if err != nil {
		return err
	}

	parent := "projects/" + project
	call := svc.Projects.ServiceAccounts.List(parent).PageSize(c.Max)
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list service accounts: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Accounts) == 0 {
		u.Err().Println("No service accounts found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "EMAIL\tNAME\tRESOURCE")
	for _, sa := range resp.Accounts {
		if sa == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(sa.Email),
			sanitizeTab(sa.DisplayName),
			sanitizeTab(sa.Name),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ServiceAccountsCreateCmd struct {
	Project     string `name:"project" help:"GCP project ID" required:""`
	Name        string `name:"name" help:"Service account ID (short name)" required:""`
	DisplayName string `name:"display-name" help:"Display name"`
}

func (c *ServiceAccountsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	project := strings.TrimSpace(c.Project)
	accountID := strings.TrimSpace(c.Name)
	if project == "" || accountID == "" {
		return usage("--project and --name are required")
	}
	if !isValidServiceAccountID(accountID) {
		return usage("--name must be a valid service account ID (lowercase letters, digits, hyphens)")
	}

	display := strings.TrimSpace(c.DisplayName)
	if display == "" {
		display = accountID
	}

	svc, err := newIAMService(ctx, account)
	if err != nil {
		return err
	}

	parent := "projects/" + project
	req := &iam.CreateServiceAccountRequest{
		AccountId: accountID,
		ServiceAccount: &iam.ServiceAccount{
			DisplayName: display,
		},
	}

	resp, err := svc.Projects.ServiceAccounts.Create(parent, req).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create service account: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	u.Out().Printf("Created service account %s\n", resp.Email)
	return nil
}

type ServiceAccountsDeleteCmd struct {
	ServiceAccount string `arg:"" name:"service-account" help:"Service account email or resource name"`
}

func (c *ServiceAccountsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	sa := strings.TrimSpace(c.ServiceAccount)
	if sa == "" {
		return usage("service account is required")
	}

	if err = confirmDestructive(ctx, flags, fmt.Sprintf("delete service account %s", sa)); err != nil {
		return err
	}

	svc, err := newIAMService(ctx, account)
	if err != nil {
		return err
	}

	name := normalizeServiceAccountName(sa)
	if _, err := svc.Projects.ServiceAccounts.Delete(name).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete service account %s: %w", sa, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"deleted": true, "serviceAccount": sa})
	}

	u.Out().Printf("Deleted service account %s\n", sa)
	return nil
}

type ServiceAccountsKeysCmd struct {
	List   ServiceAccountsKeysListCmd   `cmd:"" name:"list" aliases:"ls" help:"List service account keys"`
	Create ServiceAccountsKeysCreateCmd `cmd:"" name:"create" help:"Create a service account key"`
	Delete ServiceAccountsKeysDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete a service account key"`
}

type ServiceAccountsKeysListCmd struct {
	ServiceAccount string `arg:"" name:"service-account" help:"Service account email or resource name"`
}

func (c *ServiceAccountsKeysListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	sa := strings.TrimSpace(c.ServiceAccount)
	if sa == "" {
		return usage("service account is required")
	}

	svc, err := newIAMService(ctx, account)
	if err != nil {
		return err
	}

	name := normalizeServiceAccountName(sa)
	resp, err := svc.Projects.ServiceAccounts.Keys.List(name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list keys: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Keys) == 0 {
		u.Err().Println("No keys found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "KEY\tTYPE\tCREATED\tEXPIRES")
	for _, key := range resp.Keys {
		if key == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(key.Name),
			sanitizeTab(key.KeyType),
			sanitizeTab(key.ValidAfterTime),
			sanitizeTab(key.ValidBeforeTime),
		)
	}
	return nil
}

type ServiceAccountsKeysCreateCmd struct {
	ServiceAccount string `arg:"" name:"service-account" help:"Service account email or resource name"`
	Output         string `name:"output" help:"Output file path" required:""`
}

func (c *ServiceAccountsKeysCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	sa := strings.TrimSpace(c.ServiceAccount)
	if sa == "" {
		return usage("service account is required")
	}
	output := strings.TrimSpace(c.Output)
	if output == "" {
		return usage("--output is required")
	}

	svc, err := newIAMService(ctx, account)
	if err != nil {
		return err
	}

	name := normalizeServiceAccountName(sa)
	resp, err := svc.Projects.ServiceAccounts.Keys.Create(name, &iam.CreateServiceAccountKeyRequest{
		PrivateKeyType: "TYPE_GOOGLE_CREDENTIALS_FILE",
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create key: %w", err)
	}

	payload, err := base64.StdEncoding.DecodeString(resp.PrivateKeyData)
	if err != nil {
		return fmt.Errorf("decode key data: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o750); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := os.WriteFile(output, payload, 0o600); err != nil {
		return fmt.Errorf("write key file: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"key": resp.Name, "output": output})
	}

	u.Out().Printf("Created key %s\n", resp.Name)
	u.Out().Printf("Wrote credentials to %s\n", output)
	return nil
}

type ServiceAccountsKeysDeleteCmd struct {
	ServiceAccount string `arg:"" name:"service-account" help:"Service account email or resource name"`
	KeyID          string `arg:"" name:"key" help:"Key ID or resource name"`
}

func (c *ServiceAccountsKeysDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	sa := strings.TrimSpace(c.ServiceAccount)
	key := strings.TrimSpace(c.KeyID)
	if sa == "" || key == "" {
		return usage("service account and key are required")
	}

	if err = confirmDestructive(ctx, flags, fmt.Sprintf("delete key %s", key)); err != nil {
		return err
	}

	svc, err := newIAMService(ctx, account)
	if err != nil {
		return err
	}

	keyName := normalizeServiceAccountKeyName(sa, key)
	if _, err := svc.Projects.ServiceAccounts.Keys.Delete(keyName).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete key %s: %w", key, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"deleted": true, "key": key})
	}

	u.Out().Printf("Deleted key %s\n", key)
	return nil
}

func normalizeServiceAccountName(sa string) string {
	trimmed := strings.TrimSpace(sa)
	if strings.HasPrefix(trimmed, "projects/") {
		return trimmed
	}
	return "projects/-/serviceAccounts/" + trimmed
}

func normalizeServiceAccountKeyName(sa, key string) string {
	trimmed := strings.TrimSpace(key)
	if strings.HasPrefix(trimmed, "projects/") {
		return trimmed
	}
	return fmt.Sprintf("%s/keys/%s", normalizeServiceAccountName(sa), trimmed)
}

func isValidServiceAccountID(id string) bool {
	if id == "" {
		return false
	}
	for i, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
		if i == 0 && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}
