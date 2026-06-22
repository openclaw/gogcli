package googleapi

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/googleauth"
)

const (
	// responseHeaderTimeout limits the time waiting for the server to begin
	// responding (send response headers). Once headers arrive and the body
	// starts streaming, there is no hard cap — large file downloads are not
	// cut short. This replaces the former http.Client.Timeout which applied
	// to the entire request lifecycle and caused timeouts on large Drive
	// file downloads.
	responseHeaderTimeout = 30 * time.Second

	// tokenExchangeTimeout is applied to the short-lived HTTP client used
	// for OAuth2 token refresh exchanges, which should always be fast.
	tokenExchangeTimeout = 30 * time.Second
)

func optionsForAccount(ctx context.Context, service googleauth.Service, email string) ([]option.ClientOption, error) {
	scopes, err := googleauth.Scopes(service)
	if err != nil {
		return nil, fmt.Errorf("resolve scopes: %w", err)
	}

	return optionsForAccountScopes(ctx, string(service), email, scopes)
}

type googleServiceFactory[T any] func(context.Context, ...option.ClientOption) (*T, error)

func newGoogleServiceForAccount[T any](
	ctx context.Context,
	email string,
	service googleauth.Service,
	label string,
	factory googleServiceFactory[T],
) (*T, error) {
	opts, err := optionsForAccount(ctx, service, email)
	if err != nil {
		return nil, fmt.Errorf("%s options: %w", label, err)
	}

	return newGoogleService(ctx, label, opts, factory)
}

func newGoogleServiceForScopes[T any](
	ctx context.Context,
	email string,
	serviceLabel string,
	errorLabel string,
	scopes []string,
	factory googleServiceFactory[T],
) (*T, error) {
	opts, err := optionsForAccountScopes(ctx, serviceLabel, email, scopes)
	if err != nil {
		return nil, fmt.Errorf("%s options: %w", errorLabel, err)
	}

	return newGoogleService(ctx, errorLabel, opts, factory)
}

func newGoogleServiceForRequiredScopes[T any](
	ctx context.Context,
	email string,
	serviceLabel string,
	errorLabel string,
	scopes []string,
	factory googleServiceFactory[T],
) (*T, error) {
	opts, err := optionsForAccountScopesRequiringStoredGrant(ctx, serviceLabel, email, scopes)
	if err != nil {
		return nil, fmt.Errorf("%s options: %w", errorLabel, err)
	}

	return newGoogleService(ctx, errorLabel, opts, factory)
}

func newGoogleServiceForServiceAccountScopes[T any](
	ctx context.Context,
	email string,
	serviceLabel string,
	errorLabel string,
	scopes []string,
	factory googleServiceFactory[T],
) (*T, error) {
	opts, err := optionsForServiceAccountScopes(ctx, serviceLabel, email, scopes)
	if err != nil {
		return nil, fmt.Errorf("%s options: %w", errorLabel, err)
	}

	return newGoogleService(ctx, errorLabel, opts, factory)
}

func newGoogleService[T any](
	ctx context.Context,
	label string,
	opts []option.ClientOption,
	factory googleServiceFactory[T],
) (*T, error) {
	svc, err := factory(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create %s service: %w", label, err)
	}

	return svc, nil
}

func authenticatedTransport(ctx context.Context, serviceLabel string, email string, scopes []string) (http.RoundTripper, error) {
	return authenticatedTransportWithStoredScopeCheck(ctx, serviceLabel, email, scopes, false)
}

func authenticatedTransportWithStoredScopeCheck(
	ctx context.Context,
	serviceLabel string,
	email string,
	scopes []string,
	requireStoredGrant bool,
) (http.RoundTripper, error) {
	var ts oauth2.TokenSource

	if dependencies, ok := authDependenciesFromContext(ctx); ok && dependencies.Mode == AuthModeADC {
		slog.Debug("using Application Default Credentials (GOG_AUTH_MODE=adc)", "serviceLabel", serviceLabel)

		adcTS, err := dependencies.adcTokenSource(ctx, scopes)
		if err != nil {
			return nil, err
		}

		ts = adcTS
	} else {
		var err error

		ts, err = tokenSourceForAvailableAccountAuthWithStoredScopeCheck(ctx, serviceLabel, email, scopes, requireStoredGrant)
		if err != nil {
			return nil, err
		}
	}

	return readOnlyTransportFromContext(ctx, NewRetryTransport(&oauth2.Transport{
		Source: ts,
		Base:   newBaseTransport(),
	})), nil
}

func optionsForAccountScopes(ctx context.Context, serviceLabel string, email string, scopes []string) ([]option.ClientOption, error) {
	return optionsForAccountScopesWithStoredScopeCheck(ctx, serviceLabel, email, scopes, false)
}

func optionsForAccountScopesRequiringStoredGrant(ctx context.Context, serviceLabel string, email string, scopes []string) ([]option.ClientOption, error) {
	return optionsForAccountScopesWithStoredScopeCheck(ctx, serviceLabel, email, scopes, true)
}

func optionsForServiceAccountScopes(ctx context.Context, serviceLabel string, email string, scopes []string) ([]option.ClientOption, error) {
	if dependencies, ok := authDependenciesFromContext(ctx); ok && dependencies.Mode == AuthModeADC {
		slog.Debug("using Application Default Credentials (GOG_AUTH_MODE=adc)", "serviceLabel", serviceLabel)

		ts, err := dependencies.adcTokenSource(ctx, scopes)
		if err != nil {
			return nil, err
		}

		return tokenSourceClientOptions(ctx, ts), nil
	}

	if accessToken := authclient.AccessTokenFromContext(ctx); accessToken != "" {
		slog.Debug("using direct access token", "serviceLabel", serviceLabel)

		return tokenSourceClientOptions(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})), nil
	}

	dependencies, err := requireAuthDependencies(ctx)
	if err != nil {
		return nil, err
	}

	ts, path, ok, err := tokenSourceForServiceAccountScopes(ctx, dependencies, serviceLabel, email, scopes)
	if err != nil {
		return nil, fmt.Errorf("service account token source: %w", err)
	}

	if !ok {
		return nil, &AuthRequiredError{Service: serviceLabel, Email: email}
	}

	slog.Debug("using required service account credentials", "email", email, "path", path)

	return tokenSourceClientOptions(ctx, ts), nil
}

func tokenSourceClientOptions(ctx context.Context, ts oauth2.TokenSource) []option.ClientOption {
	return []option.ClientOption{option.WithHTTPClient(&http.Client{
		Transport: readOnlyTransportFromContext(ctx, NewRetryTransport(&oauth2.Transport{
			Source: ts,
			Base:   newBaseTransport(),
		})),
	})}
}

func optionsForAccountScopesWithStoredScopeCheck(
	ctx context.Context,
	serviceLabel string,
	email string,
	scopes []string,
	requireStoredGrant bool,
) ([]option.ClientOption, error) {
	slog.Debug("creating client options with custom scopes", "serviceLabel", serviceLabel, "email", email)

	transport, err := authenticatedTransportWithStoredScopeCheck(ctx, serviceLabel, email, scopes, requireStoredGrant)
	if err != nil {
		return nil, err
	}

	c := &http.Client{
		Transport: transport,
		// No Timeout set: large file downloads (Drive videos, etc.) must not
		// be cut short. Server responsiveness is guarded by the transport's
		// ResponseHeaderTimeout instead.
	}

	slog.Debug("client options with custom scopes created successfully", "serviceLabel", serviceLabel, "email", email)

	return []option.ClientOption{option.WithHTTPClient(c)}, nil
}

// NewHTTPClient returns a raw *http.Client authenticated for the given service
// and account. The caller may set CheckRedirect or other policies on the
// returned client.
func NewHTTPClient(ctx context.Context, service googleauth.Service, email string) (*http.Client, error) {
	scopes, err := googleauth.Scopes(service)
	if err != nil {
		return nil, fmt.Errorf("resolve scopes: %w", err)
	}

	transport, err := authenticatedTransport(ctx, string(service), email, scopes)
	if err != nil {
		return nil, err
	}

	return &http.Client{Transport: transport}, nil
}

// NewHTTPClientForScopes returns an authenticated raw client for a
// Discovery-described API method whose scopes are not part of the static
// service registry.
func NewHTTPClientForScopes(ctx context.Context, serviceLabel, email string, scopes []string) (*http.Client, error) {
	transport, err := authenticatedTransport(ctx, serviceLabel, email, scopes)
	if err != nil {
		return nil, err
	}

	return &http.Client{Transport: transport}, nil
}

func newBaseTransport() *http.Transport {
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || defaultTransport == nil {
		return &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			ResponseHeaderTimeout: responseHeaderTimeout,
		}
	}

	// Clone() deep-copies TLSClientConfig, so no additional clone needed.
	transport := defaultTransport.Clone()
	transport.ResponseHeaderTimeout = responseHeaderTimeout

	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		return transport
	}

	if transport.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		transport.TLSClientConfig.MinVersion = tls.VersionTLS12
	}

	return transport
}
