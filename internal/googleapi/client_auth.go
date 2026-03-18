package googleapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/99designs/keyring"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

var (
	readClientCredentials = config.ReadClientCredentialsFor
	openSecretsStore      = secrets.OpenDefault
)

type persistingTokenSource struct {
	base   oauth2.TokenSource
	store  secrets.Store
	client string
	email  string

	mu  sync.Mutex
	tok secrets.Token
}

func newPersistingTokenSource(base oauth2.TokenSource, store secrets.Store, client string, email string, tok secrets.Token) oauth2.TokenSource {
	return &persistingTokenSource{
		base:   base,
		store:  store,
		client: client,
		email:  email,
		tok:    tok,
	}
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	t, err := p.base.Token()
	if err != nil {
		return nil, fmt.Errorf("base token source: %w", err)
	}

	refreshToken := strings.TrimSpace(t.RefreshToken)
	if refreshToken == "" {
		return t, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if refreshToken == p.tok.RefreshToken {
		return t, nil
	}

	updated := p.tok
	updated.RefreshToken = refreshToken

	if err := p.store.SetToken(p.client, p.email, updated); err != nil {
		slog.Warn("persist rotated refresh token failed", "email", p.email, "client", p.client, "err", err)
		return t, nil
	}

	p.tok = updated
	slog.Debug("persisted rotated refresh token", "email", p.email, "client", p.client)

	return t, nil
}

func tokenSourceForAccount(ctx context.Context, service googleauth.Service, email string) (oauth2.TokenSource, error) {
	client, creds, err := clientCredentialsForAccount(ctx, email)
	if err != nil {
		return nil, err
	}

	scopes, err := googleauth.Scopes(service)
	if err != nil {
		return nil, fmt.Errorf("resolve scopes: %w", err)
	}

	return tokenSourceForAccountScopes(ctx, string(service), email, client, creds.ClientID, creds.ClientSecret, scopes)
}

func clientCredentialsForAccount(ctx context.Context, email string) (string, config.ClientCredentials, error) {
	client, err := authclient.ResolveClient(ctx, email)
	if err != nil {
		return "", config.ClientCredentials{}, fmt.Errorf("resolve client: %w", err)
	}

	creds, err := readClientCredentials(client)
	if err != nil {
		return "", config.ClientCredentials{}, fmt.Errorf("read credentials: %w", err)
	}

	return client, creds, nil
}

func tokenSourceForAvailableAccountAuth(ctx context.Context, serviceLabel string, email string, scopes []string) (oauth2.TokenSource, error) {
	if accessToken := authclient.AccessTokenFromContext(ctx); accessToken != "" {
		slog.Debug("using direct access token", "serviceLabel", serviceLabel)
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken}), nil
	}

	if serviceAccountTS, saPath, ok, err := tokenSourceForServiceAccountScopes(ctx, serviceLabel, email, scopes); err != nil {
		return nil, fmt.Errorf("service account token source: %w", err)
	} else if ok {
		slog.Debug("using service account credentials", "email", email, "path", saPath)
		return serviceAccountTS, nil
	}

	client, creds, err := clientCredentialsForAccount(ctx, email)
	if err != nil {
		return nil, err
	}

	tokenSource, err := tokenSourceForAccountScopes(ctx, serviceLabel, email, client, creds.ClientID, creds.ClientSecret, scopes)
	if err != nil {
		return nil, fmt.Errorf("token source: %w", err)
	}

	return tokenSource, nil
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

	// Ensure refresh-token exchanges don't hang forever.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: tokenExchangeTimeout})

	baseSource := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: tok.RefreshToken})

	return newPersistingTokenSource(baseSource, store, client, email, tok), nil
}
