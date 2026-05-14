package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

var readAuthImportStdin = func() ([]byte, error) {
	return io.ReadAll(os.Stdin)
}

type AuthImportCmd struct {
	Email             string `name:"email" required:"" help:"Account email"`
	RefreshToken      string `name:"refresh-token" hidden:"" help:"OAuth refresh token to store (prefer --refresh-token-stdin, --refresh-token-file, or --refresh-token-env)"`
	RefreshTokenStdin bool   `name:"refresh-token-stdin" help:"Read OAuth refresh token from stdin"`
	RefreshTokenFile  string `name:"refresh-token-file" type:"path" help:"Read OAuth refresh token from file"`
	RefreshTokenEnv   string `name:"refresh-token-env" help:"Read OAuth refresh token from the named environment variable"`
	ServicesCSV       string `name:"services" help:"Comma-separated services to record on the token (informational; does not affect scopes)"`
}

func (c *AuthImportCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	email := normalizeEmail(c.Email)
	if email == "" {
		return usage("--email is required")
	}

	refreshToken, tokenErr := c.resolveRefreshToken()
	if tokenErr != nil {
		return tokenErr
	}

	override := ""
	if flags != nil {
		override = flags.Client
	}
	client, clientErr := resolveClientForEmail(email, flags)
	if clientErr != nil {
		return clientErr
	}

	services := splitCommaList(c.ServicesCSV)
	force := flags != nil && flags.Force

	if err := dryRunExit(ctx, flags, "auth.import", map[string]any{
		"client":   client,
		"email":    email,
		"services": services,
		"force":    force,
	}); err != nil {
		return err
	}

	if err := ensureKeychainAccessIfNeeded(); err != nil {
		return fmt.Errorf("keychain access: %w", err)
	}

	store, err := openSecretsStore()
	if err != nil {
		return err
	}

	if _, getErr := store.GetToken(client, email); getErr == nil {
		if !force {
			return usagef("entry already exists for client=%q email=%q (use --force to overwrite)", client, email)
		}
	} else if !errors.Is(getErr, keyring.ErrKeyNotFound) {
		return getErr
	}

	if err := store.SetToken(client, email, secrets.Token{
		Client:       client,
		Email:        email,
		Services:     services,
		RefreshToken: refreshToken,
	}); err != nil {
		return err
	}
	if strings.TrimSpace(override) != "" {
		cfg, err := config.ReadConfig()
		if err != nil {
			return err
		}
		if err := config.SetAccountClient(&cfg, email, client); err != nil {
			return err
		}
		if err := config.WriteConfig(cfg); err != nil {
			return err
		}
	}

	return writeResult(ctx, u,
		kv("imported", true),
		kv("client", client),
		kv("email", email),
	)
}

func (c *AuthImportCmd) resolveRefreshToken() (string, error) {
	sources := 0
	if strings.TrimSpace(c.RefreshToken) != "" {
		sources++
	}
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
		return "", usage("provide refresh token with --refresh-token-stdin, --refresh-token-file, or --refresh-token-env")
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
		raw, err = readAuthImportStdin()
		if err != nil {
			return "", fmt.Errorf("read --refresh-token-stdin: %w", err)
		}
	case strings.TrimSpace(c.RefreshTokenFile) != "":
		raw, err = os.ReadFile(strings.TrimSpace(c.RefreshTokenFile))
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
	default:
		raw = []byte(c.RefreshToken)
	}

	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", usage("refresh token must not be empty")
	}
	return token, nil
}
