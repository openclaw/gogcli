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

type tokenAliasDeleter interface {
	DeleteTokenAlias(client string, email string) error
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

	p.mu.Lock()
	defer p.mu.Unlock()

	updated := p.tok
	changed := false
	emailChangedFromIdentity := false

	if refreshToken != "" && refreshToken != p.tok.RefreshToken {
		updated.RefreshToken = refreshToken
		changed = true
	}

	if rawIDToken, ok := t.Extra("id_token").(string); ok && strings.TrimSpace(rawIDToken) != "" {
		if identity, identityErr := googleauth.IdentityFromIDToken(rawIDToken); identityErr == nil {
			if strings.TrimSpace(identity.Subject) != "" && strings.TrimSpace(identity.Subject) != strings.TrimSpace(updated.Subject) {
				updated.Subject = strings.TrimSpace(identity.Subject)
				changed = true
			}

			if email := strings.TrimSpace(identity.Email); email != "" && !strings.EqualFold(email, updated.Email) {
				updated.Email = email
				changed = true
				emailChangedFromIdentity = true
			}
		}
	}

	if !changed {
		return t, nil
	}

	persistEmail := strings.TrimSpace(p.email)
	if emailChangedFromIdentity || persistEmail == "" {
		persistEmail = strings.TrimSpace(updated.Email)
	}

	if persistEmail == "" {
		persistEmail = p.email
	}

	if err := p.store.SetToken(p.client, persistEmail, updated); err != nil {
		slog.Warn("persist refreshed token metadata failed", "email", persistEmail, "client", p.client, "err", err)
		return t, nil
	}

	if !strings.EqualFold(p.email, persistEmail) {
		aliasDeleter, ok := p.store.(tokenAliasDeleter)
		if !ok {
			slog.Debug("token store cannot delete renamed email alias", "old_email", p.email, "new_email", persistEmail, "client", p.client)
		} else if err := aliasDeleter.DeleteTokenAlias(p.client, p.email); err != nil {
			slog.Warn("delete renamed token alias failed", "old_email", p.email, "new_email", persistEmail, "client", p.client, "err", err)
		}
	}

	p.tok = updated
	p.email = persistEmail
	slog.Debug("persisted refreshed token metadata", "email", persistEmail, "client", p.client)

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
