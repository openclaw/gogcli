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

	accounts := findAccountsForClient(client)

	action := fmt.Sprintf("remove OAuth credentials for client %q", client)
	if len(accounts) > 0 {
		action += fmt.Sprintf(" and %d associated token(s) (%s)", len(accounts), strings.Join(accounts, ", "))
	}
	if err := confirmDestructive(ctx, flags, action); err != nil {
		return err
	}

	if err := config.DeleteClientCredentialsFor(client); err != nil {
		return err
	}

	tokensRemoved := removeTokensForClient(client, accounts)
	domainsRemoved := removeDomainMappings(client)

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
	for _, info := range creds {
		names = append(names, info.Client)
	}
	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove all OAuth credentials (%s)", strings.Join(names, ", "))); err != nil {
		return err
	}

	var allTokens []string
	for _, info := range creds {
		accounts := findAccountsForClient(info.Client)
		if err := config.DeleteClientCredentialsFor(info.Client); err != nil {
			return err
		}
		allTokens = append(allTokens, removeTokensForClient(info.Client, accounts)...)
		removeDomainMappings(info.Client)
	}

	return writeResult(ctx, u,
		kv("removed", len(creds)),
		kv("clients", names),
		kv("tokens_removed", allTokens),
	)
}

// findAccountsForClient returns emails that have tokens stored under the given client.
func findAccountsForClient(client string) []string {
	store, err := openSecretsStore()
	if err != nil {
		return nil
	}
	tokens, err := store.ListTokens()
	if err != nil {
		return nil
	}
	var emails []string
	for _, tok := range tokens {
		tokClient, _ := config.NormalizeClientNameOrDefault(tok.Client)
		if tokClient == client {
			emails = append(emails, tok.Email)
		}
	}
	return emails
}

// removeTokensForClient deletes tokens for the given accounts under the specified client.
func removeTokensForClient(client string, emails []string) []string {
	if len(emails) == 0 {
		return nil
	}
	store, err := openSecretsStore()
	if err != nil {
		return nil
	}
	var removed []string
	for _, email := range emails {
		if err := store.DeleteToken(client, email); err == nil {
			removed = append(removed, email)
		}
	}
	return removed
}

// removeDomainMappings deletes config domain entries that point to the given client.
func removeDomainMappings(client string) []string {
	cfg, err := config.ReadConfig()
	if err != nil {
		return nil
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
		_ = config.WriteConfig(cfg)
	}
	return removed
}
