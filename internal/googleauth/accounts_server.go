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
	"net"
	"net/http"
	"os"
	"strings"
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

// ManageServerOptions configures the accounts management server
type ManageServerOptions struct {
	Timeout      time.Duration
	Services     []Service
	ForceConsent bool
	Client       string
}

// ManageServer handles the accounts management UI
type ManageServer struct {
	opts       ManageServerOptions
	client     string
	csrfToken  string
	listener   net.Listener
	server     *http.Server
	store      secrets.Store
	fetchEmail func(ctx context.Context, tok *oauth2.Token) (string, error)
	oauthState string
	resultCh   chan error
}

var (
	openDefaultStore          = secrets.OpenDefault
	resolveKeyringBackendInfo = secrets.ResolveKeyringBackendInfo
	ensureKeychainAccess      = secrets.EnsureKeychainAccess
)

var (
	errUserinfoRequestFailed = errors.New("userinfo request failed")
	errMissingToken          = errors.New("missing token")
	errMissingAccessToken    = errors.New("missing access token")
	errInvalidIDToken        = errors.New("invalid id_token")
	errNoEmailInIDToken      = errors.New("no email in id_token")
	errNoEmailInResponse     = errors.New("no email in userinfo response")
)

const userinfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

func shouldEnsureKeychainAccess() (bool, error) {
	backendInfo, err := resolveKeyringBackendInfo()
	if err != nil {
		return false, err
	}

	return backendInfo.Value != "file", nil
}

// StartManageServer starts the accounts management server and opens browser
func StartManageServer(ctx context.Context, opts ManageServerOptions) error {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}

	client, err := config.NormalizeClientNameOrDefault(opts.Client)
	if err != nil {
		return fmt.Errorf("resolve client: %w", err)
	}
	opts.Client = client

	store, err := openDefaultStore()
	if err != nil {
		return fmt.Errorf("failed to open secrets store: %w", err)
	}

	csrfToken, err := generateCSRFToken()
	if err != nil {
		return fmt.Errorf("failed to generate CSRF token: %w", err)
	}

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	ms := &ManageServer{
		opts:       opts,
		client:     opts.Client,
		csrfToken:  csrfToken,
		listener:   ln,
		store:      store,
		fetchEmail: fetchUserEmailDefault,
		resultCh:   make(chan error, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", ms.handleAccountsPage)
	mux.HandleFunc("/accounts", ms.handleListAccounts)
	mux.HandleFunc("/auth/start", ms.handleAuthStart)
	mux.HandleFunc("/auth/upgrade", ms.handleAuthUpgrade)
	mux.HandleFunc("/oauth2/callback", ms.handleOAuthCallback)
	mux.HandleFunc("/set-default", ms.handleSetDefault)
	mux.HandleFunc("/remove-account", ms.handleRemoveAccount)

	ms.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = ms.server.Close()
	}()

	go func() {
		if err := ms.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case ms.resultCh <- err:
			default:
			}
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	fmt.Fprintln(os.Stderr, "Opening accounts manager in browser...")
	fmt.Fprintln(os.Stderr, "If the browser doesn't open, visit:", url)
	_ = openBrowserFn(url)

	select {
	case err := <-ms.resultCh:
		return err
	case <-ctx.Done():
		_ = ms.server.Close()
		return nil
	}
}

func (ms *ManageServer) handleAccountsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl, err := template.New("accounts").Parse(accountsTemplate)
	if err != nil {
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}

	data := struct {
		CSRFToken string
	}{
		CSRFToken: ms.csrfToken,
	}

	_ = tmpl.Execute(w, data)
}

func (ms *ManageServer) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tokens, err := ms.store.ListTokens()
	if err != nil {
		writeJSONError(w, "Failed to list accounts", http.StatusInternalServerError)
		return
	}

	filtered := make([]secrets.Token, 0, len(tokens))
	for _, tok := range tokens {
		if tok.Client == ms.client {
			filtered = append(filtered, tok)
		}
	}

	defaultEmail, _ := ms.store.GetDefaultAccount(ms.client)

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

func (ms *ManageServer) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	creds, err := readClientCredentials(ms.client)
	if err != nil {
		http.Error(w, "OAuth credentials not configured. Run: gog auth credentials <file>", http.StatusInternalServerError)
		return
	}

	state, err := randomStateFn()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}
	ms.oauthState = state

	services := manageServices(ms.opts.Services)

	scopes, err := ScopesForManage(services)
	if err != nil {
		http.Error(w, "Failed to get scopes", http.StatusInternalServerError)
		return
	}

	port := ms.listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/oauth2/callback", port)

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauthEndpoint,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}

	authURL := cfg.AuthCodeURL(state, authURLParams(ms.opts.ForceConsent, true)...)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (ms *ManageServer) handleAuthUpgrade(w http.ResponseWriter, r *http.Request) {
	// Similar to handleAuthStart, but always forces consent to get new scopes
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "Missing email parameter", http.StatusBadRequest)
		return
	}

	creds, err := readClientCredentials(ms.client)
	if err != nil {
		http.Error(w, "OAuth credentials not configured. Run: gog auth credentials <file>", http.StatusInternalServerError)
		return
	}

	state, err := randomStateFn()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}
	ms.oauthState = state

	// Use requested manage services (exclude Keep)
	services := manageServices(ms.opts.Services)

	scopes, err := ScopesForManage(services)
	if err != nil {
		http.Error(w, "Failed to get scopes", http.StatusInternalServerError)
		return
	}

	port := ms.listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/oauth2/callback", port)

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauthEndpoint,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}

	// Always force consent for upgrades to ensure user sees all scopes
	// Add login_hint to pre-select the account
	authURL := cfg.AuthCodeURL(state,
		append(authURLParams(true, true),
			oauth2.SetAuthURLParam("login_hint", email))...)

	http.Redirect(w, r, authURL, http.StatusFound)
}

func (ms *ManageServer) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if q.Get("error") != "" {
		w.WriteHeader(http.StatusOK)
		renderCancelledPage(w)

		return
	}

	if q.Get("state") != ms.oauthState {
		w.WriteHeader(http.StatusBadRequest)
		renderErrorPage(w, "State mismatch - possible CSRF attack. Please try again.")

		return
	}

	code := q.Get("code")
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		renderErrorPage(w, "Missing authorization code. Please try again.")

		return
	}

	creds, err := readClientCredentials(ms.client)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to read credentials")

		return
	}

	services := manageServices(ms.opts.Services)

	scopes, err := ScopesForManage(services)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to get scopes: "+err.Error())

		return
	}

	port := ms.listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/oauth2/callback", port)

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauthEndpoint,
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	tok, err := cfg.Exchange(ctx, code)
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

	fetchEmail := ms.fetchEmail
	if fetchEmail == nil {
		fetchEmail = fetchUserEmailDefault
	}

	// Fetch user email from Google's userinfo API
	email, err := fetchEmail(ctx, tok)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to fetch user email: "+err.Error())

		return
	}

	// Pre-flight: ensure keychain is accessible before storing token
	needKeychain, err := shouldEnsureKeychainAccess()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to resolve keyring backend: "+err.Error())

		return
	}

	if needKeychain {
		if err := ensureKeychainAccess(); err != nil { //nolint:contextcheck,nolintlint // keychain ops don't use context; nolint unused on non-Darwin
			w.WriteHeader(http.StatusInternalServerError)
			renderErrorPage(w, "Keychain is locked: "+err.Error())

			return
		}
	}

	// Store the token
	serviceNames := make([]string, 0, len(services))
	for _, svc := range services {
		serviceNames = append(serviceNames, string(svc))
	}

	if err := ms.store.SetToken(ms.client, email, secrets.Token{
		Email:        email,
		Services:     serviceNames,
		Scopes:       scopes,
		RefreshToken: tok.RefreshToken,
	}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to store token: "+err.Error())

		return
	}

	// Render success page with the new template
	w.WriteHeader(http.StatusOK)
	renderSuccessPageWithDetails(w, email, serviceNames)
}

func (ms *ManageServer) handleSetDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("X-CSRF-Token") != ms.csrfToken {
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

	if err := ms.store.SetDefaultAccount(ms.client, req.Email); err != nil {
		writeJSONError(w, "Failed to set default account", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"success": true})
}

func (ms *ManageServer) handleRemoveAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("X-CSRF-Token") != ms.csrfToken {
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

	if err := ms.store.DeleteToken(ms.client, req.Email); err != nil {
		writeJSONError(w, "Failed to remove account", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"success": true})
}

func fetchUserEmailDefault(ctx context.Context, tok *oauth2.Token) (string, error) {
	if tok == nil {
		return "", errMissingToken
	}

	if raw, ok := tok.Extra("id_token").(string); ok && raw != "" {
		if email, err := emailFromIDToken(raw); err == nil {
			return email, nil
		}
	}

	if tok.AccessToken == "" {
		return "", errMissingAccessToken
	}

	return fetchUserEmailWithURL(ctx, tok.AccessToken, userinfoURL)
}

// fetchUserEmailWithURL retrieves the user's email from the specified userinfo URL.
// This is separated for testability.
func fetchUserEmailWithURL(ctx context.Context, accessToken string, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create userinfo request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := readHTTPBodySnippet(resp.Body, 512)
		if msg != "" {
			return "", fmt.Errorf("%w: status %d: %s", errUserinfoRequestFailed, resp.StatusCode, msg)
		}

		return "", fmt.Errorf("%w: status %d", errUserinfoRequestFailed, resp.StatusCode)
	}

	var userInfo struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return "", fmt.Errorf("decode userinfo response: %w", err)
	}

	if userInfo.Email == "" {
		return "", errNoEmailInResponse
	}

	return userInfo.Email, nil
}

func emailFromIDToken(idToken string) (string, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return "", errInvalidIDToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("%w: decode payload: %w", errInvalidIDToken, err)
	}

	var claims struct {
		Email string `json:"email"`
	}

	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("%w: parse payload: %w", errInvalidIDToken, err)
	}

	email := strings.TrimSpace(claims.Email)
	if email == "" {
		return "", errNoEmailInIDToken
	}

	return email, nil
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
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate csrf token: %w", err)
	}

	return hex.EncodeToString(b), nil
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
	tmpl, err := template.New("success").Parse(successTemplate)
	if err != nil {
		_, _ = w.Write([]byte("Success! You can close this window."))
		return
	}

	// Show available user services for connected vs missing
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
	}
	_ = tmpl.Execute(w, data)
}
