package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type AuthCredentialsCmd struct {
	Set    AuthCredentialsSetCmd    `cmd:"" default:"withargs" help:"Store OAuth client credentials"`
	List   AuthCredentialsListCmd   `cmd:"" name:"list" help:"List stored OAuth client credentials"`
	Remove AuthCredentialsRemoveCmd `cmd:"" name:"remove" help:"Remove stored OAuth client credentials"`
}

type AuthCredentialsSetCmd struct {
	Path    string `arg:"" name:"credentials" help:"Path to credentials.json or '-' for stdin"`
	Domains string `name:"domain" help:"Comma-separated domains to map to this client (e.g. example.com)"`
}

func (c *AuthCredentialsSetCmd) Run(ctx context.Context, _ *RootFlags) error {
	u := ui.FromContext(ctx)
	client, err := normalizeClientForFlag(authclient.ClientOverrideFromContext(ctx))
	if err != nil {
		return err
	}
	inPath := c.Path
	var b []byte
	if inPath == "-" {
		b, err = io.ReadAll(os.Stdin)
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

	creds, err := config.ParseGoogleOAuthClientJSON(b)
	if err != nil {
		return err
	}

	if err := config.WriteClientCredentialsFor(client, creds); err != nil {
		return err
	}

	outPath, _ := config.ClientCredentialsPathFor(client)
	if strings.TrimSpace(c.Domains) != "" {
		cfg, err := config.ReadConfig()
		if err != nil {
			return err
		}
		for _, domain := range splitCommaList(c.Domains) {
			if err := config.SetClientDomain(&cfg, domain, client); err != nil {
				return err
			}
		}
		if err := config.WriteConfig(cfg); err != nil {
			return err
		}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"saved":  true,
			"path":   outPath,
			"client": client,
		})
	}
	u.Out().Printf("path\t%s", outPath)
	u.Out().Printf("client\t%s", client)
	return nil
}

type AuthCredentialsListCmd struct{}

func (c *AuthCredentialsListCmd) Run(ctx context.Context, _ *RootFlags) error {
	u := ui.FromContext(ctx)
	cfg, err := config.ReadConfig()
	if err != nil {
		return err
	}
	creds, err := config.ListClientCredentials()
	if err != nil {
		return err
	}

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
		Client  string   `json:"client"`
		Path    string   `json:"path,omitempty"`
		Default bool     `json:"default"`
		Domains []string `json:"domains,omitempty"`
	}

	entries := make([]entry, 0, len(creds))
	seen := make(map[string]struct{})
	for _, info := range creds {
		domains := domainMap[info.Client]
		sort.Strings(domains)
		entries = append(entries, entry{
			Client:  info.Client,
			Path:    info.Path,
			Default: info.Default,
			Domains: domains,
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
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"clients": []entry{}})
		}
		u.Err().Println("No OAuth client credentials stored")
		return nil
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"clients": entries})
	}

	w, done := tableWriter(ctx)
	defer done()
	_, _ = fmt.Fprintln(w, "CLIENT\tPATH\tDOMAINS")
	for _, e := range entries {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", e.Client, e.Path, strings.Join(e.Domains, ","))
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

	accounts, err := accountsForClient(client)
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

	if deleteErr := config.DeleteClientCredentialsFor(client); deleteErr != nil {
		return deleteErr
	}

	tokensRemoved, err := removeTokensForClient(client, accounts)
	if err != nil {
		return err
	}
	domainsRemoved, err := removeDomainMappings(client)
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
	creds, err := config.ListClientCredentials()
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
		accounts, accountsErr := accountsForClient(info.Client)
		if accountsErr != nil {
			return accountsErr
		}
		planned = append(planned, authCredentialsRemovalResult{
			Client:        info.Client,
			TokensRemoved: accounts,
		})
	}
	if dryRunErr := dryRunExit(ctx, flags, "auth.credentials.remove_all", planned); dryRunErr != nil {
		return dryRunErr
	}
	if err := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), fmt.Sprintf("remove all OAuth credentials (%s)", strings.Join(names, ", "))); err != nil {
		return err
	}

	var allTokens []string
	var allDomains []string
	for _, item := range planned {
		if err := config.DeleteClientCredentialsFor(item.Client); err != nil {
			return err
		}
		tokens, err := removeTokensForClient(item.Client, item.TokensRemoved)
		if err != nil {
			return err
		}
		allTokens = append(allTokens, tokens...)
		domains, err := removeDomainMappings(item.Client)
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

// accountsForClient returns emails that have tokens stored under the given client.
func accountsForClient(client string) ([]string, error) {
	store, err := openSecretsStore()
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
func removeTokensForClient(client string, emails []string) ([]string, error) {
	if len(emails) == 0 {
		return nil, nil
	}
	store, err := openSecretsStore()
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
func removeDomainMappings(client string) ([]string, error) {
	cfg, err := config.ReadConfig()
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
		if err := config.WriteConfig(cfg); err != nil {
			return nil, err
		}
	}
	return removed, nil
}
