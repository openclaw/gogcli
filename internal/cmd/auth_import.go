package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

var authImportNow = time.Now

type AuthImportCmd struct {
	Email                string `name:"email" required:"" help:"Account email"`
	RefreshTokenStdin    bool   `name:"refresh-token-stdin" help:"Read OAuth refresh token from stdin"`
	RefreshTokenFile     string `name:"refresh-token-file" type:"path" help:"Read OAuth refresh token from file"`
	RefreshTokenEnv      string `name:"refresh-token-env" help:"Read OAuth refresh token from the named environment variable"`
	AccessTokenStdin     bool   `name:"access-token-stdin" help:"Also read a current OAuth access token from stdin (requires a refresh-token source)"`
	AccessTokenFile      string `name:"access-token-file" type:"path" help:"Also read a current OAuth access token from file (requires a refresh-token source)"`
	AccessTokenEnv       string `name:"access-token-env" help:"Also read a current OAuth access token from the named environment variable (requires a refresh-token source)"`
	AccessTokenExpiresAt string `name:"access-token-expires-at" help:"Expiry for the optional access token (RFC3339; default: now+1h when provided)"`
	ServicesCSV          string `name:"services" help:"Comma-separated services to record on the token (informational; does not affect scopes)"`
}

func (c *AuthImportCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	email := normalizeEmail(c.Email)
	if email == "" {
		return usage("--email is required")
	}
	if c.RefreshTokenStdin && c.AccessTokenStdin {
		return usage("--access-token-stdin cannot be combined with --refresh-token-stdin")
	}

	refreshToken, tokenErr := c.resolveRefreshToken(ctx)
	if tokenErr != nil {
		return tokenErr
	}
	accessToken, accessTokenExpiresAt, accessErr := c.resolveAccessToken(ctx)
	if accessErr != nil {
		return accessErr
	}

	override := ""
	if flags != nil {
		override = flags.Client
	}
	client, clientErr := resolveClientForEmail(ctx, email, flags)
	if clientErr != nil {
		return clientErr
	}

	services := splitCommaList(c.ServicesCSV)
	force := flags != nil && flags.Force

	if err := dryRunExit(ctx, flags, "auth.import", map[string]any{
		"client":                  client,
		"email":                   email,
		"services":                services,
		"force":                   force,
		"access_token_provided":   accessToken != "",
		"access_token_expires_at": formatOptionalTime(accessTokenExpiresAt),
	}); err != nil {
		return err
	}

	if err := ensureKeychainAccessIfNeeded(ctx); err != nil {
		return fmt.Errorf("keychain access: %w", err)
	}

	store, err := openAuthSecretsStore(ctx)
	if err != nil {
		return err
	}

	if !force {
		if _, getErr := store.GetToken(client, email); getErr == nil {
			return usagef("entry already exists for client=%q email=%q (use --force to overwrite)", client, email)
		} else if !errors.Is(getErr, keyring.ErrKeyNotFound) {
			return getErr
		}
	}

	if err := store.SetToken(client, email, secrets.Token{
		Client:               client,
		Email:                email,
		Services:             services,
		RefreshToken:         refreshToken,
		AccessToken:          accessToken,
		AccessTokenExpiresAt: accessTokenExpiresAt,
	}); err != nil {
		return err
	}
	if strings.TrimSpace(override) != "" {
		configStore, err := commandConfigStore(ctx)
		if err != nil {
			return err
		}
		cfg, err := configStore.Read()
		if err != nil {
			return err
		}
		if err := config.SetAccountClient(&cfg, email, client); err != nil {
			return err
		}
		if err := configStore.Write(cfg); err != nil {
			return err
		}
	}

	return writeResult(ctx, u,
		kv("imported", true),
		kv("client", client),
		kv("email", email),
	)
}

func (c *AuthImportCmd) resolveRefreshToken(ctx context.Context) (string, error) {
	sources := 0
	if c.RefreshTokenStdin {
		sources++
	}
	if strings.TrimSpace(c.RefreshTokenFile) != "" {
		sources++
	}
	if strings.TrimSpace(c.RefreshTokenEnv) != "" {
		sources++
	}
	if sources == 0 {
		return "", usage("provide required refresh token with --refresh-token-stdin, --refresh-token-file, or --refresh-token-env; access-token sources are optional and cannot replace it")
	}
	if sources > 1 {
		return "", usage("provide exactly one refresh token source")
	}

	var (
		raw []byte
		err error
	)
	switch {
	case c.RefreshTokenStdin:
		raw, err = io.ReadAll(stdinReader(ctx))
		if err != nil {
			return "", fmt.Errorf("read --refresh-token-stdin: %w", err)
		}
	case strings.TrimSpace(c.RefreshTokenFile) != "":
		path, expandErr := config.ExpandPath(strings.TrimSpace(c.RefreshTokenFile))
		if expandErr != nil {
			return "", fmt.Errorf("expand --refresh-token-file: %w", expandErr)
		}
		raw, err = os.ReadFile(path) //nolint:gosec // user-provided token file path
		if err != nil {
			return "", fmt.Errorf("read --refresh-token-file: %w", err)
		}
	case strings.TrimSpace(c.RefreshTokenEnv) != "":
		envName := strings.TrimSpace(c.RefreshTokenEnv)
		value, ok := os.LookupEnv(envName)
		if !ok {
			return "", usagef("environment variable %s is not set", envName)
		}
		raw = []byte(value)
	}

	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", usage("refresh token must not be empty")
	}
	return token, nil
}

func (c *AuthImportCmd) resolveAccessToken(ctx context.Context) (string, time.Time, error) {
	sources := 0
	if c.AccessTokenStdin {
		sources++
	}
	if strings.TrimSpace(c.AccessTokenFile) != "" {
		sources++
	}
	if strings.TrimSpace(c.AccessTokenEnv) != "" {
		sources++
	}
	if sources == 0 {
		if strings.TrimSpace(c.AccessTokenExpiresAt) != "" {
			return "", time.Time{}, usage("--access-token-expires-at requires an access token source")
		}
		return "", time.Time{}, nil
	}
	if sources > 1 {
		return "", time.Time{}, usage("provide exactly one access token source")
	}
	var (
		raw []byte
		err error
	)
	switch {
	case c.AccessTokenStdin:
		raw, err = io.ReadAll(stdinReader(ctx))
		if err != nil {
			return "", time.Time{}, fmt.Errorf("read --access-token-stdin: %w", err)
		}
	case strings.TrimSpace(c.AccessTokenFile) != "":
		path, expandErr := config.ExpandPath(strings.TrimSpace(c.AccessTokenFile))
		if expandErr != nil {
			return "", time.Time{}, fmt.Errorf("expand --access-token-file: %w", expandErr)
		}
		raw, err = os.ReadFile(path) //nolint:gosec // user-provided token file path
		if err != nil {
			return "", time.Time{}, fmt.Errorf("read --access-token-file: %w", err)
		}
	case strings.TrimSpace(c.AccessTokenEnv) != "":
		envName := strings.TrimSpace(c.AccessTokenEnv)
		value, ok := os.LookupEnv(envName)
		if !ok {
			return "", time.Time{}, usagef("environment variable %s is not set", envName)
		}
		raw = []byte(value)
	}

	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", time.Time{}, usage("access token must not be empty")
	}

	expiresAt := authImportNow().UTC().Add(time.Hour)
	if strings.TrimSpace(c.AccessTokenExpiresAt) != "" {
		parsed, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(c.AccessTokenExpiresAt))
		if parseErr != nil {
			return "", time.Time{}, fmt.Errorf("parse --access-token-expires-at: %w", parseErr)
		}
		if !parsed.After(authImportNow()) {
			return "", time.Time{}, usage("--access-token-expires-at must be in the future")
		}
		expiresAt = parsed.UTC()
	}

	return token, expiresAt, nil
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
