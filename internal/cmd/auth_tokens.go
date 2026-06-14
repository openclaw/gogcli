package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

type AuthTokensCmd struct {
	List   AuthTokensListCmd   `cmd:"" name:"list" help:"List stored tokens (by key only)"`
	Delete AuthTokensDeleteCmd `cmd:"" name:"delete" help:"Delete a stored refresh token"`
	Export AuthTokensExportCmd `cmd:"" name:"export" help:"Export a refresh token to a file (contains secrets)"`
	Import AuthTokensImportCmd `cmd:"" name:"import" help:"Import a refresh token file into keyring (contains secrets)"`
}

type AuthTokensListCmd struct{}

func (c *AuthTokensListCmd) Run(ctx context.Context, _ *RootFlags) error {
	u := ui.FromContext(ctx)
	store, err := openAuthSecretsStore(ctx)
	if err != nil {
		return err
	}
	filtered, err := storedTokenKeys(store)
	if err != nil {
		return err
	}

	if len(filtered) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"keys": []string{}})
		}
		u.Err().Println("No tokens stored")
		return nil
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"keys": filtered})
	}
	for _, k := range filtered {
		u.Out().Println(k)
	}
	return nil
}

func storedTokenKeys(store secrets.Store) ([]string, error) {
	keys, err := store.Keys()
	if err != nil {
		return nil, err
	}

	filtered := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		client, email, ok := secrets.ParseTokenKey(key)
		if !ok {
			continue
		}
		tokenKey := secrets.TokenKey(client, email)
		if _, ok := seen[tokenKey]; ok {
			continue
		}
		seen[tokenKey] = struct{}{}
		filtered = append(filtered, tokenKey)
	}
	sort.Strings(filtered)

	return filtered, nil
}

type AuthTokensDeleteCmd struct {
	Email string `arg:"" name:"email" help:"Email"`
}

func (c *AuthTokensDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, "auth.tokens.delete", map[string]any{
		"email": email,
	}, fmt.Sprintf("delete stored token for %s", email)); err != nil {
		return err
	}

	store, err := openAuthSecretsStore(ctx)
	if err != nil {
		return err
	}
	client, err := resolveClientForEmail(ctx, email, flags)
	if err != nil {
		return err
	}
	if err := store.DeleteToken(client, email); err != nil {
		return err
	}
	return writeResult(ctx, u,
		kv("deleted", true),
		kv("email", email),
		kv("client", client),
	)
}

type AuthTokensExportCmd struct {
	Email     string                 `arg:"" name:"email" help:"Email"`
	Output    OutputPathRequiredFlag `embed:""`
	Overwrite bool                   `name:"overwrite" help:"Overwrite output file if it exists"`
}

type tokenNoMigrateGetter interface {
	GetTokenNoMigrate(client string, email string) (secrets.Token, error)
}

func (c *AuthTokensExportCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}
	outPath := strings.TrimSpace(c.Output.Path)
	if outPath == "" {
		return usage("empty outPath")
	}
	outPath, err := config.ExpandPath(outPath)
	if err != nil {
		return err
	}
	if dryRunErr := dryRunExit(ctx, flags, "auth.tokens.export", map[string]any{
		"email":            email,
		"out":              outPath,
		"overwrite":        c.Overwrite,
		"contains_secrets": true,
	}); dryRunErr != nil {
		return dryRunErr
	}

	store, err := openAuthSecretsStore(ctx)
	if err != nil {
		return err
	}
	client, err := resolveClientForEmailWithContext(ctx, email, "")
	if err != nil {
		return err
	}
	tok, err := getTokenForExport(store, client, email)
	if err != nil {
		return err
	}

	f, outPath, openErr := openUserOutputFile(outPath, outputFileOptions{
		Overwrite: c.Overwrite,
		FileMode:  0o600,
		DirMode:   0o700,
	})
	if openErr != nil {
		return openErr
	}
	defer func() { _ = f.Close() }()

	type export struct {
		Email                string   `json:"email"`
		Subject              string   `json:"subject,omitempty"`
		Client               string   `json:"client,omitempty"`
		Services             []string `json:"services,omitempty"`
		Scopes               []string `json:"scopes,omitempty"`
		CreatedAt            string   `json:"created_at,omitempty"`
		RefreshToken         string   `json:"refresh_token"`
		AccessToken          string   `json:"access_token,omitempty"`
		AccessTokenExpiresAt string   `json:"access_token_expires_at,omitempty"`
	}
	created := ""
	if !tok.CreatedAt.IsZero() {
		created = tok.CreatedAt.UTC().Format(time.RFC3339)
	}
	accessExpires := ""
	if !tok.AccessTokenExpiresAt.IsZero() {
		accessExpires = tok.AccessTokenExpiresAt.UTC().Format(time.RFC3339)
	}

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if encErr := enc.Encode(export{ //nolint:gosec // explicit token export writes the requested refresh token payload
		Email:                tok.Email,
		Subject:              tok.Subject,
		Client:               client,
		Services:             tok.Services,
		Scopes:               tok.Scopes,
		CreatedAt:            created,
		RefreshToken:         tok.RefreshToken,
		AccessToken:          tok.AccessToken,
		AccessTokenExpiresAt: accessExpires,
	}); encErr != nil {
		return encErr
	}

	u.Err().Println("WARNING: exported file contains OAuth tokens (keep it safe and delete it when done)")
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"exported": true,
			"email":    tok.Email,
			"client":   client,
			"path":     outPath,
		})
	}
	u.Out().Linef("exported\ttrue")
	u.Out().Linef("email\t%s", tok.Email)
	u.Out().Linef("client\t%s", client)
	u.Out().Linef("path\t%s", outPath)
	return nil
}

func getTokenForExport(store secrets.Store, client string, email string) (secrets.Token, error) {
	if noMigrate, ok := store.(tokenNoMigrateGetter); ok {
		return noMigrate.GetTokenNoMigrate(client, email)
	}

	return store.GetToken(client, email)
}

type AuthTokensImportCmd struct {
	InPath string `arg:"" name:"inPath" help:"Input path or '-' for stdin"`
}

func (c *AuthTokensImportCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	inPath := c.InPath
	var b []byte
	var err error
	if inPath == "-" {
		b, err = io.ReadAll(stdinReader(ctx))
	} else {
		inPath, err = config.ExpandPath(inPath)
		if err != nil {
			return err
		}
		b, err = os.ReadFile(inPath) //nolint:gosec // user-provided path
	}
	if err != nil {
		return err
	}

	type export struct {
		Email                string   `json:"email"`
		Subject              string   `json:"subject,omitempty"`
		Client               string   `json:"client,omitempty"`
		Services             []string `json:"services,omitempty"`
		Scopes               []string `json:"scopes,omitempty"`
		CreatedAt            string   `json:"created_at,omitempty"`
		RefreshToken         string   `json:"refresh_token"`
		AccessToken          string   `json:"access_token,omitempty"`
		AccessTokenExpiresAt string   `json:"access_token_expires_at,omitempty"`
	}
	var ex export
	if unmarshalErr := json.Unmarshal(b, &ex); unmarshalErr != nil {
		return usagef("invalid token JSON: %v", unmarshalErr)
	}
	ex.Email = strings.TrimSpace(ex.Email)
	if ex.Email == "" {
		return usage("missing email in token file")
	}
	if strings.TrimSpace(ex.RefreshToken) == "" {
		return usage("missing refresh_token in token file")
	}

	var createdAt time.Time
	if strings.TrimSpace(ex.CreatedAt) != "" {
		parsed, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(ex.CreatedAt))
		if parseErr != nil {
			return usagef("invalid created_at %q (expected RFC3339)", ex.CreatedAt)
		}
		createdAt = parsed
	}
	var accessTokenExpiresAt time.Time
	if strings.TrimSpace(ex.AccessTokenExpiresAt) != "" {
		parsed, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(ex.AccessTokenExpiresAt))
		if parseErr != nil {
			return usagef("invalid access_token_expires_at %q (expected RFC3339)", ex.AccessTokenExpiresAt)
		}
		accessTokenExpiresAt = parsed
	}

	clientOverride := authclient.ClientOverrideFromContext(ctx)
	if strings.TrimSpace(clientOverride) == "" {
		clientOverride = strings.TrimSpace(ex.Client)
	}
	client, err := resolveClientForEmailWithContext(ctx, ex.Email, clientOverride)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "auth.tokens.import", map[string]any{
		"email":                   ex.Email,
		"client":                  client,
		"subject":                 strings.TrimSpace(ex.Subject),
		"services":                ex.Services,
		"scopes":                  ex.Scopes,
		"created_at":              strings.TrimSpace(ex.CreatedAt),
		"refresh_token":           "provided",
		"access_token_provided":   strings.TrimSpace(ex.AccessToken) != "",
		"access_token_expires_at": strings.TrimSpace(ex.AccessTokenExpiresAt),
	}); dryRunErr != nil {
		return dryRunErr
	}

	if keychainErr := ensureKeychainAccessIfNeeded(ctx); keychainErr != nil {
		return fmt.Errorf("keychain access: %w", keychainErr)
	}

	store, err := openAuthSecretsStore(ctx)
	if err != nil {
		return err
	}

	if err := store.SetToken(client, ex.Email, secrets.Token{
		Client:               client,
		Subject:              strings.TrimSpace(ex.Subject),
		Email:                ex.Email,
		Services:             ex.Services,
		Scopes:               ex.Scopes,
		CreatedAt:            createdAt,
		RefreshToken:         ex.RefreshToken,
		AccessToken:          strings.TrimSpace(ex.AccessToken),
		AccessTokenExpiresAt: accessTokenExpiresAt,
	}); err != nil {
		return err
	}

	u.Err().Println("Imported refresh token into keyring")
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"imported": true,
			"email":    ex.Email,
			"client":   client,
		})
	}
	u.Out().Linef("imported\ttrue")
	u.Out().Linef("email\t%s", ex.Email)
	u.Out().Linef("client\t%s", client)
	return nil
}
