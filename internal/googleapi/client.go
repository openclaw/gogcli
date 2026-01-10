package googleapi

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/99designs/keyring"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

const defaultHTTPTimeout = 30 * time.Second

var (
	readClientCredentials = config.ReadClientCredentials
	openSecretsStore      = secrets.OpenDefault
)

func tokenSourceForAccount(ctx context.Context, service googleauth.Service, email string) (oauth2.TokenSource, error) {
	var creds config.ClientCredentials

	if c, err := readClientCredentials(); err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	} else {
		creds = c
	}

	var requiredScopes []string
	// Always request full scopes for the client. The actual permissions are determined by the
	// tokens returned during authentication (which honor the --readonly flag).
	// If we restricted scopes here, write operations would fail with a scope error before
	// even attempting the request, even if the token technically had permission.
	// By requesting full scopes, we let the API decide based on the token's privileges.

	if scopes, err := googleauth.Scopes(service, false); err != nil {
		return nil, fmt.Errorf("resolve scopes: %w", err)
	} else {
		requiredScopes = scopes
	}

	return tokenSourceForAccountScopes(ctx, string(service), email, creds.ClientID, creds.ClientSecret, requiredScopes)
}

func tokenSourceForAccountScopes(ctx context.Context, serviceLabel string, email string, clientID string, clientSecret string, requiredScopes []string) (oauth2.TokenSource, error) {
	var store secrets.Store

	if s, err := openSecretsStore(); err != nil {
		return nil, fmt.Errorf("open secrets store: %w", err)
	} else {
		store = s
	}

	var tok secrets.Token

	if t, err := store.GetToken(email); err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return nil, &AuthRequiredError{Service: serviceLabel, Email: email, Cause: err}
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

	// Ensure refresh-token exchanges don't hang forever.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: defaultHTTPTimeout})

	return cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: tok.RefreshToken}), nil
}

func optionsForAccount(ctx context.Context, service googleauth.Service, email string) ([]option.ClientOption, error) {
	slog.Debug("creating client options", "service", service, "email", email)

	ts, err := tokenSourceForAccount(ctx, service, email)
	if err != nil {
		return nil, fmt.Errorf("token source: %w", err)
	}
	baseTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	// Wrap with retry logic for 429 and 5xx errors
	retryTransport := NewRetryTransport(&oauth2.Transport{
		Source: ts,
		Base:   baseTransport,
	})
	c := &http.Client{
		Transport: retryTransport,
		Timeout:   defaultHTTPTimeout,
	}

	slog.Debug("client options created successfully", "service", service, "email", email)

	return []option.ClientOption{option.WithHTTPClient(c)}, nil
}

func optionsForAccountScopes(ctx context.Context, serviceLabel string, email string, scopes []string) ([]option.ClientOption, error) {
	slog.Debug("creating client options with custom scopes", "serviceLabel", serviceLabel, "email", email)

	var creds config.ClientCredentials

	if c, err := readClientCredentials(); err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	} else {
		creds = c
	}

	var ts oauth2.TokenSource

	if tokenSource, err := tokenSourceForAccountScopes(ctx, serviceLabel, email, creds.ClientID, creds.ClientSecret, scopes); err != nil {
		return nil, fmt.Errorf("token source: %w", err)
	} else {
		ts = tokenSource
	}
	baseTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
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
