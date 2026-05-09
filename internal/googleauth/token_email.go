package googleauth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// IdentityForRefreshToken exchanges a refresh token and returns the authorized
// Google account identity. Subject is Google's stable OIDC sub claim when
// available; Email is the display/contact address.
func IdentityForRefreshToken(ctx context.Context, client string, refreshToken string, scopes []string, timeout time.Duration) (Identity, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return Identity{}, errMissingToken
	}

	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	creds, err := readClientCredentials(client)
	if err != nil {
		return Identity{}, fmt.Errorf("read credentials: %w", err)
	}

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauthEndpoint,
		Scopes:       scopes,
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: timeout})

	ts := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})

	tok, err := ts.Token()
	if err != nil {
		return Identity{}, fmt.Errorf("refresh access token: %w", err)
	}

	if raw, ok := tok.Extra("id_token").(string); ok {
		if identity, err := IdentityFromIDToken(raw); err == nil {
			return identity, nil
		}
	}

	if strings.TrimSpace(tok.AccessToken) == "" {
		return Identity{}, errMissingAccessToken
	}

	return fetchUserIdentityWithURL(ctx, tok.AccessToken, userinfoURL)
}

// EmailForRefreshToken exchanges a refresh token and returns the authorized email address.
func EmailForRefreshToken(ctx context.Context, client string, refreshToken string, scopes []string, timeout time.Duration) (string, error) {
	identity, err := IdentityForRefreshToken(ctx, client, refreshToken, scopes, timeout)
	if err != nil {
		return "", err
	}

	return identity.Email, nil
}
