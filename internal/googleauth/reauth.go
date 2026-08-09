package googleauth

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/openclaw/gogcli/internal/secrets"
)

// ReauthOptions configures an automatic re-authorization flow triggered
// when the stored refresh token is expired or revoked.
type ReauthOptions struct {
	Email    string
	Client   string
	Services []string
	Scopes   []string
	// StoredToken, if non-nil, provides the full scope/service set from the
	// original authorization. When present, its Scopes and Services are used
	// for the re-authorization instead of the triggering request's narrower
	// scopes, preventing silent grant narrowing.
	StoredToken *secrets.Token
	// OpenSecretsStore opens the token store for persisting the new token.
	OpenSecretsStore func() (secrets.Store, error)
	// EnsureKeychainAccess ensures the keychain is accessible for writes.
	// May be nil if keychain access is not needed (e.g. file backend).
	EnsureKeychainAccess func(context.Context) error
	// AuthorizeFunc performs the OAuth authorization flow. If nil,
	// googleauth.Authorize is used.
	AuthorizeFunc func(context.Context, AuthorizeOptions) (string, error)
	// FetchIdentityFunc fetches the authorized identity. If nil,
	// googleauth.IdentityForRefreshToken is used.
	FetchIdentityFunc func(context.Context, string, string, []string, time.Duration) (Identity, error)
	// Timeout for the browser flow. If zero, defaults to 2 minutes.
	Timeout time.Duration
	// Stderr is where progress messages are written. If nil, os.Stderr is used.
	Stderr interface {
		Write([]byte) (int, error)
	}
}

// Reauth performs an automatic re-authorization when the stored refresh
// token is expired or revoked (invalid_grant). It launches a browser-based
// OAuth flow using the same client, services, and scopes as the original
// authorization, then persists the new refresh token to the secret store.
//
// It returns the new refresh token so the caller can update any in-memory
// token source that still holds the revoked token.
//
// This is the auto-reauth counterpart to `gog auth add`, designed to be
// called from the retry transport when an invalid_grant error is detected
// during an API call.
func Reauth(ctx context.Context, opts ReauthOptions) (string, error) {
	if opts.Email == "" {
		return "", fmt.Errorf("reauth: email is required")
	}
	if opts.Client == "" {
		return "", fmt.Errorf("reauth: client is required")
	}
	if len(opts.Scopes) == 0 {
		return "", fmt.Errorf("reauth: scopes are required")
	}

	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}

	authorizeFn := opts.AuthorizeFunc
	if authorizeFn == nil {
		authorizeFn = Authorize
	}

	fetchIdentityFn := opts.FetchIdentityFunc
	if fetchIdentityFn == nil {
		fetchIdentityFn = IdentityForRefreshToken
	}

	if opts.EnsureKeychainAccess != nil {
		if err := opts.EnsureKeychainAccess(ctx); err != nil {
			return "", fmt.Errorf("reauth: keychain access: %w", err)
		}
	}

	// Determine the scopes and services to request. Prefer the stored
	// token's full scope set to prevent silent grant narrowing (e.g. a
	// calendar command narrowing a gmail+calendar+drive grant).
	reauthScopes := opts.Scopes
	reauthServices := opts.Services
	if opts.StoredToken != nil {
		if len(opts.StoredToken.Scopes) > 0 {
			reauthScopes = opts.StoredToken.Scopes
		}
		if len(opts.StoredToken.Services) > 0 {
			reauthServices = opts.StoredToken.Services
		}
	}

	// Convert service strings to Service types.
	services := make([]Service, 0, len(reauthServices))
	for _, s := range reauthServices {
		svc, err := ParseService(s)
		if err != nil {
			// If we can't parse the service label, use the scopes directly.
			slog.Debug("reauth: could not parse service label, using scopes only", "service", s, "err", err)
			continue
		}
		services = append(services, svc)
	}

	// If we couldn't parse any services, derive them from the scopes.
	if len(services) == 0 {
		services = servicesFromScopes(reauthScopes)
	}

	fmt.Fprintln(stderr, "Refresh token expired or revoked. Re-authorizing…")

	authorizeOpts := AuthorizeOptions{
		Services:     services,
		Scopes:       reauthScopes,
		ForceConsent: true, // Ensure Google returns a new refresh token
		Timeout:      timeout,
		Client:       opts.Client,
	}

	refreshToken, err := authorizeFn(ctx, authorizeOpts)
	if err != nil {
		return "", fmt.Errorf("reauth: authorization failed: %w", err)
	}

	// Fetch the authorized identity to verify the email matches.
	identity, err := fetchIdentityFn(ctx, opts.Client, refreshToken, reauthScopes, 15*time.Second)
	if err != nil {
		return "", fmt.Errorf("reauth: fetch authorized identity: %w", err)
	}

	authorizedEmail := identity.Email
	if authorizedEmail == "" {
		authorizedEmail = opts.Email
	}

	// Verify the authorized account matches the expected email.
	if !strings.EqualFold(strings.TrimSpace(authorizedEmail), strings.TrimSpace(opts.Email)) {
		return "", fmt.Errorf("reauth: authorized as %s, expected %s", authorizedEmail, opts.Email)
	}

	// Persist the new refresh token.
	if opts.OpenSecretsStore == nil {
		return "", fmt.Errorf("reauth: secret store opener is required")
	}

	store, err := opts.OpenSecretsStore()
	if err != nil {
		return "", fmt.Errorf("reauth: open secret store: %w", err)
	}

	serviceNames := make([]string, 0, len(services))
	for _, svc := range services {
		serviceNames = append(serviceNames, string(svc))
	}
	sort.Strings(serviceNames)

	if err := store.SetToken(opts.Client, authorizedEmail, secrets.Token{
		Client:       opts.Client,
		Subject:      identity.Subject,
		Email:        authorizedEmail,
		Services:     serviceNames,
		Scopes:       reauthScopes,
		RefreshToken: refreshToken,
	}); err != nil {
		return "", fmt.Errorf("reauth: persist new refresh token: %w", err)
	}

	fmt.Fprintln(stderr, "Re-authorization successful. Retrying request…")

	return refreshToken, nil
}

// servicesFromScopes attempts to derive the service list from a set of
// OAuth scopes. This is a best-effort fallback when the service label
// is not available or cannot be parsed.
func servicesFromScopes(scopes []string) []Service {
	scopeSet := make(map[string]struct{}, len(scopes))
	for _, s := range scopes {
		scopeSet[s] = struct{}{}
	}

	var matched []Service
	for _, svc := range serviceOrder {
		info, ok := serviceInfoByService[svc]
		if !ok || !info.user {
			continue
		}

		// Check if all scopes for this service are present in the scope set.
		allPresent := true
		for _, svcScope := range info.scopes {
			if _, ok := scopeSet[svcScope]; !ok {
				allPresent = false
				break
			}
		}

		if allPresent {
			matched = append(matched, svc)
		}
	}

	if len(matched) == 0 {
		// Fallback: return empty, Authorize will use the scopes directly.
		return nil
	}

	return matched
}
