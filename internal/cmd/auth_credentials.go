package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/oauthclient"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var errOAuthCredentialSecretStoreRequired = errors.New("OAuth credential secret store is required")

type AuthCredentialsCmd struct {
	Set    AuthCredentialsSetCmd    `cmd:"" default:"withargs" help:"Store OAuth client credentials"`
	List   AuthCredentialsListCmd   `cmd:"" name:"list" help:"List stored OAuth client credentials"`
	Remove AuthCredentialsRemoveCmd `cmd:"" name:"remove" help:"Remove stored OAuth client credentials"`
}

type AuthCredentialsSetCmd struct {
	Path      string `arg:"" name:"credentials" help:"Path to credentials.json or '-' for stdin"`
	Domains   string `name:"domain" help:"Comma-separated domains to map to this client (e.g. example.com)"`
	ExpandEnv bool   `name:"expand-env" help:"Expand environment placeholders in client_id/client_secret values"`
	Insecure  bool   `name:"insecure" help:"Store OAuth client_secret in credentials.json instead of the keyring"`
}

func (c *AuthCredentialsSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	client, err := normalizeClientForFlag(authclient.ClientOverrideFromContext(ctx))
	if err != nil {
		return err
	}
	inPath := c.Path
	var b []byte
	if inPath == "-" {
		b, err = io.ReadAll(stdinReader(ctx))
	} else {
		inPath, err = config.ExpandPath(inPath)
		if err != nil {
			return err
		}
		b, err = os.ReadFile(inPath) //nolint:gosec // user-provided path
	}
	if err != nil {
		return err
	}

	creds, err := config.ParseGoogleOAuthClientJSONWithOptions(b, config.ParseGoogleOAuthClientJSONOptions{
		ExpandEnv: c.ExpandEnv,
	})
	if err != nil {
		return err
	}

	domains := splitCommaList(c.Domains)
	var (
		configStore *config.ConfigStore
		cfg         config.File
	)
	if len(domains) > 0 {
		configStore, err = commandConfigStore(ctx)
		if err != nil {
			return err
		}
		cfg, err = configStore.Read()
		if err != nil {
			return err
		}
		for _, domain := range domains {
			if setErr := config.SetClientDomain(&cfg, domain, client); setErr != nil {
				return setErr
			}
		}
	}

	if dryRunErr := dryRunExit(ctx, flags, "auth.credentials.set", map[string]any{
		"client":                   client,
		"credentials_source":       inPath,
		"domains":                  domains,
		"expand_env":               c.ExpandEnv,
		"client_secret_in_keyring": !c.Insecure,
	}); dryRunErr != nil {
		return dryRunErr
	}

	credentialStore, err := commandOAuthCredentialsStore(ctx)
	if err != nil {
		return err
	}
	if writeErr := credentialStore.Write(client, creds, c.Insecure); writeErr != nil {
		return writeErr
	}

	outPath, err := credentialStore.PathFor(client)
	if err != nil {
		return err
	}
	if configStore != nil {
		if err := configStore.Write(cfg); err != nil {
			return err
		}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"saved":                    true,
			"path":                     outPath,
			"client":                   client,
			"client_secret_in_keyring": !c.Insecure,
		})
	}
	u.Out().Linef("path\t%s", outPath)
	u.Out().Linef("client\t%s", client)
	u.Out().Linef("client_secret_in_keyring\t%t", !c.Insecure)
	return nil
}

type AuthCredentialsListCmd struct{}

func (c *AuthCredentialsListCmd) Run(ctx context.Context, _ *RootFlags) error {
	u := ui.FromContext(ctx)
	configStore, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	cfg, err := configStore.Read()
	if err != nil {
		return err
	}
	files, err := commandClientCredentialsStore(ctx)
	if err != nil {
		return err
	}
	creds, err := files.List()
	if err != nil {
		return err
	}
	credentialStore, _ := commandOAuthCredentialsStore(ctx)

	domainMap := make(map[string][]string)
	for domain, client := range cfg.ClientDomains {
		if strings.TrimSpace(client) == "" {
			continue
		}
		normalizedClient, err := config.NormalizeClientNameOrDefault(client)
		if err != nil {
			continue
		}
		domainMap[normalizedClient] = append(domainMap[normalizedClient], domain)
	}

	type entry struct {
		Client                string   `json:"client"`
		Path                  string   `json:"path,omitempty"`
		Default               bool     `json:"default"`
		Domains               []string `json:"domains,omitempty"`
		ClientSecretInKeyring bool     `json:"client_secret_in_keyring,omitempty"`
	}

	entries := make([]entry, 0, len(creds))
	seen := make(map[string]struct{})
	for _, info := range creds {
		domains := domainMap[info.Client]
		sort.Strings(domains)
		entries = append(entries, entry{
			Client:                info.Client,
			Path:                  info.Path,
			Default:               info.Default,
			Domains:               domains,
			ClientSecretInKeyring: credentialStore != nil && credentialStore.ClientSecretInKeyring(info.Client),
		})
		seen[info.Client] = struct{}{}
	}

	for client, domains := range domainMap {
		if _, ok := seen[client]; ok {
			continue
		}
		sort.Strings(domains)
		entries = append(entries, entry{
			Client:  client,
			Domains: domains,
		})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Client < entries[j].Client })

	if len(entries) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"clients": []entry{}})
		}
		u.Err().Println("No OAuth client credentials stored")
		return nil
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"clients": entries})
	}

	w, done := tableWriter(ctx)
	defer done()
	_, _ = fmt.Fprintln(w, "CLIENT\tPATH\tSECRET_KEYRING\tDOMAINS")
	for _, e := range entries {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%t\t%s\n", e.Client, e.Path, e.ClientSecretInKeyring, strings.Join(e.Domains, ","))
	}
	return nil
}

type AuthCredentialsRemoveCmd struct {
	Client string `arg:"" optional:"" name:"client" help:"Client name to remove (omit for default, or 'all' to remove every client)"`
}

type authCredentialsRemovalResult struct {
	Client         string   `json:"client"`
	TokensRemoved  []string `json:"tokens_removed,omitempty"`
	DomainsRemoved []string `json:"domains_removed,omitempty"`
}

func (c *AuthCredentialsRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	// Determine target client(s): explicit arg > --client flag > default.
	target := strings.TrimSpace(c.Client)
	if target == "" {
		t, err := normalizeClientForFlag(authclient.ClientOverrideFromContext(ctx))
		if err != nil {
			return err
		}
		target = t
	}

	if strings.EqualFold(target, "all") {
		return c.removeAll(ctx, flags, u)
	}

	client, err := config.NormalizeClientNameOrDefault(target)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "auth.credentials.remove", map[string]any{
		"client": client,
	}); dryRunErr != nil {
		return dryRunErr
	}

	accounts, err := accountsForClient(ctx, client)
	if err != nil {
		return err
	}

	action := fmt.Sprintf("remove OAuth credentials for client %q", client)
	if len(accounts) > 0 {
		action += fmt.Sprintf(" and %d associated token(s) (%s)", len(accounts), strings.Join(accounts, ", "))
	}
	if confirmErr := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), action); confirmErr != nil {
		return confirmErr
	}

	credentialStore, err := commandOAuthCredentialsStore(ctx)
	if err != nil {
		return err
	}
	if deleteErr := credentialStore.Delete(client); deleteErr != nil {
		return deleteErr
	}

	tokensRemoved, err := removeTokensForClient(ctx, client, accounts)
	if err != nil {
		return err
	}
	domainsRemoved, err := removeDomainMappings(ctx, client)
	if err != nil {
		return err
	}

	return writeResult(ctx, u,
		kv("removed", true),
		kv("client", client),
		kv("tokens_removed", tokensRemoved),
		kv("domains_removed", domainsRemoved),
	)
}

func (c *AuthCredentialsRemoveCmd) removeAll(ctx context.Context, flags *RootFlags, u *ui.UI) error {
	files, err := commandClientCredentialsStore(ctx)
	if err != nil {
		return err
	}
	creds, err := files.List()
	if err != nil {
		return err
	}
	if len(creds) == 0 {
		return writeResult(ctx, u, kv("removed", 0))
	}

	names := make([]string, 0, len(creds))
	planned := make([]authCredentialsRemovalResult, 0, len(creds))
	for _, info := range creds {
		names = append(names, info.Client)
		accounts, accountsErr := accountsForClient(ctx, info.Client)
		if accountsErr != nil {
			return accountsErr
		}
		planned = append(planned, authCredentialsRemovalResult{
			Client:        info.Client,
			TokensRemoved: accounts,
		})
	}
	if dryRunErr := dryRunExit(ctx, flags, "auth.credentials.remove", planned); dryRunErr != nil {
		return dryRunErr
	}
	if confirmErr := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), fmt.Sprintf("remove all OAuth credentials (%s)", strings.Join(names, ", "))); confirmErr != nil {
		return confirmErr
	}

	var allTokens []string
	var allDomains []string
	credentialStore, err := commandOAuthCredentialsStore(ctx)
	if err != nil {
		return err
	}
	for _, item := range planned {
		if err := credentialStore.Delete(item.Client); err != nil {
			return err
		}
		tokens, err := removeTokensForClient(ctx, item.Client, item.TokensRemoved)
		if err != nil {
			return err
		}
		allTokens = append(allTokens, tokens...)
		domains, err := removeDomainMappings(ctx, item.Client)
		if err != nil {
			return err
		}
		allDomains = append(allDomains, domains...)
	}
	sort.Strings(allTokens)
	sort.Strings(allDomains)

	return writeResult(ctx, u,
		kv("removed", len(creds)),
		kv("clients", names),
		kv("tokens_removed", allTokens),
		kv("domains_removed", allDomains),
	)
}

func commandClientCredentialsStore(ctx context.Context) (*config.ClientCredentialsStore, error) {
	layout, err := commandLayout(ctx, config.PathKindConfig, config.PathKindData)
	if err != nil {
		return nil, err
	}

	return config.NewClientCredentialsStore(layout), nil
}

func commandOAuthCredentialsStore(ctx context.Context) (*oauthclient.CredentialsStore, error) {
	runtime, ok := app.FromContext(ctx)
	if !ok || runtime.Auth.OpenSecretStore == nil {
		return nil, errOAuthCredentialSecretStoreRequired
	}

	secretStore, err := runtime.Auth.OpenSecretStore()
	if err != nil {
		return nil, fmt.Errorf("open OAuth credential secret store: %w", err)
	}
	files, err := commandClientCredentialsStore(ctx)
	if err != nil {
		return nil, err
	}

	return oauthclient.NewCredentialsStore(files, secretStore)
}

func commandClientSecretInKeyring(ctx context.Context, client string) bool {
	store, err := commandOAuthCredentialsStore(ctx)
	return err == nil && store.ClientSecretInKeyring(client)
}

// accountsForClient returns emails that have tokens stored under the given client.
func accountsForClient(ctx context.Context, client string) ([]string, error) {
	store, err := openAuthSecretsStore(ctx)
	if err != nil {
		return nil, err
	}
	tokens, err := store.ListTokens()
	if err != nil {
		return nil, err
	}
	var emails []string
	for _, tok := range tokens {
		tokClient, err := config.NormalizeClientNameOrDefault(tok.Client)
		if err != nil {
			continue
		}
		if tokClient == client {
			emails = append(emails, tok.Email)
		}
	}
	sort.Strings(emails)
	return emails, nil
}

// removeTokensForClient deletes tokens for the given accounts under the specified client.
func removeTokensForClient(ctx context.Context, client string, emails []string) ([]string, error) {
	if len(emails) == 0 {
		return nil, nil
	}
	store, err := openAuthSecretsStore(ctx)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, email := range emails {
		if err := store.DeleteToken(client, email); err != nil {
			return removed, fmt.Errorf("delete token for %s: %w", email, err)
		}
		removed = append(removed, email)
	}
	sort.Strings(removed)
	return removed, nil
}

// removeDomainMappings deletes config domain entries that point to the given client.
func removeDomainMappings(ctx context.Context, client string) ([]string, error) {
	configStore, err := commandConfigStore(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := configStore.Read()
	if err != nil {
		return nil, err
	}
	var removed []string
	for domain, mapped := range cfg.ClientDomains {
		normalized, nerr := config.NormalizeClientNameOrDefault(mapped)
		if nerr != nil {
			continue
		}
		if normalized == client {
			removed = append(removed, domain)
			delete(cfg.ClientDomains, domain)
		}
	}
	if len(removed) > 0 {
		sort.Strings(removed)
		if err := configStore.Write(cfg); err != nil {
			return nil, err
		}
	}
	return removed, nil
}
