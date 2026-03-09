package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	store, err := openSecretsStore()
	if err != nil {
		return err
	}
	tokens, err := store.ListTokens()
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if strings.TrimSpace(tok.Email) == "" {
			continue
		}
		filtered = append(filtered, secrets.TokenKey(tok.Client, tok.Email))
	}
	sort.Strings(filtered)

	if len(filtered) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"keys": []string{}})
		}
		u.Err().Println("No tokens stored")
		return nil
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"keys": filtered})
	}
	for _, k := range filtered {
		u.Out().Println(k)
	}
	return nil
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

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete stored token for %s", email)); err != nil {
		return err
	}

	store, err := openSecretsStore()
	if err != nil {
		return err
	}
	client, err := resolveClientForEmail(email, flags, "")
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

func (c *AuthTokensExportCmd) Run(ctx context.Context, _ *RootFlags) error {
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

	store, err := openSecretsStore()
	if err != nil {
		return err
	}
	client, err := resolveClientForEmailWithContext(ctx, email, "")
	if err != nil {
		return err
	}
	tok, err := store.GetToken(client, email)
	if err != nil {
		return err
	}

	// #nosec G301 -- destination directory is explicitly chosen by the caller.
	if mkErr := os.MkdirAll(filepath.Dir(outPath), 0o700); mkErr != nil {
		return mkErr
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !c.Overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	f, openErr := os.OpenFile(outPath, flags, 0o600) //nolint:gosec // user-provided path
	if openErr != nil {
		return openErr
	}
	defer func() { _ = f.Close() }()

	type export struct {
		Email        string   `json:"email"`
		Client       string   `json:"client,omitempty"`
		Services     []string `json:"services,omitempty"`
		Scopes       []string `json:"scopes,omitempty"`
		CreatedAt    string   `json:"created_at,omitempty"`
		RefreshToken string   `json:"refresh_token"` //nolint:gosec
	}
	created := ""
	if !tok.CreatedAt.IsZero() {
		created = tok.CreatedAt.UTC().Format(time.RFC3339)
	}

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if encErr := enc.Encode(export{
		Email:        tok.Email,
		Client:       client,
		Services:     tok.Services,
		Scopes:       tok.Scopes,
		CreatedAt:    created,
		RefreshToken: tok.RefreshToken,
	}); encErr != nil {
		return encErr
	}

	u.Err().Println("WARNING: exported file contains a refresh token (keep it safe and delete it when done)")
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"exported": true,
			"email":    tok.Email,
			"client":   client,
			"path":     outPath,
		})
	}
	u.Out().Printf("exported\ttrue")
	u.Out().Printf("email\t%s", tok.Email)
	u.Out().Printf("client\t%s", client)
	u.Out().Printf("path\t%s", outPath)
	return nil
}

type AuthTokensImportCmd struct {
	InPath string `arg:"" name:"inPath" help:"Input path or '-' for stdin"`
}

func (c *AuthTokensImportCmd) Run(ctx context.Context, _ *RootFlags) error {
	u := ui.FromContext(ctx)
	inPath := c.InPath
	var b []byte
	var err error
	if inPath == "-" {
		b, err = io.ReadAll(os.Stdin)
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
		Email        string   `json:"email"`
		Client       string   `json:"client,omitempty"`
		Services     []string `json:"services,omitempty"`
		Scopes       []string `json:"scopes,omitempty"`
		CreatedAt    string   `json:"created_at,omitempty"`
		RefreshToken string   `json:"refresh_token"` //nolint:gosec
	}
	var ex export
	if unmarshalErr := json.Unmarshal(b, &ex); unmarshalErr != nil {
		return unmarshalErr
	}
	ex.Email = strings.TrimSpace(ex.Email)
	if ex.Email == "" {
		return usage("missing email in token file")
	}
	if strings.TrimSpace(ex.RefreshToken) == "" {
		return usage("missing refresh_token in token file")
	}
	clientOverride := authclient.ClientOverrideFromContext(ctx)
	if strings.TrimSpace(clientOverride) == "" {
		clientOverride = strings.TrimSpace(ex.Client)
	}
	client, err := resolveClientForEmailWithContext(ctx, ex.Email, clientOverride)
	if err != nil {
		return err
	}
	var createdAt time.Time
	if strings.TrimSpace(ex.CreatedAt) != "" {
		parsed, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(ex.CreatedAt))
		if parseErr != nil {
			return parseErr
		}
		createdAt = parsed
	}

	if keychainErr := ensureKeychainAccessIfNeeded(); keychainErr != nil {
		return fmt.Errorf("keychain access: %w", keychainErr)
	}

	store, err := openSecretsStore()
	if err != nil {
		return err
	}

	if err := store.SetToken(client, ex.Email, secrets.Token{
		Client:       client,
		Email:        ex.Email,
		Services:     ex.Services,
		Scopes:       ex.Scopes,
		CreatedAt:    createdAt,
		RefreshToken: ex.RefreshToken,
	}); err != nil {
		return err
	}

	u.Err().Println("Imported refresh token into keyring")
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"imported": true,
			"email":    ex.Email,
			"client":   client,
		})
	}
	u.Out().Printf("imported\ttrue")
	u.Out().Printf("email\t%s", ex.Email)
	u.Out().Printf("client\t%s", client)
	return nil
}
