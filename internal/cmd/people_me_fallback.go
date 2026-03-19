package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

const googleUserinfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

var fallbackPeopleMeProfile = fetchFallbackPeopleMeProfile

type fallbackProfile struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func fetchFallbackPeopleMeProfile(ctx context.Context, account string) (*people.Person, error) {
	client, err := authclient.ResolveClient(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("resolve client: %w", err)
	}

	creds, err := config.ReadClientCredentialsFor(client)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	store, err := secrets.OpenDefault()
	if err != nil {
		return nil, fmt.Errorf("open secrets store: %w", err)
	}

	tok, err := store.GetToken(client, account)
	if err != nil {
		return nil, fmt.Errorf("get token for %s: %w", account, err)
	}

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     google.Endpoint,
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 15 * time.Second})

	issued, err := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: tok.RefreshToken}).Token()
	if err != nil {
		return nil, fmt.Errorf("refresh access token: %w", err)
	}

	profile := fallbackProfile{}
	if raw, ok := issued.Extra("id_token").(string); ok && strings.TrimSpace(raw) != "" {
		if decoded, err := profileFromIDToken(raw); err == nil {
			profile = decoded
		}
	}

	if strings.TrimSpace(issued.AccessToken) != "" {
		if remote, err := profileFromUserinfo(ctx, issued.AccessToken); err == nil {
			profile = mergeFallbackProfiles(profile, remote)
		}
	}

	return personFromFallbackProfile(profile, account), nil
}

func profileFromIDToken(idToken string) (fallbackProfile, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return fallbackProfile{}, fmt.Errorf("invalid id_token")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fallbackProfile{}, fmt.Errorf("decode id_token payload: %w", err)
	}

	var profile fallbackProfile
	if err := json.Unmarshal(payload, &profile); err != nil {
		return fallbackProfile{}, fmt.Errorf("parse id_token payload: %w", err)
	}
	return profile, nil
}

func profileFromUserinfo(ctx context.Context, accessToken string) (fallbackProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserinfoURL, nil)
	if err != nil {
		return fallbackProfile{}, fmt.Errorf("create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return fallbackProfile{}, fmt.Errorf("fetch userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fallbackProfile{}, fmt.Errorf("userinfo status: %d", resp.StatusCode)
	}

	var profile fallbackProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return fallbackProfile{}, fmt.Errorf("decode userinfo response: %w", err)
	}
	return profile, nil
}

func mergeFallbackProfiles(base fallbackProfile, update fallbackProfile) fallbackProfile {
	if strings.TrimSpace(update.Email) != "" {
		base.Email = update.Email
	}
	if strings.TrimSpace(update.Name) != "" {
		base.Name = update.Name
	}
	if strings.TrimSpace(update.Picture) != "" {
		base.Picture = update.Picture
	}
	return base
}

func personFromFallbackProfile(profile fallbackProfile, account string) *people.Person {
	person := &people.Person{ResourceName: peopleMeResource}

	email := strings.TrimSpace(profile.Email)
	if email == "" {
		email = strings.TrimSpace(account)
	}
	if email != "" {
		person.EmailAddresses = []*people.EmailAddress{{Value: email}}
	}

	if name := strings.TrimSpace(profile.Name); name != "" {
		person.Names = []*people.Name{{DisplayName: name}}
	}

	if picture := strings.TrimSpace(profile.Picture); picture != "" {
		person.Photos = []*people.Photo{{Url: picture}}
	}

	return person
}
