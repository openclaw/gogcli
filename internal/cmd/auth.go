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

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

var (
	openSecretsStore     = secrets.OpenDefault
	authorizeGoogle      = googleauth.Authorize
	startManageServer    = googleauth.StartManageServer
	checkRefreshToken    = googleauth.CheckRefreshToken
	ensureKeychainAccess = secrets.EnsureKeychainAccess
)

type AuthCmd struct {
	Credentials AuthCredentialsCmd `cmd:"" name:"credentials" help:"Store OAuth client credentials"`
	Add         AuthAddCmd         `cmd:"" name:"add" help:"Authorize and store a refresh token"`
	List        AuthListCmd        `cmd:"" name:"list" help:"List stored accounts"`
	Status      AuthStatusCmd      `cmd:"" name:"status" help:"Show auth configuration and keyring backend"`
	Remove      AuthRemoveCmd      `cmd:"" name:"remove" help:"Remove a stored refresh token"`
	Tokens      AuthTokensCmd      `cmd:"" name:"tokens" help:"Manage stored refresh tokens"`
	Manage      AuthManageCmd      `cmd:"" name:"manage" help:"Open accounts manager in browser" aliases:"login"`
}

type AuthCredentialsCmd struct {
	Path string `arg:"" name:"credentials" help:"Path to credentials.json or '-' for stdin"`
}

func (c *AuthCredentialsCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	inPath := c.Path
	var b []byte
	var err error
	if inPath == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(inPath) //nolint:gosec // user-provided path
	}
	if err != nil {
		return err
	}

	creds, err := config.ParseGoogleOAuthClientJSON(b)
	if err != nil {
		return err
	}

	if err := config.WriteClientCredentials(creds); err != nil {
		return err
	}

	outPath, _ := config.ClientCredentialsPath()
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"saved": true,
			"path":  outPath,
		})
	}
	u.Out().Printf("path\t%s", outPath)
	return nil
}

type AuthTokensCmd struct {
	List   AuthTokensListCmd   `cmd:"" name:"list" help:"List stored tokens (by key only)"`
	Delete AuthTokensDeleteCmd `cmd:"" name:"delete" help:"Delete a stored refresh token"`
	Export AuthTokensExportCmd `cmd:"" name:"export" help:"Export a refresh token to a file (contains secrets)"`
	Import AuthTokensImportCmd `cmd:"" name:"import" help:"Import a refresh token file into keyring (contains secrets)"`
}

type AuthTokensListCmd struct{}

func (c *AuthTokensListCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	store, err := openSecretsStore()
	if err != nil {
		return err
	}
	keys, err := store.Keys()
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(os.Stdout, map[string]any{"keys": []string{}})
		}
		u.Err().Println("No tokens stored")
		return nil
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"keys": keys})
	}
	for _, k := range keys {
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
	if err := store.DeleteToken(email); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted": true,
			"email":   email,
		})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("email\t%s", email)
	return nil
}

type AuthTokensExportCmd struct {
	Email     string `arg:"" name:"email" help:"Email"`
	OutPath   string `name:"out" help:"Output file path (required)"`
	Overwrite bool   `name:"overwrite" help:"Overwrite output file if it exists"`
}

func (c *AuthTokensExportCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}
	outPath := strings.TrimSpace(c.OutPath)
	if outPath == "" {
		return usage("empty outPath")
	}

	store, err := openSecretsStore()
	if err != nil {
		return err
	}
	tok, err := store.GetToken(email)
	if err != nil {
		return err
	}

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
		Services     []string `json:"services,omitempty"`
		Scopes       []string `json:"scopes,omitempty"`
		CreatedAt    string   `json:"created_at,omitempty"`
		RefreshToken string   `json:"refresh_token"`
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
		Services:     tok.Services,
		Scopes:       tok.Scopes,
		CreatedAt:    created,
		RefreshToken: tok.RefreshToken,
	}); encErr != nil {
		return encErr
	}

	u.Err().Println("WARNING: exported file contains a refresh token (keep it safe and delete it when done)")
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"exported": true,
			"email":    tok.Email,
			"path":     outPath,
		})
	}
	u.Out().Printf("exported\ttrue")
	u.Out().Printf("email\t%s", tok.Email)
	u.Out().Printf("path\t%s", outPath)
	return nil
}

type AuthTokensImportCmd struct {
	InPath string `arg:"" name:"inPath" help:"Input path or '-' for stdin"`
}

func (c *AuthTokensImportCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	inPath := c.InPath
	var b []byte
	var err error
	if inPath == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(inPath) //nolint:gosec // user-provided path
	}
	if err != nil {
		return err
	}

	type export struct {
		Email        string   `json:"email"`
		Services     []string `json:"services,omitempty"`
		Scopes       []string `json:"scopes,omitempty"`
		CreatedAt    string   `json:"created_at,omitempty"`
		RefreshToken string   `json:"refresh_token"`
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
	var createdAt time.Time
	if strings.TrimSpace(ex.CreatedAt) != "" {
		parsed, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(ex.CreatedAt))
		if parseErr != nil {
			return parseErr
		}
		createdAt = parsed
	}

	// Pre-flight: ensure keychain is accessible before storing token
	if keychainErr := ensureKeychainAccess(); keychainErr != nil {
		return fmt.Errorf("keychain access: %w", keychainErr)
	}

	store, err := openSecretsStore()
	if err != nil {
		return err
	}

	// Pre-flight: ensure keychain is accessible before storing token
	if err := ensureKeychainAccess(); err != nil {
		return fmt.Errorf("keychain access: %w", err)
	}

	if err := store.SetToken(ex.Email, secrets.Token{
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"imported": true,
			"email":    ex.Email,
		})
	}
	u.Out().Printf("imported\ttrue")
	u.Out().Printf("email\t%s", ex.Email)
	return nil
}

type AuthAddCmd struct {
	Email        string `arg:"" name:"email" help:"Email"`
	Manual       bool   `name:"manual" help:"Browserless auth flow (paste redirect URL)"`
	ForceConsent bool   `name:"force-consent" help:"Force consent screen to obtain a refresh token"`
	ServicesCSV  string `name:"services" help:"Services to authorize: all or comma-separated gmail,calendar,drive,contacts,tasks,sheets,people" default:"all"`
}

func (c *AuthAddCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)

	var services []googleauth.Service
	if strings.EqualFold(strings.TrimSpace(c.ServicesCSV), "") || strings.EqualFold(strings.TrimSpace(c.ServicesCSV), "all") {
		services = googleauth.AllServices()
	} else {
		parts := strings.Split(c.ServicesCSV, ",")
		seen := make(map[googleauth.Service]struct{})
		for _, p := range parts {
			svc, err := googleauth.ParseService(p)
			if err != nil {
				return err
			}
			if _, ok := seen[svc]; ok {
				continue
			}
			seen[svc] = struct{}{}
			services = append(services, svc)
		}
	}
	if len(services) == 0 {
		return fmt.Errorf("no services selected")
	}

	scopes, err := googleauth.ScopesForServices(services)
	if err != nil {
		return err
	}

	// Pre-flight: ensure keychain is accessible before starting OAuth
	if keychainErr := ensureKeychainAccess(); keychainErr != nil {
		return fmt.Errorf("keychain access: %w", keychainErr)
	}

	refreshToken, err := authorizeGoogle(ctx, googleauth.AuthorizeOptions{
		Services:     services,
		Scopes:       scopes,
		Manual:       c.Manual,
		ForceConsent: c.ForceConsent,
	})
	if err != nil {
		return err
	}

	store, err := openSecretsStore()
	if err != nil {
		return err
	}
	serviceNames := make([]string, 0, len(services))
	for _, svc := range services {
		serviceNames = append(serviceNames, string(svc))
	}
	sort.Strings(serviceNames)

	if err := store.SetToken(c.Email, secrets.Token{
		Email:        c.Email,
		Services:     serviceNames,
		Scopes:       scopes,
		RefreshToken: refreshToken,
	}); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"stored":   true,
			"email":    c.Email,
			"services": serviceNames,
		})
	}
	u.Out().Printf("email\t%s", c.Email)
	u.Out().Printf("services\t%s", strings.Join(serviceNames, ","))
	return nil
}

type AuthListCmd struct {
	Check   bool          `name:"check" help:"Verify refresh tokens by exchanging for an access token (requires credentials.json)"`
	Timeout time.Duration `name:"timeout" help:"Per-token check timeout" default:"15s"`
}

type AuthStatusCmd struct{}

func (c *AuthStatusCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	configPath, err := config.ConfigPath()
	if err != nil {
		return err
	}
	configExists, err := config.ConfigExists()
	if err != nil {
		return err
	}
	backendInfo, err := secrets.ResolveKeyringBackendInfo()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"config": map[string]any{
				"path":   configPath,
				"exists": configExists,
			},
			"keyring": map[string]any{
				"backend": backendInfo.Value,
				"source":  backendInfo.Source,
			},
		})
	}
	u.Out().Printf("config_path\t%s", configPath)
	u.Out().Printf("config_exists\t%t", configExists)
	u.Out().Printf("keyring_backend\t%s", backendInfo.Value)
	u.Out().Printf("keyring_backend_source\t%s", backendInfo.Source)
	return nil
}

func (c *AuthListCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	store, err := openSecretsStore()
	if err != nil {
		return err
	}
	tokens, err := store.ListTokens()
	if err != nil {
		return err
	}
	sort.Slice(tokens, func(i, j int) bool { return tokens[i].Email < tokens[j].Email })
	if outfmt.IsJSON(ctx) {
		type item struct {
			Email     string   `json:"email"`
			Services  []string `json:"services,omitempty"`
			Scopes    []string `json:"scopes,omitempty"`
			CreatedAt string   `json:"created_at,omitempty"`
			Valid     *bool    `json:"valid,omitempty"`
			Error     string   `json:"error,omitempty"`
		}
		out := make([]item, 0, len(tokens))
		for _, t := range tokens {
			created := ""
			if !t.CreatedAt.IsZero() {
				created = t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			}
			it := item{
				Email:     t.Email,
				Services:  t.Services,
				Scopes:    t.Scopes,
				CreatedAt: created,
			}
			if c.Check {
				err := checkRefreshToken(ctx, t.RefreshToken, t.Scopes, c.Timeout)
				valid := err == nil
				it.Valid = &valid
				if err != nil {
					it.Error = err.Error()
				}
			}
			out = append(out, it)
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{"accounts": out})
	}
	if len(tokens) == 0 {
		u.Err().Println("No tokens stored")
		return nil
	}
	for _, t := range tokens {
		created := ""
		if !t.CreatedAt.IsZero() {
			created = t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		if c.Check {
			err := checkRefreshToken(ctx, t.RefreshToken, t.Scopes, c.Timeout)
			valid := err == nil
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			u.Out().Printf("%s\t%s\t%s\t%t\t%s", t.Email, strings.Join(t.Services, ","), created, valid, msg)
			continue
		}
		u.Out().Printf("%s\t%s\t%s", t.Email, strings.Join(t.Services, ","), created)
	}
	return nil
}

type AuthRemoveCmd struct {
	Email string `arg:"" name:"email" help:"Email"`
}

func (c *AuthRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove stored token for %s", email)); err != nil {
		return err
	}
	store, err := openSecretsStore()
	if err != nil {
		return err
	}
	if err := store.DeleteToken(email); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted": true,
			"email":   email,
		})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("email\t%s", email)
	return nil
}

type AuthManageCmd struct {
	ForceConsent bool          `name:"force-consent" help:"Force consent screen when adding accounts"`
	ServicesCSV  string        `name:"services" help:"Services to authorize: all or comma-separated gmail,calendar,drive,contacts,tasks,sheets,people" default:"all"`
	Timeout      time.Duration `name:"timeout" help:"Server timeout duration" default:"10m"`
}

func (c *AuthManageCmd) Run(ctx context.Context) error {
	var services []googleauth.Service
	if strings.EqualFold(strings.TrimSpace(c.ServicesCSV), "") || strings.EqualFold(strings.TrimSpace(c.ServicesCSV), "all") {
		services = googleauth.AllServices()
	} else {
		parts := strings.Split(c.ServicesCSV, ",")
		seen := make(map[googleauth.Service]struct{})
		for _, p := range parts {
			svc, err := googleauth.ParseService(p)
			if err != nil {
				return err
			}
			if _, ok := seen[svc]; ok {
				continue
			}
			seen[svc] = struct{}{}
			services = append(services, svc)
		}
	}

	return startManageServer(ctx, googleauth.ManageServerOptions{
		Timeout:      c.Timeout,
		Services:     services,
		ForceConsent: c.ForceConsent,
	})
}
