package googleauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

// AccountInfo represents an account for the UI
type AccountInfo struct {
	Email     string   `json:"email"`
	Services  []string `json:"services"`
	IsDefault bool     `json:"isDefault"`
}

type Identity struct {
	Subject string `json:"subject,omitempty"`
	Email   string `json:"email"`
}

type FetchIdentityFunc func(context.Context, *oauth2.Token) (Identity, error)

// ManagerOptions configures the accounts manager application.
type ManagerOptions struct {
	Services     []Service
	ForceConsent bool
	Client       string
	RedirectURI  string
}

// ManagerDependencies contains the accounts manager's external operations.
type ManagerDependencies struct {
	Tokens                secrets.Store
	ReadCredentials       func(client string) (config.ClientCredentials, error)
	UpdateEmailReferences EmailReferenceUpdater
	FetchIdentity         FetchIdentityFunc
	EnsureKeychainAccess  func(context.Context) error
	Random                io.Reader
	OAuthEndpoint         oauth2.Endpoint
}

// ManagerApplication handles the accounts management UI.
type ManagerApplication struct {
	opts         ManagerOptions
	deps         ManagerDependencies
	client       string
	csrfToken    string
	handler      http.Handler
	accountsPage *template.Template
	successPage  *template.Template
	oauthMu      sync.Mutex
	oauthStates  map[string]string
	randomMu     sync.Mutex
}

var (
	errUserinfoRequestFailed = errors.New("userinfo request failed")
	errMissingToken          = errors.New("missing token")
	errMissingAccessToken    = errors.New("missing access token")
	errInvalidIDToken        = errors.New("invalid id_token")
	errNoEmailInIDToken      = errors.New("no email in id_token")
	errNoEmailInResponse     = errors.New("no email in userinfo response")
)

var (
	errManagerTokensRequired       = errors.New("accounts manager token store is required")
	errManagerRedirectURIRequired  = errors.New("accounts manager redirect URI is required")
	errManagerRandomRequired       = errors.New("accounts manager random source is required")
	errManagerIdentityRequired     = errors.New("accounts manager identity fetcher is required")
	errManagerKeychainRequired     = errors.New("accounts manager keychain check is required")
	errManagerOAuthEndpointInvalid = errors.New("accounts manager OAuth endpoint is required")
)

const userinfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

// NewManagerApplication builds the accounts manager HTTP application.
func NewManagerApplication(opts ManagerOptions, deps ManagerDependencies) (*ManagerApplication, error) {
	client, err := config.NormalizeClientNameOrDefault(opts.Client)
	if err != nil {
		return nil, fmt.Errorf("resolve client: %w", err)
	}
	opts.Client = client
	opts.RedirectURI = strings.TrimSpace(opts.RedirectURI)

	switch {
	case deps.Tokens == nil:
		return nil, errManagerTokensRequired
	case deps.ReadCredentials == nil:
		return nil, errCredentialsReaderRequired
	case deps.UpdateEmailReferences == nil:
		return nil, errEmailReferenceUpdaterRequired
	case deps.FetchIdentity == nil:
		return nil, errManagerIdentityRequired
	case deps.EnsureKeychainAccess == nil:
		return nil, errManagerKeychainRequired
	case deps.Random == nil:
		return nil, errManagerRandomRequired
	case deps.OAuthEndpoint.AuthURL == "" || deps.OAuthEndpoint.TokenURL == "":
		return nil, errManagerOAuthEndpointInvalid
	case opts.RedirectURI == "":
		return nil, errManagerRedirectURIRequired
	}

	accountsPage, err := template.New("accounts").Parse(accountsTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse accounts manager template: %w", err)
	}

	successPage, err := template.New("success").Parse(successTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse accounts manager success template: %w", err)
	}

	csrfToken, err := generateRandomHex(deps.Random, 32)
	if err != nil {
		return nil, fmt.Errorf("generate CSRF token: %w", err)
	}

	app := &ManagerApplication{
		opts:         opts,
		deps:         deps,
		client:       client,
		csrfToken:    csrfToken,
		accountsPage: accountsPage,
		successPage:  successPage,
		oauthStates:  make(map[string]string),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleAccountsPage)
	mux.HandleFunc("/accounts", app.handleListAccounts)
	mux.HandleFunc("/auth/start", app.handleAuthStart)
	mux.HandleFunc("/auth/upgrade", app.handleAuthUpgrade)
	mux.HandleFunc("/oauth2/callback", app.handleOAuthCallback)
	mux.HandleFunc("/set-default", app.handleSetDefault)
	mux.HandleFunc("/remove-account", app.handleRemoveAccount)
	app.handler = mux

	return app, nil
}

// Handler returns the accounts manager HTTP handler.
func (app *ManagerApplication) Handler() http.Handler {
	return app.handler
}

func (app *ManagerApplication) handleAccountsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := struct {
		CSRFToken string
	}{
		CSRFToken: app.csrfToken,
	}

	_ = app.accountsPage.Execute(w, data)
}

func (app *ManagerApplication) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tokens, err := app.deps.Tokens.ListTokens()
	if err != nil {
		writeJSONError(w, "Failed to list accounts", http.StatusInternalServerError)
		return
	}

	filtered := make([]secrets.Token, 0, len(tokens))
	for _, tok := range tokens {
		if tok.Client == app.client {
			filtered = append(filtered, tok)
		}
	}

	defaultEmail, _ := app.deps.Tokens.GetDefaultAccount(app.client)
	if !tokenListHasEmail(filtered, defaultEmail) {
		defaultEmail = ""
	}

	accounts := make([]AccountInfo, 0, len(filtered))
	for i, t := range filtered {
		isDefault := i == 0 // First account is default if none set
		if defaultEmail != "" {
			isDefault = t.Email == defaultEmail
		}

		accounts = append(accounts, AccountInfo{
			Email:     t.Email,
			Services:  t.Services,
			IsDefault: isDefault,
		})
	}

	writeJSON(w, map[string]any{"accounts": accounts})
}

func (app *ManagerApplication) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !app.validQueryCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	creds, err := app.deps.ReadCredentials(app.client)
	if err != nil {
		http.Error(w, "OAuth credentials not configured. Run: gog auth credentials <file>", http.StatusInternalServerError)
		return
	}

	state, codeVerifier, err := app.newOAuthState()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}

	services := manageServices(app.opts.Services)

	scopes, err := ScopesForManage(services)
	if err != nil {
		http.Error(w, "Failed to get scopes", http.StatusInternalServerError)
		return
	}

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     app.deps.OAuthEndpoint,
		RedirectURL:  app.opts.RedirectURI,
		Scopes:       scopes,
	}

	authURL := cfg.AuthCodeURL(state, pkceAuthURLParams(app.opts.ForceConsent, true, codeVerifier)...)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (app *ManagerApplication) handleAuthUpgrade(w http.ResponseWriter, r *http.Request) {
	// Similar to handleAuthStart, but always forces consent to get new scopes
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !app.validQueryCSRF(r) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "Missing email parameter", http.StatusBadRequest)
		return
	}

	creds, err := app.deps.ReadCredentials(app.client)
	if err != nil {
		http.Error(w, "OAuth credentials not configured. Run: gog auth credentials <file>", http.StatusInternalServerError)
		return
	}

	state, codeVerifier, err := app.newOAuthState()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}

	// Use requested manage services (exclude Keep)
	services := manageServices(app.opts.Services)

	scopes, err := ScopesForManage(services)
	if err != nil {
		http.Error(w, "Failed to get scopes", http.StatusInternalServerError)
		return
	}

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     app.deps.OAuthEndpoint,
		RedirectURL:  app.opts.RedirectURI,
		Scopes:       scopes,
	}

	// Always force consent for upgrades to ensure user sees all scopes
	// Add login_hint to pre-select the account
	authURL := cfg.AuthCodeURL(state,
		append(pkceAuthURLParams(true, true, codeVerifier),
			oauth2.SetAuthURLParam("login_hint", email))...)

	http.Redirect(w, r, authURL, http.StatusFound) //nolint:gosec // provider URL is fixed; login_hint is query-escaped
}

func (app *ManagerApplication) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if q.Get("error") != "" {
		w.WriteHeader(http.StatusOK)
		renderCancelledPage(w)

		return
	}

	codeVerifier, ok := app.consumeOAuthState(q.Get("state"))
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		renderErrorPage(w, "State mismatch - possible CSRF attack. Please try again.")

		return
	}

	if codeVerifier == "" {
		w.WriteHeader(http.StatusBadRequest)
		renderErrorPage(w, "Missing PKCE verifier. Please try again.")

		return
	}

	code := q.Get("code")
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		renderErrorPage(w, "Missing authorization code. Please try again.")

		return
	}

	creds, err := app.deps.ReadCredentials(app.client)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to read credentials")

		return
	}

	services := manageServices(app.opts.Services)

	scopes, err := ScopesForManage(services)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to get scopes: "+err.Error())

		return
	}

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     app.deps.OAuthEndpoint,
		RedirectURL:  app.opts.RedirectURI,
		Scopes:       scopes,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to exchange code for token: "+err.Error())

		return
	}

	if tok.RefreshToken == "" {
		w.WriteHeader(http.StatusBadRequest)
		renderErrorPage(w, "No refresh token received. Try again with force-consent.")

		return
	}

	identity, err := app.deps.FetchIdentity(ctx, tok)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to fetch user email: "+err.Error())

		return
	}
	email := identity.Email

	if keychainErr := app.deps.EnsureKeychainAccess(ctx); keychainErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Keychain is locked: "+keychainErr.Error())

		return
	}

	serviceNames := make([]string, 0, len(services))
	for _, svc := range services {
		serviceNames = append(serviceNames, string(svc))
	}

	migratedEmail, err := FindStoredSubjectIdentityEmail(app.deps.Tokens, app.client, identity)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to inspect stored token: "+err.Error())

		return
	}

	if err := app.deps.Tokens.SetToken(app.client, email, secrets.Token{
		Subject:      identity.Subject,
		Email:        email,
		Services:     serviceNames,
		Scopes:       scopes,
		RefreshToken: tok.RefreshToken,
	}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to store token: "+err.Error())

		return
	}

	if migratedEmail != "" {
		if err := MigrateStoredEmailReferences(
			app.deps.Tokens,
			app.deps.UpdateEmailReferences,
			app.client,
			migratedEmail,
			email,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			renderErrorPage(w, "Failed to migrate stored token references: "+err.Error())

			return
		}

		if err := DeleteStoredEmailAlias(app.deps.Tokens, app.client, migratedEmail); err != nil {
			slog.Warn("delete migrated token alias failed", "old_email", migratedEmail, "new_email", email, "client", app.client, "err", err)
		}
	}

	w.WriteHeader(http.StatusOK)
	app.renderSuccessPage(w, email, serviceNames)
}

func (app *ManagerApplication) handleSetDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("X-CSRF-Token") != app.csrfToken {
		writeJSONError(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	var req struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	email := normalizeEmail(req.Email)
	if !app.accountExists(email) {
		writeJSONError(w, "Account not found", http.StatusBadRequest)
		return
	}

	if err := app.deps.Tokens.SetDefaultAccount(app.client, email); err != nil {
		writeJSONError(w, "Failed to set default account", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"success": true})
}

func (app *ManagerApplication) handleRemoveAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("X-CSRF-Token") != app.csrfToken {
		writeJSONError(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}

	var req struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	email := normalizeEmail(req.Email)
	if err := app.deps.Tokens.DeleteToken(app.client, email); err != nil {
		writeJSONError(w, "Failed to remove account", http.StatusInternalServerError)
		return
	}

	if defaultEmail, err := app.deps.Tokens.GetDefaultAccount(app.client); err == nil && normalizeEmail(defaultEmail) == email {
		if err := app.resetDefaultAfterRemoval(email); err != nil {
			writeJSONError(w, "Failed to update default account", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, map[string]any{"success": true})
}

func (app *ManagerApplication) validQueryCSRF(r *http.Request) bool {
	return app.csrfToken != "" && r.URL.Query().Get("csrf") == app.csrfToken
}

type defaultAccountDeleter interface {
	DeleteDefaultAccount(client string) error
}

func (app *ManagerApplication) newOAuthState() (string, string, error) {
	app.randomMu.Lock()
	defer app.randomMu.Unlock()

	state, err := generateRandomBase64(app.deps.Random, 32)
	if err != nil {
		return "", "", fmt.Errorf("generate OAuth state: %w", err)
	}

	codeVerifier, err := generateRandomBase64(app.deps.Random, 32)
	if err != nil {
		return "", "", fmt.Errorf("generate PKCE verifier: %w", err)
	}

	app.addOAuthState(state, codeVerifier)

	return state, codeVerifier, nil
}

func (app *ManagerApplication) addOAuthState(state string, codeVerifier string) {
	app.oauthMu.Lock()
	defer app.oauthMu.Unlock()
	app.oauthStates[state] = codeVerifier
}

func (app *ManagerApplication) consumeOAuthState(state string) (string, bool) {
	app.oauthMu.Lock()
	defer app.oauthMu.Unlock()

	codeVerifier, ok := app.oauthStates[state]
	if !ok {
		return "", false
	}

	delete(app.oauthStates, state)

	return codeVerifier, true
}

func (app *ManagerApplication) accountExists(email string) bool {
	tokens, err := app.deps.Tokens.ListTokens()
	if err != nil {
		return false
	}

	for _, tok := range tokens {
		if tok.Client == app.client && normalizeEmail(tok.Email) == email {
			return true
		}
	}

	return false
}

func (app *ManagerApplication) resetDefaultAfterRemoval(removedEmail string) error {
	tokens, err := app.deps.Tokens.ListTokens()
	if err != nil {
		return fmt.Errorf("list accounts after removing default: %w", err)
	}

	for _, tok := range tokens {
		email := normalizeEmail(tok.Email)
		if tok.Client == app.client && email != "" && email != removedEmail {
			if err := app.deps.Tokens.SetDefaultAccount(app.client, email); err != nil {
				return fmt.Errorf("set replacement default account: %w", err)
			}

			return nil
		}
	}

	if deleter, ok := app.deps.Tokens.(defaultAccountDeleter); ok {
		if err := deleter.DeleteDefaultAccount(app.client); err != nil {
			return fmt.Errorf("delete default account: %w", err)
		}
	}

	return nil
}

func tokenListHasEmail(tokens []secrets.Token, email string) bool {
	email = normalizeEmail(email)
	if email == "" {
		return false
	}

	for _, tok := range tokens {
		if normalizeEmail(tok.Email) == email {
			return true
		}
	}

	return false
}

// FetchUserIdentity resolves an OAuth token to a stable Google identity.
func FetchUserIdentity(ctx context.Context, tok *oauth2.Token) (Identity, error) {
	if tok == nil {
		return Identity{}, errMissingToken
	}

	if raw, ok := tok.Extra("id_token").(string); ok && raw != "" {
		if identity, err := IdentityFromIDToken(raw); err == nil {
			return identity, nil
		}
	}

	if tok.AccessToken == "" {
		return Identity{}, errMissingAccessToken
	}

	return fetchUserIdentityWithURL(ctx, tok.AccessToken, userinfoURL)
}

func fetchUserEmailDefault(ctx context.Context, tok *oauth2.Token) (string, error) {
	identity, err := FetchUserIdentity(ctx, tok)
	if err != nil {
		return "", err
	}

	return identity.Email, nil
}

// fetchUserEmailWithURL retrieves the user's email from the specified userinfo URL.
// This is separated for testability.
//
//nolint:unparam // retained for email-only tests and package callers.
func fetchUserEmailWithURL(ctx context.Context, accessToken string, url string) (string, error) {
	identity, err := fetchUserIdentityWithURL(ctx, accessToken, url)
	if err != nil {
		return "", err
	}

	return identity.Email, nil
}

func fetchUserIdentityWithURL(ctx context.Context, accessToken string, url string) (Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Identity{}, fmt.Errorf("create userinfo request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("fetch userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := readHTTPBodySnippet(resp.Body, 512)
		if msg != "" {
			return Identity{}, fmt.Errorf("%w: status %d: %s", errUserinfoRequestFailed, resp.StatusCode, msg)
		}

		return Identity{}, fmt.Errorf("%w: status %d", errUserinfoRequestFailed, resp.StatusCode)
	}

	var userInfo struct {
		Email string `json:"email"`
		Sub   string `json:"sub"`
		ID    string `json:"id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return Identity{}, fmt.Errorf("decode userinfo response: %w", err)
	}

	email := strings.TrimSpace(userInfo.Email)
	if email == "" {
		return Identity{}, errNoEmailInResponse
	}

	subject := strings.TrimSpace(userInfo.Sub)
	if subject == "" {
		subject = strings.TrimSpace(userInfo.ID)
	}

	return Identity{Subject: subject, Email: email}, nil
}

func emailFromIDToken(idToken string) (string, error) {
	identity, err := IdentityFromIDToken(idToken)
	if err != nil {
		return "", err
	}

	return identity.Email, nil
}

func IdentityFromIDToken(idToken string) (Identity, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return Identity{}, errInvalidIDToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}, fmt.Errorf("%w: decode payload: %w", errInvalidIDToken, err)
	}

	var claims struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
	}

	if err := json.Unmarshal(payload, &claims); err != nil {
		return Identity{}, fmt.Errorf("%w: parse payload: %w", errInvalidIDToken, err)
	}

	email := strings.TrimSpace(claims.Email)
	if email == "" {
		return Identity{}, errNoEmailInIDToken
	}

	return Identity{Subject: strings.TrimSpace(claims.Sub), Email: email}, nil
}

func readHTTPBodySnippet(r io.Reader, limit int64) string {
	b, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return ""
	}

	s := strings.TrimSpace(string(b))
	if s == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(s))
	if strings.Contains(s, "access_token") || strings.Contains(s, "refresh_token") || strings.Contains(s, "id_token") {
		return fmt.Sprintf("response_sha256=%x", sum)
	}

	return s
}

func generateCSRFToken() (string, error) {
	return generateRandomHex(rand.Reader, 32)
}

func generateRandomHex(random io.Reader, size int) (string, error) {
	b := make([]byte, size)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}

	return hex.EncodeToString(b), nil
}

func generateRandomBase64(random io.Reader, size int) (string, error) {
	b := make([]byte, size)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func writeJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

// renderSuccessPageWithDetails renders the success template with email and services
func renderSuccessPageWithDetails(w http.ResponseWriter, email string, services []string) {
	renderSuccessPageWithDetailsAndCSRF(w, email, services, "")
}

func renderSuccessPageWithDetailsAndCSRF(w http.ResponseWriter, email string, services []string, csrfToken string) {
	tmpl, err := template.New("success").Parse(successTemplate)
	if err != nil {
		_, _ = w.Write([]byte("Success! You can close this window."))
		return
	}

	renderSuccessTemplate(w, tmpl, email, services, csrfToken)
}

func (app *ManagerApplication) renderSuccessPage(w http.ResponseWriter, email string, services []string) {
	renderSuccessTemplate(w, app.successPage, email, services, app.csrfToken)
}

func renderSuccessTemplate(
	w http.ResponseWriter,
	tmpl *template.Template,
	email string,
	services []string,
	csrfToken string,
) {
	userServices := UserServices()
	allServices := make([]string, 0, len(userServices))

	for _, svc := range userServices {
		allServices = append(allServices, string(svc))
	}

	data := successTemplateData{
		Email:            email,
		Services:         services,
		AllServices:      allServices,
		CountdownSeconds: postSuccessDisplaySeconds,
		CSRFToken:        csrfToken,
	}
	_ = tmpl.Execute(w, data)
}
