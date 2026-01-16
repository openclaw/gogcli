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
	fetchAuthorizedEmail = googleauth.EmailForRefreshToken
)

func ensureKeychainAccessIfNeeded() error {
	backendInfo, err := secrets.ResolveKeyringBackendInfo()
	if err != nil {
		return fmt.Errorf("resolve keyring backend: %w", err)
	}
	if backendInfo.Value == strFile {
		return nil
	}
	return ensureKeychainAccess()
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

const (
	authTypeOAuth               = "oauth"
	authTypeServiceAccount      = "service_account"
	authTypeOAuthServiceAccount = "oauth+service_account"
)

type AuthCmd struct {
	Credentials AuthCredentialsCmd    `cmd:"" name:"credentials" help:"Store OAuth client credentials"`
	Add         AuthAddCmd            `cmd:"" name:"add" help:"Authorize and store a refresh token"`
	Services    AuthServicesCmd       `cmd:"" name:"services" help:"List supported auth services and scopes"`
	List        AuthListCmd           `cmd:"" name:"list" help:"List stored accounts"`
	Aliases     AuthAliasCmd          `cmd:"" name:"alias" help:"Manage account aliases"`
	Status      AuthStatusCmd         `cmd:"" name:"status" help:"Show auth configuration and keyring backend"`
	Keyring     AuthKeyringCmd        `cmd:"" name:"keyring" help:"Configure keyring backend"`
	Remove      AuthRemoveCmd         `cmd:"" name:"remove" help:"Remove a stored refresh token"`
	Tokens      AuthTokensCmd         `cmd:"" name:"tokens" help:"Manage stored refresh tokens"`
	Manage      AuthManageCmd         `cmd:"" name:"manage" help:"Open accounts manager in browser" aliases:"login"`
	ServiceAcct AuthServiceAccountCmd `cmd:"" name:"service-account" help:"Configure service account (Workspace only; domain-wide delegation)"`
	Keep        AuthKeepCmd           `cmd:"" name:"keep" help:"Configure service account for Google Keep (Workspace only)"`
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
		inPath, err = config.ExpandPath(inPath)
		if err != nil {
			return err
		}
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
	filtered := make([]string, 0, len(keys))
	for _, k := range keys {
		if _, ok := secrets.ParseTokenKey(k); ok {
			filtered = append(filtered, k)
		}
	}

	if len(filtered) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(os.Stdout, map[string]any{"keys": []string{}})
		}
		u.Err().Println("No tokens stored")
		return nil
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"keys": filtered})
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
	Email     string                 `arg:"" name:"email" help:"Email"`
	Output    OutputPathRequiredFlag `embed:""`
	Overwrite bool                   `name:"overwrite" help:"Overwrite output file if it exists"`
}

func (c *AuthTokensExportCmd) Run(ctx context.Context) error {
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
	if keychainErr := ensureKeychainAccessIfNeeded(); keychainErr != nil {
		return fmt.Errorf("keychain access: %w", keychainErr)
	}

	store, err := openSecretsStore()
	if err != nil {
		return err
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
	ServicesCSV  string `name:"services" help:"Services to authorize: user|all or comma-separated ${auth_services} (Keep uses service account: gog auth service-account set)" default:"user"`
	Readonly     bool   `name:"readonly" help:"Use read-only scopes where available (still includes OIDC identity scopes)"`
	DriveScope   string `name:"drive-scope" help:"Drive scope mode: full|readonly|file" enum:"full,readonly,file" default:"full"`
}

func (c *AuthAddCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)

	services, err := parseAuthServices(c.ServicesCSV)
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return fmt.Errorf("no services selected")
	}

	if c.Readonly && c.DriveScope == strFile {
		return usage("cannot combine --readonly with --drive-scope=file (file is write-capable)")
	}
	scopes, err := googleauth.ScopesForManageWithOptions(services, googleauth.ScopeOptions{
		Readonly:   c.Readonly,
		DriveScope: googleauth.DriveScopeMode(c.DriveScope),
	})
	if err != nil {
		return err
	}

	// Pre-flight: ensure keychain is accessible before starting OAuth
	if keychainErr := ensureKeychainAccessIfNeeded(); keychainErr != nil {
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

	authorizedEmail, err := fetchAuthorizedEmail(ctx, refreshToken, scopes, 15*time.Second)
	if err != nil {
		return fmt.Errorf("fetch authorized email: %w", err)
	}
	if normalizeEmail(authorizedEmail) != normalizeEmail(c.Email) {
		return fmt.Errorf("authorized as %s, expected %s", authorizedEmail, c.Email)
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

	if err := store.SetToken(authorizedEmail, secrets.Token{
		Email:        authorizedEmail,
		Services:     serviceNames,
		Scopes:       scopes,
		RefreshToken: refreshToken,
	}); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"stored":   true,
			"email":    authorizedEmail,
			"services": serviceNames,
		})
	}
	u.Out().Printf("email\t%s", authorizedEmail)
	u.Out().Printf("services\t%s", strings.Join(serviceNames, ","))
	return nil
}

type AuthListCmd struct {
	Check   bool          `name:"check" help:"Verify refresh tokens by exchanging for an access token (requires credentials.json)"`
	Timeout time.Duration `name:"timeout" help:"Per-token check timeout" default:"15s"`
}

type AuthStatusCmd struct{}

func (c *AuthStatusCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	account := ""
	authPreferred := ""
	serviceAccountConfigured := false
	serviceAccountPath := ""

	if flags != nil {
		if a, err := requireAccount(flags); err == nil {
			account = a
			if p, _, ok := bestServiceAccountPathAndMtime(normalizeEmail(account)); ok {
				serviceAccountConfigured = true
				serviceAccountPath = p
			}
			if serviceAccountConfigured {
				authPreferred = authTypeServiceAccount
			} else {
				authPreferred = authTypeOAuth
			}
		}
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
			"account": map[string]any{
				"email":                      account,
				"auth_preferred":             authPreferred,
				"service_account_configured": serviceAccountConfigured,
				"service_account_path":       serviceAccountPath,
			},
		})
	}
	u.Out().Printf("config_path\t%s", configPath)
	u.Out().Printf("config_exists\t%t", configExists)
	u.Out().Printf("keyring_backend\t%s", backendInfo.Value)
	u.Out().Printf("keyring_backend_source\t%s", backendInfo.Source)
	if account != "" {
		u.Out().Printf("account\t%s", account)
		u.Out().Printf("auth_preferred\t%s", authPreferred)
		u.Out().Printf("service_account_configured\t%t", serviceAccountConfigured)
		if serviceAccountPath != "" {
			u.Out().Printf("service_account_path\t%s", serviceAccountPath)
		}
	}
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

	serviceAccountEmails, err := config.ListServiceAccountEmails()
	if err != nil {
		return err
	}

	sort.Slice(tokens, func(i, j int) bool { return tokens[i].Email < tokens[j].Email })

	type tokenByEmail struct {
		tok secrets.Token
		ok  bool
	}
	tokMap := make(map[string]tokenByEmail, len(tokens))
	for _, t := range tokens {
		email := normalizeEmail(t.Email)
		if email == "" {
			continue
		}
		tokMap[email] = tokenByEmail{tok: t, ok: true}
	}

	type entry struct {
		Email string
		Token *secrets.Token
		SA    bool
	}
	entries := make([]entry, 0, len(tokens)+len(serviceAccountEmails))
	seen := make(map[string]struct{})
	for _, email := range serviceAccountEmails {
		email = normalizeEmail(email)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		te := tokMap[email]
		var tok *secrets.Token
		if te.ok {
			t := te.tok
			tok = &t
		}
		entries = append(entries, entry{Email: email, Token: tok, SA: true})
	}
	for _, t := range tokens {
		email := normalizeEmail(t.Email)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		t2 := t
		entries = append(entries, entry{Email: email, Token: &t2, SA: false})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Email < entries[j].Email })

	if outfmt.IsJSON(ctx) {
		type item struct {
			Email     string   `json:"email"`
			Services  []string `json:"services,omitempty"`
			Scopes    []string `json:"scopes,omitempty"`
			CreatedAt string   `json:"created_at,omitempty"`
			Auth      string   `json:"auth"`
			Valid     *bool    `json:"valid,omitempty"`
			Error     string   `json:"error,omitempty"`
		}
		out := make([]item, 0, len(entries))
		for _, e := range entries {
			auth := authTypeOAuth
			if e.SA {
				auth = authTypeServiceAccount
			}
			if e.Token != nil && e.SA {
				auth = authTypeOAuthServiceAccount
			}

			created := ""
			services := []string(nil)
			scopes := []string(nil)

			if e.Token != nil {
				if !e.Token.CreatedAt.IsZero() {
					created = e.Token.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
				}
				services = e.Token.Services
				scopes = e.Token.Scopes
			} else if e.SA {
				if p, mtime, ok := bestServiceAccountPathAndMtime(e.Email); ok {
					_ = p
					created = mtime.UTC().Format("2006-01-02T15:04:05Z07:00")
				}
				services = []string{"service-account"}
			}

			it := item{
				Email:     e.Email,
				Services:  services,
				Scopes:    scopes,
				CreatedAt: created,
				Auth:      auth,
			}
			if c.Check {
				if e.Token == nil {
					valid := true
					it.Valid = &valid
					it.Error = "service account (not checked)"
				} else {
					err := checkRefreshToken(ctx, e.Token.RefreshToken, e.Token.Scopes, c.Timeout)
					valid := err == nil
					it.Valid = &valid
					if err != nil {
						it.Error = err.Error()
					}
				}
			}
			out = append(out, it)
		}
		return outfmt.WriteJSON(os.Stdout, map[string]any{"accounts": out})
	}
	if len(entries) == 0 {
		u.Err().Println("No tokens stored")
		return nil
	}

	for _, e := range entries {
		auth := authTypeOAuth
		if e.SA {
			auth = authTypeServiceAccount
		}
		if e.Token != nil && e.SA {
			auth = authTypeOAuthServiceAccount
		}

		created := ""
		servicesCSV := ""

		if e.Token != nil {
			if !e.Token.CreatedAt.IsZero() {
				created = e.Token.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			}
			servicesCSV = strings.Join(e.Token.Services, ",")
		} else if e.SA {
			if _, mtime, ok := bestServiceAccountPathAndMtime(e.Email); ok {
				created = mtime.UTC().Format("2006-01-02T15:04:05Z07:00")
			}
			servicesCSV = "service-account"
		}

		if c.Check {
			if e.Token == nil {
				u.Out().Printf("%s\t%s\t%s\t%t\t%s\t%s", e.Email, servicesCSV, created, true, "service account (not checked)", auth)
				continue
			}

			err := checkRefreshToken(ctx, e.Token.RefreshToken, e.Token.Scopes, c.Timeout)
			valid := err == nil
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			u.Out().Printf("%s\t%s\t%s\t%t\t%s\t%s", e.Email, servicesCSV, created, valid, msg, auth)
			continue
		}

		u.Out().Printf("%s\t%s\t%s\t%s", e.Email, servicesCSV, created, auth)
	}
	return nil
}

func bestServiceAccountPathAndMtime(email string) (string, time.Time, bool) {
	if p, err := config.ServiceAccountPath(email); err == nil {
		if st, err := os.Stat(p); err == nil {
			return p, st.ModTime(), true
		}
	}
	if p, err := config.KeepServiceAccountPath(email); err == nil {
		if st, err := os.Stat(p); err == nil {
			return p, st.ModTime(), true
		}
	}
	if p, err := config.KeepServiceAccountLegacyPath(email); err == nil {
		if st, err := os.Stat(p); err == nil {
			return p, st.ModTime(), true
		}
	}
	return "", time.Time{}, false
}

type AuthServicesCmd struct {
	Markdown bool `name:"markdown" help:"Output Markdown table"`
}

func (c *AuthServicesCmd) Run(ctx context.Context) error {
	infos := googleauth.ServicesInfo()
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"services": infos})
	}
	if c.Markdown {
		_, err := io.WriteString(os.Stdout, googleauth.ServicesMarkdown(infos))
		return err
	}

	w, done := tableWriter(ctx)
	defer done()

	_, _ = fmt.Fprintln(w, "SERVICE\tUSER\tAPIS\tSCOPES\tNOTE")
	for _, info := range infos {
		_, _ = fmt.Fprintf(
			w,
			"%s\t%t\t%s\t%s\t%s\n",
			info.Service,
			info.User,
			strings.Join(info.APIs, ", "),
			strings.Join(info.Scopes, ", "),
			info.Note,
		)
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
	ServicesCSV  string        `name:"services" help:"Services to authorize: user|all or comma-separated ${auth_services} (Keep uses service account: gog auth service-account set)" default:"user"`
	Timeout      time.Duration `name:"timeout" help:"Server timeout duration" default:"10m"`
}

func (c *AuthManageCmd) Run(ctx context.Context) error {
	services, err := parseAuthServices(c.ServicesCSV)
	if err != nil {
		return err
	}

	return startManageServer(ctx, googleauth.ManageServerOptions{
		Timeout:      c.Timeout,
		Services:     services,
		ForceConsent: c.ForceConsent,
	})
}

type AuthKeepCmd struct {
	Email string `arg:"" name:"email" help:"Email to impersonate when using Keep"`
	Key   string `name:"key" required:"" help:"Path to service account JSON key file"`
}

func (c *AuthKeepCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	keyPath := strings.TrimSpace(c.Key)
	if keyPath == "" {
		return usage("empty key path")
	}
	keyPath, err := config.ExpandPath(keyPath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(keyPath) //nolint:gosec // user-provided path
	if err != nil {
		return fmt.Errorf("read service account key: %w", err)
	}

	if _, parseErr := parseServiceAccountJSON(data); parseErr != nil {
		return parseErr
	}

	destPath, err := config.KeepServiceAccountPath(email)
	if err != nil {
		return err
	}
	genericPath, err := config.ServiceAccountPath(email)
	if err != nil {
		return err
	}

	if _, err := config.EnsureDir(); err != nil {
		return err
	}

	if err := os.WriteFile(destPath, data, 0o600); err != nil {
		return fmt.Errorf("write service account: %w", err)
	}
	if err := os.WriteFile(genericPath, data, 0o600); err != nil {
		return fmt.Errorf("write service account: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"stored": true,
			"email":  email,
			"path":   destPath,
			"paths":  []string{destPath, genericPath},
		})
	}
	u.Out().Printf("email\t%s", email)
	u.Out().Printf("path\t%s", destPath)
	u.Out().Println("Keep service account configured. Use: gog keep list --account " + email)
	return nil
}

func parseAuthServices(servicesCSV string) ([]googleauth.Service, error) {
	trimmed := strings.ToLower(strings.TrimSpace(servicesCSV))
	if trimmed == "" || trimmed == "user" || trimmed == "all" {
		return googleauth.UserServices(), nil
	}

	parts := strings.Split(servicesCSV, ",")
	seen := make(map[googleauth.Service]struct{})
	out := make([]googleauth.Service, 0, len(parts))
	for _, p := range parts {
		svc, err := googleauth.ParseService(p)
		if err != nil {
			return nil, err
		}
		if svc == googleauth.ServiceKeep {
			return nil, usage("Keep auth is Workspace-only and requires a service account. Use: gog auth service-account set <email> --key <service-account.json>")
		}
		if _, ok := seen[svc]; ok {
			continue
		}
		seen[svc] = struct{}{}
		out = append(out, svc)
	}

	return out, nil
}
