package googleapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/99designs/keyring"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

const defaultHTTPTimeout = 30 * time.Second

var (
	readClientCredentials = config.ReadClientCredentialsFor
	openSecretsStore      = secrets.OpenDefault
)

// buildTLSConfig creates a TLS config that loads CA certificates from:
// 1. System cert pool
// 2. Linux system CA bundles (fallback for CGO_ENABLED=0 builds)
// 3. SSL_CERT_FILE environment variable (if set) - standard env var for custom CA certs
// This enables gogcli to work through MITM proxies (e.g., mitmproxy, corporate proxies).
func buildTLSConfig() *tls.Config {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		slog.Debug("failed to load system cert pool, creating empty pool", "error", err)
		pool = x509.NewCertPool()
	}

	// On Linux, SystemCertPool may not include all system certs (especially with CGO_ENABLED=0).
	// Explicitly load the system CA bundle as a fallback.
	for _, bundlePath := range []string{
		"/etc/ssl/certs/ca-certificates.crt", // Debian/Ubuntu
		"/etc/pki/tls/certs/ca-bundle.crt",   // RHEL/CentOS
		"/etc/ssl/ca-bundle.pem",             // OpenSUSE
	} {
		//nolint:gosec // G304: These are well-known system CA bundle paths
		if certs, err := os.ReadFile(bundlePath); err == nil {
			pool.AppendCertsFromPEM(certs)
			slog.Debug("loaded system CA bundle", "path", bundlePath)

			break
		}
	}

	// Load from SSL_CERT_FILE if set (standard env var for custom CA certs)
	if certFile := os.Getenv("SSL_CERT_FILE"); certFile != "" {
		//nolint:gosec // G304: SSL_CERT_FILE is a standard env var users set intentionally
		if certs, err := os.ReadFile(certFile); err != nil {
			slog.Debug("failed to read SSL_CERT_FILE", "path", certFile, "error", err)
		} else if pool.AppendCertsFromPEM(certs) {
			slog.Debug("loaded custom CA certs from SSL_CERT_FILE", "path", certFile)
		}
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    pool,
	}
}

func tokenSourceForAccount(ctx context.Context, service googleauth.Service, email string) (oauth2.TokenSource, error) {
	client, err := authclient.ResolveClient(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("resolve client: %w", err)
	}

	creds, err := readClientCredentials(client)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	var requiredScopes []string

	if scopes, err := googleauth.Scopes(service); err != nil {
		return nil, fmt.Errorf("resolve scopes: %w", err)
	} else {
		requiredScopes = scopes
	}

	return tokenSourceForAccountScopes(ctx, string(service), email, client, creds.ClientID, creds.ClientSecret, requiredScopes)
}

func tokenSourceForAccountScopes(ctx context.Context, serviceLabel string, email string, client string, clientID string, clientSecret string, requiredScopes []string) (oauth2.TokenSource, error) {
	var store secrets.Store

	if s, err := openSecretsStore(); err != nil {
		return nil, fmt.Errorf("open secrets store: %w", err)
	} else {
		store = s
	}

	var tok secrets.Token

	if t, err := store.GetToken(client, email); err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return nil, &AuthRequiredError{Service: serviceLabel, Email: email, Client: client, Cause: err}
		}

		return nil, fmt.Errorf("get token for %s: %w", email, err)
	} else {
		tok = t
	}

	cfg := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       requiredScopes,
	}

	// Ensure refresh-token exchanges don't hang forever, respect custom CA certs, and use proxy.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: buildTLSConfig(),
			Proxy:           http.ProxyFromEnvironment,
		},
		Timeout: defaultHTTPTimeout,
	})

	return cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: tok.RefreshToken}), nil
}

func optionsForAccount(ctx context.Context, service googleauth.Service, email string) ([]option.ClientOption, error) {
	scopes, err := googleauth.Scopes(service)
	if err != nil {
		return nil, fmt.Errorf("resolve scopes: %w", err)
	}

	return optionsForAccountScopes(ctx, string(service), email, scopes)
}

func optionsForAccountScopes(ctx context.Context, serviceLabel string, email string, scopes []string) ([]option.ClientOption, error) {
	slog.Debug("creating client options with custom scopes", "serviceLabel", serviceLabel, "email", email)

	var ts oauth2.TokenSource

	// Check for direct access token first (bypasses refresh token flow)
	if accessToken := authclient.AccessTokenFromContext(ctx); accessToken != "" {
		slog.Debug("using direct access token", "serviceLabel", serviceLabel)
		ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	} else if serviceAccountTS, saPath, ok, err := tokenSourceForServiceAccountScopes(ctx, email, scopes); err != nil {
		return nil, fmt.Errorf("service account token source: %w", err)
	} else if ok {
		slog.Debug("using service account credentials", "email", email, "path", saPath)
		ts = serviceAccountTS
	} else {
		client, err := authclient.ResolveClient(ctx, email)
		if err != nil {
			return nil, fmt.Errorf("resolve client: %w", err)
		}

		creds, err := readClientCredentials(client)
		if err != nil {
			return nil, fmt.Errorf("read credentials: %w", err)
		}

		if tokenSource, err := tokenSourceForAccountScopes(ctx, serviceLabel, email, client, creds.ClientID, creds.ClientSecret, scopes); err != nil {
			return nil, fmt.Errorf("token source: %w", err)
		} else {
			ts = tokenSource
		}
	}
	baseTransport := newBaseTransport()
	// Wrap with retry logic for 429 and 5xx errors
	retryTransport := NewRetryTransport(&oauth2.Transport{
		Source: ts,
		Base:   baseTransport,
	})
	c := &http.Client{
		Transport: retryTransport,
		Timeout:   defaultHTTPTimeout,
	}

	slog.Debug("client options with custom scopes created successfully", "serviceLabel", serviceLabel, "email", email)

	return []option.ClientOption{option.WithHTTPClient(c)}, nil
}

func newBaseTransport() *http.Transport {
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || defaultTransport == nil {
		return &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}
	}

	// Clone() deep-copies TLSClientConfig, so no additional clone needed.
	transport := defaultTransport.Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		return transport
	}

	if transport.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		transport.TLSClientConfig.MinVersion = tls.VersionTLS12
	}

	return transport
}
