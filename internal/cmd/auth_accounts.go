package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

type AuthListCmd struct {
	Check   bool          `name:"check" help:"Verify refresh tokens by exchanging for an access token (requires credentials.json)"`
	Timeout time.Duration `name:"timeout" help:"Per-token check timeout" default:"15s"`
}

type AuthStatusCmd struct{}

func (c *AuthStatusCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	configPath, err := config.ConfigPath()
	if err != nil {
		return err
	}
	configExists, err := config.ConfigExists()
	if err != nil {
		return err
	}
	backendInfo, err := secrets.ResolveKeyringBackendInfo()
	if err != nil {
		return err
	}

	account := ""
	authPreferred := ""
	serviceAccountConfigured := false
	serviceAccountPath := ""
	client := ""
	credentialsPath := ""
	credentialsExists := false

	if flags != nil {
		if a, err := requireAccount(flags); err == nil {
			account = a
			resolvedClient, resolveErr := resolveClientForEmail(account, flags, "")
			if resolveErr != nil {
				return resolveErr
			}
			client = resolvedClient
			path, pathErr := config.ClientCredentialsPathFor(client)
			if pathErr == nil {
				credentialsPath = path
				if st, statErr := os.Stat(path); statErr == nil && !st.IsDir() {
					credentialsExists = true
				}
			}
			if p, _, ok := bestServiceAccountPathAndMtime(normalizeEmail(account)); ok {
				serviceAccountConfigured = true
				serviceAccountPath = p
			}
			if serviceAccountConfigured {
				authPreferred = authTypeServiceAccount
			} else {
				authPreferred = authTypeOAuth
			}
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"config": map[string]any{
				"path":   configPath,
				"exists": configExists,
			},
			"keyring": map[string]any{
				"backend": backendInfo.Value,
				"source":  backendInfo.Source,
			},
			"account": map[string]any{
				"email":                      account,
				"client":                     client,
				"credentials_path":           credentialsPath,
				"credentials_exists":         credentialsExists,
				"auth_preferred":             authPreferred,
				"service_account_configured": serviceAccountConfigured,
				"service_account_path":       serviceAccountPath,
			},
		})
	}
	u.Out().Printf("config_path\t%s", configPath)
	u.Out().Printf("config_exists\t%t", configExists)
	u.Out().Printf("keyring_backend\t%s", backendInfo.Value)
	u.Out().Printf("keyring_backend_source\t%s", backendInfo.Source)
	if account != "" {
		u.Out().Printf("account\t%s", account)
		u.Out().Printf("client\t%s", client)
		if credentialsPath != "" {
			u.Out().Printf("credentials_path\t%s", credentialsPath)
		}
		u.Out().Printf("credentials_exists\t%t", credentialsExists)
		u.Out().Printf("auth_preferred\t%s", authPreferred)
		u.Out().Printf("service_account_configured\t%t", serviceAccountConfigured)
		if serviceAccountPath != "" {
			u.Out().Printf("service_account_path\t%s", serviceAccountPath)
		}
	}
	return nil
}

func (c *AuthListCmd) Run(ctx context.Context, _ *RootFlags) error {
	u := ui.FromContext(ctx)
	store, err := openSecretsStore()
	if err != nil {
		return err
	}
	tokens, err := store.ListTokens()
	if err != nil {
		return err
	}

	serviceAccountEmails, err := config.ListServiceAccountEmails()
	if err != nil {
		return err
	}

	sort.Slice(tokens, func(i, j int) bool { return tokens[i].Email < tokens[j].Email })

	type tokenByEmail struct {
		tok secrets.Token
		ok  bool
	}
	tokMap := make(map[string]tokenByEmail, len(tokens))
	for _, t := range tokens {
		email := normalizeEmail(t.Email)
		if email == "" {
			continue
		}
		tokMap[email] = tokenByEmail{tok: t, ok: true}
	}

	type entry struct {
		Email string
		Token *secrets.Token
		SA    bool
	}
	entries := make([]entry, 0, len(tokens)+len(serviceAccountEmails))
	seen := make(map[string]struct{})
	for _, email := range serviceAccountEmails {
		email = normalizeEmail(email)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		te := tokMap[email]
		var tok *secrets.Token
		if te.ok {
			t := te.tok
			tok = &t
		}
		entries = append(entries, entry{Email: email, Token: tok, SA: true})
	}
	for _, t := range tokens {
		email := normalizeEmail(t.Email)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		t2 := t
		entries = append(entries, entry{Email: email, Token: &t2, SA: false})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Email < entries[j].Email })

	if outfmt.IsJSON(ctx) {
		type item struct {
			Email     string   `json:"email"`
			Client    string   `json:"client,omitempty"`
			Services  []string `json:"services,omitempty"`
			Scopes    []string `json:"scopes,omitempty"`
			CreatedAt string   `json:"created_at,omitempty"`
			Auth      string   `json:"auth"`
			Valid     *bool    `json:"valid,omitempty"`
			Error     string   `json:"error,omitempty"`
		}
		out := make([]item, 0, len(entries))
		for _, e := range entries {
			auth := authTypeOAuth
			if e.SA {
				auth = authTypeServiceAccount
			}
			if e.Token != nil && e.SA {
				auth = authTypeOAuthServiceAccount
			}

			created := ""
			services := []string(nil)
			scopes := []string(nil)

			if e.Token != nil {
				if !e.Token.CreatedAt.IsZero() {
					created = e.Token.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
				}
				services = e.Token.Services
				scopes = e.Token.Scopes
			} else if e.SA {
				if _, mtime, ok := bestServiceAccountPathAndMtime(e.Email); ok {
					created = mtime.UTC().Format("2006-01-02T15:04:05Z07:00")
				}
				services = []string{"service-account"}
			}

			it := item{
				Email:     e.Email,
				Client:    "",
				Services:  services,
				Scopes:    scopes,
				CreatedAt: created,
				Auth:      auth,
			}
			if e.Token != nil {
				it.Client = e.Token.Client
			}
			if c.Check {
				if e.Token == nil {
					valid := true
					it.Valid = &valid
					it.Error = "service account (not checked)"
				} else {
					err := checkRefreshToken(ctx, e.Token.Client, e.Token.RefreshToken, e.Token.Scopes, c.Timeout)
					valid := err == nil
					it.Valid = &valid
					if err != nil {
						it.Error = err.Error()
					}
				}
			}
			out = append(out, it)
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"accounts": out})
	}
	if len(entries) == 0 {
		u.Err().Println("No tokens stored")
		return nil
	}

	for _, e := range entries {
		auth := authTypeOAuth
		if e.SA {
			auth = authTypeServiceAccount
		}
		if e.Token != nil && e.SA {
			auth = authTypeOAuthServiceAccount
		}

		client := ""
		if e.Token != nil {
			client = e.Token.Client
		}
		created := ""
		servicesCSV := ""

		if e.Token != nil {
			if !e.Token.CreatedAt.IsZero() {
				created = e.Token.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			}
			servicesCSV = strings.Join(e.Token.Services, ",")
		} else if e.SA {
			if _, mtime, ok := bestServiceAccountPathAndMtime(e.Email); ok {
				created = mtime.UTC().Format("2006-01-02T15:04:05Z07:00")
			}
			servicesCSV = "service-account"
		}

		if c.Check {
			if e.Token == nil {
				u.Out().Printf("%s\t%s\t%s\t%s\t%t\t%s\t%s", e.Email, client, servicesCSV, created, true, "service account (not checked)", auth)
				continue
			}

			err := checkRefreshToken(ctx, e.Token.Client, e.Token.RefreshToken, e.Token.Scopes, c.Timeout)
			valid := err == nil
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			u.Out().Printf("%s\t%s\t%s\t%s\t%t\t%s\t%s", e.Email, client, servicesCSV, created, valid, msg, auth)
			continue
		}

		u.Out().Printf("%s\t%s\t%s\t%s\t%s", e.Email, client, servicesCSV, created, auth)
	}
	return nil
}

func bestServiceAccountPathAndMtime(email string) (string, time.Time, bool) {
	if p, err := config.ServiceAccountPath(email); err == nil {
		if st, err := os.Stat(p); err == nil {
			return p, st.ModTime(), true
		}
	}
	if p, err := config.KeepServiceAccountPath(email); err == nil {
		if st, err := os.Stat(p); err == nil {
			return p, st.ModTime(), true
		}
	}
	if p, err := config.KeepServiceAccountLegacyPath(email); err == nil {
		if st, err := os.Stat(p); err == nil {
			return p, st.ModTime(), true
		}
	}
	return "", time.Time{}, false
}

type AuthServicesCmd struct {
	Markdown bool `name:"markdown" help:"Output Markdown table"`
}

func (c *AuthServicesCmd) Run(ctx context.Context, _ *RootFlags) error {
	infos := googleauth.ServicesInfo()
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"services": infos})
	}
	if c.Markdown {
		_, err := io.WriteString(os.Stdout, googleauth.ServicesMarkdown(infos))
		return err
	}

	w, done := tableWriter(ctx)
	defer done()

	_, _ = fmt.Fprintln(w, "SERVICE\tUSER\tAPIS\tSCOPES\tNOTE")
	for _, info := range infos {
		_, _ = fmt.Fprintf(
			w,
			"%s\t%t\t%s\t%s\t%s\n",
			info.Service,
			info.User,
			strings.Join(info.APIs, ", "),
			strings.Join(info.Scopes, ", "),
			info.Note,
		)
	}
	return nil
}

type AuthRemoveCmd struct {
	Email string `arg:"" name:"email" help:"Email"`
}

func (c *AuthRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove stored token for %s", email)); err != nil {
		return err
	}
	store, err := openSecretsStore()
	if err != nil {
		return err
	}
	client, err := resolveClientForEmail(email, flags, "")
	if err != nil {
		return err
	}
	if err := store.DeleteToken(client, email); err != nil {
		return err
	}
	return writeResult(ctx, u,
		kv("deleted", true),
		kv("email", email),
		kv("client", client),
	)
}

type AuthManageCmd struct {
	ForceConsent bool          `name:"force-consent" help:"Force consent screen when adding accounts"`
	ServicesCSV  string        `name:"services" help:"Services to authorize: user|all or comma-separated ${auth_services} (Keep uses service account: gog auth service-account set)" default:"user"`
	Timeout      time.Duration `name:"timeout" help:"Server timeout duration" default:"10m"`
	ListenAddr   string        `name:"listen-addr" help:"Address to listen on for OAuth callback (for example 0.0.0.0 or 0.0.0.0:8080)"`
	RedirectHost string        `name:"redirect-host" help:"Hostname for OAuth callback; builds https://{host}/oauth2/callback"`
}

func (c *AuthManageCmd) Run(ctx context.Context, _ *RootFlags) error {
	services, err := parseAuthServices(c.ServicesCSV)
	if err != nil {
		return err
	}
	redirectURI := ""
	if strings.TrimSpace(c.RedirectHost) != "" {
		redirectURI, err = redirectURIFromHost(c.RedirectHost)
		if err != nil {
			return err
		}
	}

	return startManageServer(ctx, googleauth.ManageServerOptions{
		Timeout:      c.Timeout,
		Services:     services,
		ForceConsent: c.ForceConsent,
		Client:       authclient.ClientOverrideFromContext(ctx),
		ListenAddr:   strings.TrimSpace(c.ListenAddr),
		RedirectURI:  redirectURI,
	})
}

type AuthKeepCmd struct {
	Email string `arg:"" name:"email" help:"Email to impersonate when using Keep"`
	Key   string `name:"key" required:"" help:"Path to service account JSON key file"`
}

func (c *AuthKeepCmd) Run(ctx context.Context, _ *RootFlags) error {
	u := ui.FromContext(ctx)

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	keyPath := strings.TrimSpace(c.Key)
	if keyPath == "" {
		return usage("empty key path")
	}
	keyPath, err := config.ExpandPath(keyPath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(keyPath) //nolint:gosec // user-provided path
	if err != nil {
		return fmt.Errorf("read service account key: %w", err)
	}

	if _, parseErr := parseServiceAccountJSON(data); parseErr != nil {
		return parseErr
	}

	destPath, err := config.KeepServiceAccountPath(email)
	if err != nil {
		return err
	}
	genericPath, err := config.ServiceAccountPath(email)
	if err != nil {
		return err
	}

	if _, err := config.EnsureDir(); err != nil {
		return err
	}

	if err := writePrivateFile(destPath, data, 0o600); err != nil {
		return fmt.Errorf("write service account: %w", err)
	}
	if err := writePrivateFile(genericPath, data, 0o600); err != nil {
		return fmt.Errorf("write service account: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"stored": true,
			"email":  email,
			"path":   destPath,
			"paths":  []string{destPath, genericPath},
		})
	}
	u.Out().Printf("email\t%s", email)
	u.Out().Printf("path\t%s", destPath)
	u.Out().Println("Keep service account configured. Use: gog keep list --account " + email)
	return nil
}
