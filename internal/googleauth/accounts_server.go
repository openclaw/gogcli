package googleauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"

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
}

// ManageServer handles the accounts management UI
type ManageServer struct {
	opts       ManageServerOptions
	csrfToken  string
	listener   net.Listener
	server     *http.Server
	store      secrets.Store
	oauthState string
	resultCh   chan error
}

var openDefaultStore = secrets.OpenDefault

// StartManageServer starts the accounts management server and opens browser
func StartManageServer(ctx context.Context, opts ManageServerOptions) error {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}

	store, err := openDefaultStore()
	if err != nil {
		return fmt.Errorf("failed to open secrets store: %w", err)
	}

	csrfToken, err := generateCSRFToken()
	if err != nil {
		return fmt.Errorf("failed to generate CSRF token: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	ms := &ManageServer{
		opts:      opts,
		csrfToken: csrfToken,
		listener:  ln,
		store:     store,
		resultCh:  make(chan error, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", ms.handleAccountsPage)
	mux.HandleFunc("/accounts", ms.handleListAccounts)
	mux.HandleFunc("/auth/start", ms.handleAuthStart)
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

	defaultEmail, _ := ms.store.GetDefaultAccount()

	accounts := make([]AccountInfo, 0, len(tokens))
	for i, t := range tokens {
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
	creds, err := readClientCredentials()
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

	services := ms.opts.Services
	if len(services) == 0 {
		services = AllServices()
	}

	scopes, err := ScopesForServices(services)
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

	authURL := cfg.AuthCodeURL(state, authURLParams(ms.opts.ForceConsent)...)
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

	creds, err := readClientCredentials()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Failed to read credentials")

		return
	}

	services := ms.opts.Services
	if len(services) == 0 {
		services = AllServices()
	}

	scopes, _ := ScopesForServices(services)

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

	// Get user email from token
	email := q.Get("email")
	if email == "" {
		// Try to get email from ID token or use a placeholder
		email = "user@gmail.com"
	}

	// Pre-flight: ensure keychain is accessible before storing token
	if err := secrets.EnsureKeychainAccess(); err != nil { //nolint:contextcheck,nolintlint // keychain ops don't use context; nolint unused on non-Darwin
		w.WriteHeader(http.StatusInternalServerError)
		renderErrorPage(w, "Keychain is locked: "+err.Error())

		return
	}

	// Store the token
	serviceNames := make([]string, 0, len(services))
	for _, svc := range services {
		serviceNames = append(serviceNames, string(svc))
	}

	if err := ms.store.SetToken(email, secrets.Token{
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

	if err := ms.store.SetDefaultAccount(req.Email); err != nil {
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

	if err := ms.store.DeleteToken(req.Email); err != nil {
		writeJSONError(w, "Failed to remove account", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"success": true})
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
	data := successTemplateData{
		Email:            email,
		Services:         services,
		CountdownSeconds: postSuccessDisplaySeconds,
	}
	_ = tmpl.Execute(w, data)
}
