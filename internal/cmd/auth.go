package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

func openAuthSecretsStore(ctx context.Context) (secrets.Store, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Auth.OpenSecretsStore != nil {
		return runtime.Auth.OpenSecretsStore()
	}
	return secrets.OpenDefault()
}

func authorizeGoogleAccount(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
	if err := bindManualAuthStateStore(ctx, &opts); err != nil {
		return "", err
	}
	if runtime, ok := app.FromContext(ctx); ok && runtime.Auth.AuthorizeGoogle != nil {
		return runtime.Auth.AuthorizeGoogle(ctx, opts)
	}
	return googleauth.Authorize(ctx, opts)
}

func bindManualAuthStateStore(ctx context.Context, opts *googleauth.AuthorizeOptions) error {
	if !opts.Manual || opts.ManualStateStore != nil {
		return nil
	}
	layout, err := commandLayout(ctx, config.PathKindConfig)
	if err != nil {
		return err
	}
	store, err := googleauth.NewManualStateStore(layout)
	if err != nil {
		return err
	}
	opts.ManualStateStore = store
	return nil
}

func fetchAuthIdentity(
	ctx context.Context,
	client string,
	refreshToken string,
	scopes []string,
	timeout time.Duration,
) (googleauth.Identity, error) {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Auth.FetchAuthorizedIdentity != nil {
		return runtime.Auth.FetchAuthorizedIdentity(ctx, client, refreshToken, scopes, timeout)
	}
	return googleauth.IdentityForRefreshToken(ctx, client, refreshToken, scopes, timeout)
}

func ensureKeychainAccessIfNeeded(ctx context.Context) error {
	backendInfo, err := resolveKeyringBackendInfo(ctx)
	if err != nil {
		return fmt.Errorf("resolve keyring backend: %w", err)
	}
	if backendInfo.Value == strFile {
		return nil
	}
	if runtime, ok := app.FromContext(ctx); ok && runtime.Auth.EnsureKeychainAccess != nil {
		return runtime.Auth.EnsureKeychainAccess(ctx)
	}
	return secrets.EnsureKeychainAccessContext(ctx)
}

func resolveKeyringBackendInfo(ctx context.Context) (secrets.KeyringBackendInfo, error) {
	store, err := commandConfigStore(ctx)
	if err != nil {
		return secrets.KeyringBackendInfo{}, err
	}
	return secrets.ResolveKeyringBackendInfoFor(store)
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

const (
	authTypeOAuth               = "oauth"
	authTypeServiceAccount      = "service_account"
	authTypeOAuthServiceAccount = "oauth+service_account"
)

type AuthCmd struct {
	Credentials AuthCredentialsCmd    `cmd:"" name:"credentials" help:"Manage OAuth client credentials"`
	Add         AuthAddCmd            `cmd:"" name:"add" help:"Authorize and store a refresh token"`
	Import      AuthImportCmd         `cmd:"" name:"import" help:"Import a refresh token non-interactively from stdin, file, or env"`
	Services    AuthServicesCmd       `cmd:"" name:"services" help:"List supported auth services and scopes"`
	List        AuthListCmd           `cmd:"" name:"list" help:"List stored accounts"`
	Doctor      AuthDoctorCmd         `cmd:"" name:"doctor" help:"Diagnose auth, keyring, and refresh-token issues"`
	Aliases     AuthAliasCmd          `cmd:"" name:"alias" help:"Manage account aliases"`
	Status      AuthStatusCmd         `cmd:"" name:"status" help:"Show auth configuration and keyring backend"`
	Keyring     AuthKeyringCmd        `cmd:"" name:"keyring" help:"Configure keyring backend"`
	Remove      AuthRemoveCmd         `cmd:"" name:"remove" help:"Remove a stored refresh token"`
	Tokens      AuthTokensCmd         `cmd:"" name:"tokens" help:"Manage stored refresh tokens"`
	Manage      AuthManageCmd         `cmd:"" name:"manage" help:"Open accounts manager in browser" aliases:"login"`
	ServiceAcct AuthServiceAccountCmd `cmd:"" name:"service-account" help:"Configure service account (Workspace only; domain-wide delegation)"`
	Keep        AuthKeepCmd           `cmd:"" name:"keep" help:"Configure service account for Google Keep (Workspace only)"`
}

func parseAuthServices(servicesCSV string) ([]googleauth.Service, error) {
	trimmed := strings.ToLower(strings.TrimSpace(servicesCSV))
	if trimmed == "" || trimmed == "user" || trimmed == "all-user" || trimmed == literalAll {
		return googleauth.UserServices(), nil
	}

	parts := strings.Split(servicesCSV, ",")
	seen := make(map[googleauth.Service]struct{})
	out := make([]googleauth.Service, 0, len(parts))
	for _, p := range parts {
		svc, err := googleauth.ParseService(p)
		if err != nil {
			return nil, usage(err.Error())
		}
		switch svc {
		case googleauth.ServiceAdmin, googleauth.ServiceGroups, googleauth.ServiceKeep:
			return nil, usage(fmt.Sprintf("%s auth is Workspace/service-account-only. Use: gog auth service-account set <email> --key <service-account.json>", svc))
		}
		if _, ok := seen[svc]; ok {
			continue
		}
		seen[svc] = struct{}{}
		out = append(out, svc)
	}

	return out, nil
}

func splitCommaList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make([]string, 0)
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\t' || r == ' '
	})
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		out = append(out, f)
	}
	return out
}
