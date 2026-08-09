package googleauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
)

// ManageServerOptions configures the accounts management server.
type ManageServerOptions struct {
	Timeout      time.Duration
	Services     []Service
	ForceConsent bool
	Client       string
	ListenAddr   string
	RedirectURI  string
}

type ManagerListenFunc func(context.Context, string, string) (net.Listener, error)

// ManagerLauncherDependencies contains loopback server and application dependencies.
type ManagerLauncherDependencies struct {
	OpenTokens            func(context.Context) (secrets.Store, error)
	ReadCredentials       func(context.Context, string) (config.ClientCredentials, error)
	UpdateEmailReferences func(context.Context, string, string) error
	FetchIdentity         FetchIdentityFunc
	EnsureKeychainAccess  func(context.Context) error
	OpenBrowser           func(context.Context, string) error
	Out                   io.Writer
	Listen                ManagerListenFunc
	Random                io.Reader
	OAuthEndpoint         oauth2.Endpoint
}

// ManagerLauncher owns the loopback listener and browser lifecycle.
type ManagerLauncher struct {
	deps ManagerLauncherDependencies
}

var (
	errManagerTokenOpenerRequired  = errors.New("accounts manager token opener is required")
	errManagerBrowserRequired      = errors.New("accounts manager browser opener is required")
	errManagerOutputRequired       = errors.New("accounts manager output writer is required")
	errManagerListenerRequired     = errors.New("accounts manager listener is required")
	errManagerConfigUpdateRequired = errors.New("accounts manager config updater is required")
)

// NewManagerLauncher validates and captures launcher dependencies.
func NewManagerLauncher(deps ManagerLauncherDependencies) (*ManagerLauncher, error) {
	switch {
	case deps.OpenTokens == nil:
		return nil, errManagerTokenOpenerRequired
	case deps.ReadCredentials == nil:
		return nil, errCredentialsReaderRequired
	case deps.UpdateEmailReferences == nil:
		return nil, errManagerConfigUpdateRequired
	case deps.FetchIdentity == nil:
		return nil, errManagerIdentityRequired
	case deps.EnsureKeychainAccess == nil:
		return nil, errManagerKeychainRequired
	case deps.OpenBrowser == nil:
		return nil, errManagerBrowserRequired
	case deps.Out == nil:
		return nil, errManagerOutputRequired
	case deps.Listen == nil:
		return nil, errManagerListenerRequired
	case deps.Random == nil:
		return nil, errManagerRandomRequired
	case deps.OAuthEndpoint.AuthURL == "" || deps.OAuthEndpoint.TokenURL == "":
		return nil, errManagerOAuthEndpointInvalid
	}

	return &ManagerLauncher{deps: deps}, nil
}

// Start starts the accounts manager and waits for timeout or cancellation.
func (launcher *ManagerLauncher) Start(ctx context.Context, opts ManageServerOptions) error {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}

	listenAddr, err := normalizeListenAddr(opts.ListenAddr)
	if err != nil {
		return err
	}

	if validationErr := validateManagementListenAddr(listenAddr); validationErr != nil {
		return validationErr
	}

	if strings.TrimSpace(opts.RedirectURI) != "" {
		resolvedRedirectURI, normalizeErr := normalizeRedirectURI(opts.RedirectURI)
		if normalizeErr != nil {
			return normalizeErr
		}
		opts.RedirectURI = resolvedRedirectURI
	}

	store, err := launcher.deps.OpenTokens(ctx)
	if err != nil {
		return fmt.Errorf("open accounts manager token store: %w", err)
	}

	ln, err := launcher.deps.Listen(ctx, "tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("start accounts manager listener: %w", err)
	}
	defer ln.Close()

	app, err := NewManagerApplication(ManagerOptions{
		Services:     opts.Services,
		ForceConsent: opts.ForceConsent,
		Client:       opts.Client,
		RedirectURI:  resolveServerRedirectURI(ln, opts.RedirectURI),
	}, launcher.applicationDependencies(ctx, store))
	if err != nil {
		return err
	}

	server := &http.Server{
		Handler:      app.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	resultCh := make(chan error, 1)

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	go func() {
		if serveErr := server.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			select {
			case resultCh <- serveErr:
			default:
			}
		}
	}()

	url := listenerBaseURL(ln)

	fmt.Fprintln(launcher.deps.Out, "Opening accounts manager in browser...")
	fmt.Fprintln(launcher.deps.Out, "If the browser doesn't open, visit:", url)

	if strings.TrimSpace(opts.ListenAddr) != "" {
		fmt.Fprintf(launcher.deps.Out, "Server listening on %s\n", ln.Addr().String())
	}

	if openErr := launcher.deps.OpenBrowser(ctx, url); openErr != nil {
		fmt.Fprintln(launcher.deps.Out, "Failed to open browser:", openErr)
	}

	select {
	case serveErr := <-resultCh:
		return serveErr
	case <-ctx.Done():
		_ = server.Close()
		return nil
	}
}

func (launcher *ManagerLauncher) applicationDependencies(
	ctx context.Context,
	store secrets.Store,
) ManagerDependencies {
	return ManagerDependencies{
		Tokens: store,
		ReadCredentials: func(client string) (config.ClientCredentials, error) {
			return launcher.deps.ReadCredentials(ctx, client)
		},
		UpdateEmailReferences: func(oldEmail, newEmail string) error {
			return launcher.deps.UpdateEmailReferences(ctx, oldEmail, newEmail)
		},
		FetchIdentity: launcher.deps.FetchIdentity,
		EnsureKeychainAccess: func(context.Context) error {
			return launcher.deps.EnsureKeychainAccess(ctx)
		},
		Random:        launcher.deps.Random,
		OAuthEndpoint: launcher.deps.OAuthEndpoint,
	}
}
