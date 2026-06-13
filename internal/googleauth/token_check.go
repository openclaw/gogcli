package googleauth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

func CheckRefreshToken(ctx context.Context, client string, refreshToken string, scopes []string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	creds, err := readOAuthClientCredentials(ctx, client)
	if err != nil {
		return fmt.Errorf("read credentials: %w", err)
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
	if _, err := ts.Token(); err != nil {
		return fmt.Errorf("refresh access token: %w", err)
	}

	return nil
}
