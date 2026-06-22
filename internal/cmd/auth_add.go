package cmd

import (
	"context"
	"fmt"
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

type AuthAddCmd struct {
	Email        string        `arg:"" name:"email" help:"Email"`
	Manual       bool          `name:"manual" help:"Browserless auth flow (paste redirect URL)"`
	Remote       bool          `name:"remote" help:"Remote/server-friendly manual flow (print URL, then exchange code)"`
	Step         int           `name:"step" help:"Remote auth step: 1=print URL, 2=exchange code"`
	ListenAddr   string        `name:"listen-addr" help:"Address to listen on for OAuth callback (for example 0.0.0.0 or 0.0.0.0:8080)"`
	RedirectHost string        `name:"redirect-host" help:"Hostname for OAuth callback in browser flows; builds https://{host}/oauth2/callback"`
	RedirectURI  string        `name:"redirect-uri" help:"Override OAuth redirect URI for manual/remote flows (for example https://host.example/oauth2/callback)"`
	AuthURL      string        `name:"auth-url" help:"Redirect URL from browser (manual flow; required for --remote --step 2)"`
	AuthCode     string        `name:"auth-code" hidden:"" help:"UNSAFE: Authorization code from browser (manual flow; skips state check; not valid with --remote)"`
	Timeout      time.Duration `name:"timeout" help:"Authorization timeout (manual flows default to 5m)"`
	ForceConsent bool          `name:"force-consent" help:"Force consent screen to obtain a refresh token"`
	ServicesCSV  string        `name:"services" help:"Services to authorize: user|all-user or comma-separated ${auth_services}; explicit opt-in: photospicker; all means all default user OAuth services. Workspace service-account-only services: admin, groups, keep" default:"user"`
	DriveScope   string        `name:"drive-scope" help:"Drive scope mode: full|readonly|file" enum:"full,readonly,file" default:"full"`
	GmailScope   string        `name:"gmail-scope" help:"Gmail scope mode: full|readonly" enum:"full,readonly" default:"full"`
	ExtraScopes  string        `name:"extra-scopes" help:"Comma-separated list of additional OAuth scope URIs to request (appended after service scopes)"`
}

func formatRemoteStep2Instruction(services []googleauth.Service, c *AuthAddCmd, readonly bool) string {
	parts := []string{"--remote", "--step", "2", "--auth-url", "<redirect-url>"}
	if redirectHost := strings.TrimSpace(c.RedirectHost); redirectHost != "" {
		parts = append(parts, "--redirect-host", redirectHost)
	}
	if redirectURI := strings.TrimSpace(c.RedirectURI); redirectURI != "" {
		parts = append(parts, "--redirect-uri", redirectURI)
	}
	if len(services) > 0 {
		serialized := make([]string, 0, len(services))
		for _, service := range services {
			serialized = append(serialized, string(service))
		}
		parts = append(parts, "--services", strings.Join(serialized, ","))
	}
	if readonly {
		parts = append(parts, "--readonly")
	}
	if driveScope := strings.ToLower(strings.TrimSpace(c.DriveScope)); driveScope != "" && driveScope != string(googleauth.DriveScopeFull) {
		parts = append(parts, "--drive-scope", driveScope)
	}
	if gmailScope := strings.ToLower(strings.TrimSpace(c.GmailScope)); gmailScope != "" && gmailScope != string(googleauth.GmailScopeFull) {
		parts = append(parts, "--gmail-scope", gmailScope)
	}
	if extraScopes := parseExtraScopesCSV(c.ExtraScopes); len(extraScopes) > 0 {
		parts = append(parts, "--extra-scopes", strings.Join(extraScopes, ","))
	}
	if c.ForceConsent {
		parts = append(parts, "--force-consent")
	}
	return strings.Join(parts, " ")
}

func parseExtraScopesCSV(raw string) []string {
	var scopes []string
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			scopes = append(scopes, s)
		}
	}
	return scopes
}

func (c *AuthAddCmd) resolvedRedirectURI() (string, error) {
	redirectURI := strings.TrimSpace(c.RedirectURI)
	if strings.TrimSpace(c.RedirectHost) != "" && redirectURI != "" {
		return "", usage("cannot combine --redirect-host with --redirect-uri")
	}
	if strings.TrimSpace(c.RedirectHost) == "" {
		return redirectURI, nil
	}
	return redirectURIFromHost(c.RedirectHost)
}

func (c *AuthAddCmd) isManualFlow(authURL, authCode string) bool {
	return c.Manual || c.Remote || authURL != "" || authCode != "" || strings.TrimSpace(c.RedirectURI) != ""
}

func (c *AuthAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	readonly := readOnlyEnabled(flags)

	override := authclient.ClientOverrideFromContext(ctx)
	client, err := authclient.ResolveClientWithOverride(ctx, c.Email, override)
	if err != nil {
		return err
	}

	services, err := parseAuthServices(c.ServicesCSV)
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return fmt.Errorf("no services selected")
	}

	driveScope := strings.ToLower(strings.TrimSpace(c.DriveScope))
	if readonly && driveScope == strFile {
		return usage("cannot combine --readonly with --drive-scope=file (file is write-capable)")
	}
	gmailScope := strings.ToLower(strings.TrimSpace(c.GmailScope))
	disableIncludeGrantedScopes := readonly ||
		driveScope == "readonly" ||
		driveScope == strFile ||
		gmailScope == "readonly"

	extraScopes := parseExtraScopesCSV(c.ExtraScopes)

	scopes, err := googleauth.ScopesForManageWithOptions(services, googleauth.ScopeOptions{
		Readonly:    readonly,
		DriveScope:  googleauth.DriveScopeMode(driveScope),
		GmailScope:  googleauth.GmailScopeMode(gmailScope),
		ExtraScopes: extraScopes,
	})
	if err != nil {
		return err
	}

	authURL := strings.TrimSpace(c.AuthURL)
	authCode := strings.TrimSpace(c.AuthCode)
	redirectURI, err := c.resolvedRedirectURI()
	if err != nil {
		return err
	}
	if authURL != "" && authCode != "" {
		return usage("cannot combine --auth-url with --auth-code")
	}
	if c.Step != 0 && c.Step != 1 && c.Step != 2 {
		return usage("step must be 1 or 2")
	}
	if c.Step != 0 && !c.Remote {
		return usage("--step requires --remote")
	}

	manual := c.isManualFlow(authURL, authCode)

	if c.Remote {
		step := c.Step
		if step == 0 {
			if authURL != "" || authCode != "" {
				step = 2
			} else {
				step = 1
			}
		}
		switch step {
		case 1:
			if authURL != "" || authCode != "" {
				return usage("remote step 1 does not accept --auth-url or --auth-code")
			}
			result, manualErr := buildManualAuthURL(ctx, googleauth.AuthorizeOptions{
				Services:                    services,
				Scopes:                      scopes,
				Manual:                      true,
				ForceConsent:                c.ForceConsent,
				DisableIncludeGrantedScopes: disableIncludeGrantedScopes,
				Client:                      client,
				RedirectURI:                 redirectURI,
			})
			if manualErr != nil {
				return manualErr
			}
			if outfmt.IsJSON(ctx) {
				return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
					"auth_url":     result.URL,
					"state_reused": result.StateReused,
				})
			}
			u.Out().Linef("auth_url\t%s", result.URL)
			u.Out().Linef("state_reused\t%t", result.StateReused)
			u.Err().Linef("Run again with the same root flags and %s", formatRemoteStep2Instruction(services, c, readonly))
			return nil
		case 2:
			if authCode != "" {
				return usage("--auth-code is not valid with --remote (state check is mandatory)")
			}
			if authURL == "" {
				return usage("remote step 2 requires --auth-url")
			}
		}
	}

	timeout := c.Timeout
	if timeout == 0 && manual {
		timeout = 5 * time.Minute
	}

	if dryRunErr := dryRunExit(ctx, flags, "auth.add", map[string]any{
		"email":         strings.TrimSpace(c.Email),
		"client":        client,
		"services":      services,
		"scopes":        scopes,
		"manual":        c.Manual,
		"remote":        c.Remote,
		"step":          c.Step,
		"listen_addr":   strings.TrimSpace(c.ListenAddr),
		"redirect_host": strings.TrimSpace(c.RedirectHost),
		"redirect_uri":  redirectURI,
		"force_consent": c.ForceConsent,
		"readonly":      readonly,
		"drive_scope":   c.DriveScope,
		"gmail_scope":   c.GmailScope,
		"extra_scopes":  extraScopes,
	}); dryRunErr != nil {
		return dryRunErr
	}

	if keychainErr := ensureKeychainAccessIfNeeded(ctx); keychainErr != nil {
		return fmt.Errorf("keychain access: %w", keychainErr)
	}

	refreshToken, err := authorizeGoogleAccount(ctx, googleauth.AuthorizeOptions{
		Services:                    services,
		Scopes:                      scopes,
		Manual:                      manual,
		ForceConsent:                c.ForceConsent,
		DisableIncludeGrantedScopes: disableIncludeGrantedScopes,
		Timeout:                     timeout,
		Client:                      client,
		AuthURL:                     authURL,
		AuthCode:                    authCode,
		ListenAddr:                  strings.TrimSpace(c.ListenAddr),
		RedirectURI:                 redirectURI,
		RequireState:                c.Remote,
	})
	if err != nil {
		return err
	}

	identity, err := fetchAuthIdentity(ctx, client, refreshToken, scopes, 15*time.Second)
	if err != nil {
		return fmt.Errorf("fetch authorized email: %w", err)
	}
	authorizedEmail := identity.Email
	if normalizeEmail(authorizedEmail) != normalizeEmail(c.Email) {
		return fmt.Errorf("authorized as %s, expected %s", authorizedEmail, c.Email)
	}

	store, err := openAuthSecretsStore(ctx)
	if err != nil {
		return wrapAuthAddStoreError(err)
	}
	serviceNames := make([]string, 0, len(services))
	for _, svc := range services {
		serviceNames = append(serviceNames, string(svc))
	}
	sort.Strings(serviceNames)

	migratedEmail, err := googleauth.FindStoredSubjectIdentityEmail(store, client, identity)
	if err != nil {
		return wrapAuthAddStoreError(err)
	}

	if err := store.SetToken(client, authorizedEmail, secrets.Token{
		Client:       client,
		Subject:      identity.Subject,
		Email:        authorizedEmail,
		Services:     serviceNames,
		Scopes:       scopes,
		RefreshToken: refreshToken,
	}); err != nil {
		return wrapAuthAddStoreError(err)
	}
	if migratedEmail != "" {
		if err := googleauth.MigrateStoredEmailReferences(store, func(oldEmail, newEmail string) error {
			return authclient.UpdateEmailReferences(ctx, oldEmail, newEmail)
		}, client, migratedEmail, authorizedEmail); err != nil {
			return wrapAuthAddStoreError(err)
		}
		if err := googleauth.DeleteStoredEmailAlias(store, client, migratedEmail); err != nil {
			u.Err().Linef("Warning: failed to remove stale auth account %s: %v", migratedEmail, err)
		}
		u.Err().Linef("Migrated auth account from %s to %s", migratedEmail, authorizedEmail)
	}
	if override != "" {
		configStore, err := commandConfigStore(ctx)
		if err != nil {
			return err
		}
		cfg, err := configStore.Read()
		if err != nil {
			return err
		}
		if err := config.SetAccountClient(&cfg, authorizedEmail, client); err != nil {
			return err
		}
		if err := configStore.Write(cfg); err != nil {
			return err
		}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"stored":   true,
			"email":    authorizedEmail,
			"services": serviceNames,
			"client":   client,
		})
	}
	u.Out().Linef("email\t%s", authorizedEmail)
	u.Out().Linef("services\t%s", strings.Join(serviceNames, ","))
	u.Out().Linef("client\t%s", client)
	return nil
}

func readOnlyEnabled(flags *RootFlags) bool {
	return flags != nil && flags.ReadOnly
}

func wrapAuthAddStoreError(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("OAuth completed, but saving the refresh token failed: %w", err)
}
