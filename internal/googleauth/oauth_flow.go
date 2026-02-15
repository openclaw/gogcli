package googleauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/steipete/gogcli/internal/config"
)

type AuthorizeOptions struct {
	Services     []Service
	Scopes       []string
	Manual       bool
	ForceConsent bool
	Timeout      time.Duration
	Client       string
	AuthCode     string
	AuthURL      string
	RequireState bool
}

type ManualAuthURLResult struct {
	URL         string
	StateReused bool
}

// postSuccessDisplaySeconds is the number of seconds the success page remains
// visible before the local OAuth server shuts down.
const postSuccessDisplaySeconds = 30

// successTemplateData holds data passed to the success page template.
type successTemplateData struct {
	Email            string
	Services         []string
	AllServices      []string
	CountdownSeconds int
}

var (
	readClientCredentials = config.ReadClientCredentialsFor
	openBrowserFn         = openBrowser
	oauthEndpoint         = google.Endpoint
	randomStateFn         = randomState
	manualRedirectURIFn   = randomManualRedirectURI
)

var (
	errAuthorization       = errors.New("authorization error")
	errInvalidRedirectURL  = errors.New("invalid redirect URL")
	errMissingCode         = errors.New("missing code")
	errMissingRedirectURI  = errors.New("missing redirect uri; provide auth-url")
	errMissingState        = errors.New("missing state in redirect URL")
	errMissingScopes       = errors.New("missing scopes")
	errNoCodeInURL         = errors.New("no code found in URL")
	errNoRefreshToken      = errors.New("no refresh token received; try again with --force-consent")
	errManualStateMissing  = errors.New("manual auth state missing; run remote step 1 again")
	errManualStateMismatch = errors.New("manual auth state mismatch; run remote step 1 again")
	errStateMismatch       = errors.New("state mismatch")

	errInvalidAuthorizeOptionsAuthURLAndCode    = errors.New("cannot combine auth-url with auth-code")
	errInvalidAuthorizeOptionsAuthCodeWithState = errors.New("auth-code is not valid when state is required; provide auth-url")
)

func Authorize(ctx context.Context, opts AuthorizeOptions) (string, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}

	if strings.TrimSpace(opts.AuthURL) != "" && strings.TrimSpace(opts.AuthCode) != "" {
		return "", errInvalidAuthorizeOptionsAuthURLAndCode
	}

	if opts.RequireState && strings.TrimSpace(opts.AuthCode) != "" {
		return "", errInvalidAuthorizeOptionsAuthCodeWithState
	}

	if len(opts.Scopes) == 0 {
		return "", errMissingScopes
	}

	creds, err := readClientCredentials(opts.Client)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	if opts.Manual {
		return authorizeManual(ctx, opts, creds)
	}

	return authorizeServer(ctx, opts, creds)
}

func authorizeServer(ctx context.Context, opts AuthorizeOptions, creds config.ClientCredentials) (string, error) {
	state, err := randomStateFn()
	if err != nil {
		return "", err
	}

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen for callback: %w", err)
	}

	defer func() { _ = ln.Close() }()

	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/oauth2/callback", port)

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauthEndpoint,
		RedirectURL:  redirectURI,
		Scopes:       opts.Scopes,
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	srv := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/oauth2/callback" {
				http.NotFound(w, r)
				return
			}
			q := r.URL.Query()

			w.Header().Set("Content-Type", "text/html; charset=utf-8")

			if q.Get("error") != "" {
				select {
				case errCh <- fmt.Errorf("%w: %s", errAuthorization, q.Get("error")):
				default:
				}

				w.WriteHeader(http.StatusOK)
				renderCancelledPage(w)

				return
			}

			if q.Get("state") != state {
				select {
				case errCh <- errStateMismatch:
				default:
				}

				w.WriteHeader(http.StatusBadRequest)
				renderErrorPage(w, "State mismatch - possible CSRF attack. Please try again.")

				return
			}

			code := q.Get("code")
			if code == "" {
				select {
				case errCh <- errMissingCode:
				default:
				}

				w.WriteHeader(http.StatusBadRequest)
				renderErrorPage(w, "Missing authorization code. Please try again.")

				return
			}

			select {
			case codeCh <- code:
			default:
			}

			w.WriteHeader(http.StatusOK)
			renderSuccessPage(w)
		}),
	}

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	authURL := cfg.AuthCodeURL(state, authURLParams(opts.ForceConsent)...)

	fmt.Fprintln(os.Stderr, "Opening browser for authorization…")
	fmt.Fprintln(os.Stderr, "If the browser doesn't open, visit this URL:")
	fmt.Fprintln(os.Stderr, authURL)
	_ = openBrowserFn(authURL)

	select {
	case code := <-codeCh:
		fmt.Fprintln(os.Stderr, "Authorization received. Finishing…")
		var tok *oauth2.Token

		if t, exchangeErr := cfg.Exchange(ctx, code); exchangeErr != nil {
			_ = srv.Close()

			return "", fmt.Errorf("exchange code: %w", exchangeErr)
		} else {
			tok = t
		}

		if tok.RefreshToken == "" {
			_ = srv.Close()

			return "", errNoRefreshToken
		}

		shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)

		return tok.RefreshToken, nil
	case err := <-errCh:
		_ = srv.Close()
		return "", err
	case <-ctx.Done():
		_ = srv.Close()

		return "", fmt.Errorf("authorization canceled: %w", ctx.Err())
	}
}

func authURLParams(forceConsent bool) []oauth2.AuthCodeOption {
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
	}
	if forceConsent {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "consent"))
	}

	return opts
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// renderSuccessPage renders the success HTML template
func renderSuccessPage(w http.ResponseWriter) {
	tmpl, err := template.New("success").Parse(successTemplate)
	if err != nil {
		_, _ = w.Write([]byte("Success! You can close this window."))
		return
	}
	data := successTemplateData{
		CountdownSeconds: postSuccessDisplaySeconds,
	}
	_ = tmpl.Execute(w, data)
}

// renderErrorPage renders the error HTML template with the given message
func renderErrorPage(w http.ResponseWriter, errorMsg string) {
	tmpl, err := template.New("error").Parse(errorTemplate)
	if err != nil {
		_, _ = w.Write([]byte("Error: " + errorMsg))
		return
	}
	_ = tmpl.Execute(w, struct{ Error string }{Error: errorMsg})
}

// renderCancelledPage renders the cancelled HTML template
func renderCancelledPage(w http.ResponseWriter) {
	tmpl, err := template.New("cancelled").Parse(cancelledTemplate)
	if err != nil {
		_, _ = w.Write([]byte("Authorization cancelled. You can close this window."))
		return
	}
	_ = tmpl.Execute(w, nil)
}

// waitPostSuccess waits for the specified duration or until the context is
// cancelled (e.g., via Ctrl+C). Kept for tests and potential future UX tweaks.
func waitPostSuccess(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
