package googleauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/input"
)

func authorizeManual(ctx context.Context, opts AuthorizeOptions, creds config.ClientCredentials) (string, error) {
	authURLInput := strings.TrimSpace(opts.AuthURL)
	authCodeInput := strings.TrimSpace(opts.AuthCode)

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauthEndpoint,
		Scopes:       opts.Scopes,
	}

	if authURLInput != "" || authCodeInput != "" {
		return authorizeManualWithCode(ctx, opts, cfg, authURLInput, authCodeInput)
	}

	return authorizeManualInteractive(ctx, opts, cfg)
}

func authorizeManualWithCode(
	ctx context.Context,
	opts AuthorizeOptions,
	cfg oauth2.Config,
	authURLInput string,
	authCodeInput string,
) (string, error) {
	code := strings.TrimSpace(authCodeInput)
	gotState := ""
	gotRedirectURI := ""

	if authURLInput != "" {
		parsedCode, parsedState, parsedRedirectURI, parseErr := parseRedirectURL(authURLInput)
		if parseErr != nil {
			return "", parseErr
		}

		code = parsedCode
		gotState = parsedState
		gotRedirectURI = parsedRedirectURI

		if opts.RequireState && gotState == "" {
			return "", errMissingState
		}
	}

	if strings.TrimSpace(code) == "" {
		return "", errMissingCode
	}

	var st manualState

	if gotState != "" {
		parsed, err := validateManualState(opts, gotState, gotRedirectURI)
		if err != nil {
			return "", err
		}

		st = parsed
	}

	if cfg.RedirectURL == "" && st.RedirectURI != "" {
		cfg.RedirectURL = st.RedirectURI
	}

	if cfg.RedirectURL == "" && gotRedirectURI != "" {
		cfg.RedirectURL = gotRedirectURI
	}

	if cfg.RedirectURL == "" {
		if cached, ok, err := loadManualState(opts.Client, opts.Scopes, opts.ForceConsent); err != nil {
			return "", err
		} else if ok && cached.RedirectURI != "" {
			cfg.RedirectURL = cached.RedirectURI
		}
	}

	if cfg.RedirectURL == "" {
		return "", errMissingRedirectURI
	}

	tok, exchangeErr := cfg.Exchange(ctx, code)
	if exchangeErr != nil {
		return "", fmt.Errorf("exchange code: %w", exchangeErr)
	}

	if tok.RefreshToken == "" {
		return "", errNoRefreshToken
	}

	if gotState != "" {
		_ = clearManualState(gotState)
	}

	return tok.RefreshToken, nil
}

func authorizeManualInteractive(ctx context.Context, opts AuthorizeOptions, cfg oauth2.Config) (string, error) {
	setup, err := manualAuthSetup(ctx, opts)
	if err != nil {
		return "", err
	}

	cfg.RedirectURL = setup.redirectURI
	authURL := cfg.AuthCodeURL(setup.state, authURLParams(opts.ForceConsent)...)

	fmt.Fprintln(os.Stderr, "Visit this URL to authorize:")
	fmt.Fprintln(os.Stderr, authURL)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "After authorizing, you'll be redirected to a loopback URL that won't load.")
	fmt.Fprintln(os.Stderr, "Copy the URL from your browser's address bar and paste it here.")
	fmt.Fprintln(os.Stderr)

	line, readErr := input.PromptLine(ctx, "Paste redirect URL (Enter or Ctrl-D): ")
	if readErr != nil && !errors.Is(readErr, os.ErrClosed) {
		if errors.Is(readErr, io.EOF) {
			return "", fmt.Errorf("authorization canceled: %w", context.Canceled)
		}

		return "", fmt.Errorf("read redirect url: %w", readErr)
	}

	line = strings.TrimSpace(line)

	code, gotState, gotRedirectURI, parseErr := parseRedirectURL(line)
	if parseErr != nil {
		return "", parseErr
	}

	if gotState != "" && gotState != setup.state {
		return "", errStateMismatch
	}

	if gotState != "" {
		st, err := validateManualState(opts, gotState, gotRedirectURI)
		if err != nil {
			return "", err
		}

		if st.RedirectURI != "" {
			cfg.RedirectURL = st.RedirectURI
		}
	}

	tok, exchangeErr := cfg.Exchange(ctx, code)
	if exchangeErr != nil {
		return "", fmt.Errorf("exchange code: %w", exchangeErr)
	}

	if tok.RefreshToken == "" {
		return "", errNoRefreshToken
	}

	_ = clearManualState(setup.state)

	return tok.RefreshToken, nil
}

func validateManualState(opts AuthorizeOptions, gotState string, gotRedirectURI string) (manualState, error) {
	if opts.RequireState {
		if gotState == "" {
			return manualState{}, errMissingState
		}
	}

	if gotState == "" {
		return manualState{}, nil
	}

	path, err := manualStatePathFor(gotState)
	if err != nil {
		return manualState{}, err
	}

	st, ok, err := loadManualStateByPath(path)
	if err != nil {
		return manualState{}, err
	}

	if !ok {
		if opts.RequireState {
			return manualState{}, errManualStateMissing
		}

		return manualState{}, nil
	}

	if st.Client != opts.Client || st.ForceConsent != opts.ForceConsent || !scopesEqual(st.Scopes, opts.Scopes) {
		if opts.RequireState {
			return manualState{}, errManualStateMismatch
		}

		return manualState{}, errStateMismatch
	}

	if gotRedirectURI != "" && st.RedirectURI != "" && st.RedirectURI != gotRedirectURI {
		if opts.RequireState {
			return manualState{}, errManualStateMismatch
		}

		return manualState{}, errStateMismatch
	}

	return st, nil
}

func ManualAuthURL(ctx context.Context, opts AuthorizeOptions) (ManualAuthURLResult, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}

	if len(opts.Scopes) == 0 {
		return ManualAuthURLResult{}, errMissingScopes
	}

	creds, err := readClientCredentials(opts.Client)
	if err != nil {
		return ManualAuthURLResult{}, err
	}

	setup, err := manualAuthSetup(ctx, opts)
	if err != nil {
		return ManualAuthURLResult{}, err
	}

	cfg := oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     oauthEndpoint,
		RedirectURL:  setup.redirectURI,
		Scopes:       opts.Scopes,
	}

	return ManualAuthURLResult{
		URL:         cfg.AuthCodeURL(setup.state, authURLParams(opts.ForceConsent)...),
		StateReused: setup.reused,
	}, nil
}

type manualAuthSetupResult struct {
	state       string
	redirectURI string
	reused      bool
}

func manualAuthSetup(ctx context.Context, opts AuthorizeOptions) (manualAuthSetupResult, error) {
	st, reused, err := loadManualState(opts.Client, opts.Scopes, opts.ForceConsent)
	if err != nil {
		return manualAuthSetupResult{}, err
	}

	state := st.State
	redirectURI := st.RedirectURI

	if !reused {
		redirectURI, err = manualRedirectURIFn(ctx)
		if err != nil {
			return manualAuthSetupResult{}, err
		}

		state, err = randomStateFn()
		if err != nil {
			return manualAuthSetupResult{}, err
		}

		if err := saveManualState(opts.Client, opts.Scopes, opts.ForceConsent, state, redirectURI); err != nil {
			return manualAuthSetupResult{}, err
		}
	}

	return manualAuthSetupResult{
		state:       state,
		redirectURI: redirectURI,
		reused:      reused,
	}, nil
}
