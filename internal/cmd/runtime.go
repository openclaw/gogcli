package cmd

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	goruntime "runtime"
	"time"

	"golang.org/x/oauth2/google"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleauth"
	"github.com/openclaw/gogcli/internal/secrets"
	"github.com/openclaw/gogcli/internal/termutil"
)

var (
	errIncompleteRuntimeLayout = errors.New("injected config store has incomplete runtime layout")
	errRuntimeLayoutMismatch   = errors.New("runtime layout does not match injected config store")
	errRuntimeKeyringRequired  = errors.New("runtime keyring options are required")
	errRuntimeRequired         = errors.New("command runtime is required")
)

func newDefaultRuntime() *app.Runtime {
	keyringOptions := systemKeyringOpenOptions(config.Layout{}, nil)
	runtime := &app.Runtime{
		IO: app.IO{
			In:  os.Stdin,
			Out: os.Stdout,
			Err: os.Stderr,
		},
		Services: app.Services{
			Zoom:          newZoomMeetingClient,
			DriveDownload: driveDownload,
			DriveExport:   driveExportDownload,
			OpenURL:       openPhotosPickerBrowser,
		},
		Auth: app.AuthOperations{
			AuthorizeGoogle:         googleauth.Authorize,
			CheckRefreshToken:       googleauth.CheckRefreshToken,
			EnsureKeychainAccess:    secrets.EnsureKeychainAccessContext,
			FetchAuthorizedIdentity: googleauth.IdentityForRefreshToken,
			ManualAuthURL:           googleauth.ManualAuthURL,
		},
		KeyringOptions:  &keyringOptions,
		ServicesManaged: true,
	}

	return runtime
}

func systemKeyringOpenOptions(layout config.Layout, store *config.ConfigStore) secrets.OpenOptions {
	return secrets.OpenOptionsFromLookup(
		layout,
		store,
		os.LookupEnv,
		goruntime.GOOS,
		termutil.IsTerminal(os.Stdin),
	)
}

func normalizedRuntime(runtime *app.Runtime) *app.Runtime {
	defaults := newDefaultRuntime()
	if runtime == nil {
		return defaults
	}
	normalized := *runtime
	if normalized.IO.In == nil {
		normalized.IO.In = defaults.IO.In
	}
	if normalized.IO.Out == nil {
		normalized.IO.Out = defaults.IO.Out
	}
	if normalized.IO.Err == nil {
		normalized.IO.Err = defaults.IO.Err
	}
	normalizeManagedRuntimeServices(&normalized, defaults)
	normalizeRuntimeAuth(&normalized, defaults)
	return &normalized
}

func normalizeManagedRuntimeServices(runtime *app.Runtime, defaults *app.Runtime) {
	if runtime.Services.GmailDelete == nil && runtime.Services.Gmail != nil {
		runtime.Services.GmailDelete = runtime.Services.Gmail
	}
	if !runtime.ServicesManaged {
		return
	}
	if runtime.Services.Zoom == nil {
		runtime.Services.Zoom = defaults.Services.Zoom
	}
	if runtime.Services.DriveDownload == nil {
		runtime.Services.DriveDownload = defaults.Services.DriveDownload
	}
	if runtime.Services.DriveExport == nil {
		runtime.Services.DriveExport = defaults.Services.DriveExport
	}
	if runtime.Services.OpenURL == nil {
		runtime.Services.OpenURL = defaults.Services.OpenURL
	}
}

func normalizeRuntimeAuth(runtime *app.Runtime, defaults *app.Runtime) {
	if runtime.Auth.OpenSecretsStore == nil {
		runtime.Auth.OpenSecretsStore = func() (secrets.Store, error) {
			return openRuntimeSecretsRepository(runtime)
		}
	}
	if runtime.Auth.OpenSecretStore == nil {
		runtime.Auth.OpenSecretStore = func() (secrets.SecretStore, error) {
			return openRuntimeSecretsRepository(runtime)
		}
	}
	if runtime.Auth.AuthorizeGoogle == nil {
		runtime.Auth.AuthorizeGoogle = defaults.Auth.AuthorizeGoogle
	}
	if runtime.Auth.CheckRefreshToken == nil {
		runtime.Auth.CheckRefreshToken = defaults.Auth.CheckRefreshToken
	}
	if runtime.Auth.EnsureKeychainAccess == nil {
		runtime.Auth.EnsureKeychainAccess = defaults.Auth.EnsureKeychainAccess
	}
	if runtime.Auth.StartManageServer == nil {
		runtime.Auth.StartManageServer = runtimeManageServerStarter(runtime)
	}
	if runtime.Auth.FetchAuthorizedIdentity == nil {
		runtime.Auth.FetchAuthorizedIdentity = defaults.Auth.FetchAuthorizedIdentity
	}
	if runtime.Auth.ManualAuthURL == nil {
		runtime.Auth.ManualAuthURL = defaults.Auth.ManualAuthURL
	}
}

func openRuntimeSecretsRepository(runtime *app.Runtime) (secrets.Repository, error) {
	options, err := runtimeKeyringOpenOptions(runtime)
	if err != nil {
		return nil, err
	}

	return secrets.Open(options)
}

func runtimeKeyringOpenOptions(runtime *app.Runtime) (secrets.OpenOptions, error) {
	if runtime == nil || runtime.KeyringOptions == nil {
		return secrets.OpenOptions{}, errRuntimeKeyringRequired
	}
	if err := configureRuntimeSecrets(runtime); err != nil {
		return secrets.OpenOptions{}, err
	}

	options := *runtime.KeyringOptions
	options.Layout = runtime.Layout
	options.Config = runtime.Config

	return options, nil
}

func bindRuntimeLayoutResolver(runtime *app.Runtime, homeOverride string) error {
	if runtime == nil {
		return errors.New("runtime is nil")
	}
	if runtime.LayoutResolver != nil {
		if homeOverride != "" {
			return errors.New("--home cannot override an injected layout resolver")
		}
		return nil
	}

	runtime.LayoutResolver = config.NewSystemResolver(homeOverride)
	return nil
}

func configureRuntimeConfig(runtime *app.Runtime) error {
	if runtime.Config != nil {
		return hydrateRuntimeLayoutFromConfig(runtime)
	}

	if err := configureRuntimeLayout(runtime, config.PathKindConfig); err != nil {
		return err
	}

	runtime.Config = config.NewConfigStore(runtime.Layout)
	runtime.ConfigManaged = true
	return nil
}

func configureRuntimeSecrets(runtime *app.Runtime) error {
	if err := configureRuntimeLayout(runtime, config.PathKindConfig, config.PathKindData); err != nil {
		return err
	}
	if runtime.Config == nil {
		runtime.Config = config.NewConfigStore(runtime.Layout)
		runtime.ConfigManaged = true
	}
	return nil
}

func configureRuntimeLayout(runtime *app.Runtime, kinds ...config.PathKind) error {
	if err := hydrateRuntimeLayoutFromConfig(runtime); err != nil {
		return err
	}

	missing := make([]config.PathKind, 0, len(kinds))
	for _, kind := range kinds {
		dir, err := runtime.Layout.Dir(kind)
		if err != nil {
			return err
		}
		if dir == "" {
			missing = append(missing, kind)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if runtime.Config != nil && !runtime.ConfigManaged {
		return fmt.Errorf("%w: missing %v", errIncompleteRuntimeLayout, missing)
	}

	if runtime.LayoutResolver == nil {
		runtime.LayoutResolver = config.NewSystemResolver("")
	}
	layout, err := runtime.LayoutResolver.Resolve(missing...)
	if err != nil {
		return err
	}
	for _, kind := range missing {
		dir, dirErr := layout.Dir(kind)
		if dirErr != nil {
			return dirErr
		}
		switch kind {
		case config.PathKindConfig:
			runtime.Layout.ConfigDir = dir
			runtime.Layout.ExplicitConfig = layout.ExplicitConfig
		case config.PathKindData:
			runtime.Layout.DataDir = dir
			runtime.Layout.ExplicitData = layout.ExplicitData
		case config.PathKindState:
			runtime.Layout.StateDir = dir
			runtime.Layout.ExplicitState = layout.ExplicitState
		case config.PathKindCache:
			runtime.Layout.CacheDir = dir
			runtime.Layout.ExplicitCache = layout.ExplicitCache
		}
	}
	runtime.Layout.UsesXDG = runtime.Layout.UsesXDG || layout.UsesXDG
	runtime.Layout.UsesXDGState = runtime.Layout.UsesXDGState || layout.UsesXDGState
	return nil
}

func runtimeKeyringBackendInfo(runtime *app.Runtime) (secrets.KeyringBackendInfo, error) {
	if runtime == nil || runtime.KeyringOptions == nil {
		return secrets.KeyringBackendInfo{}, errRuntimeKeyringRequired
	}
	if err := configureRuntimeConfig(runtime); err != nil {
		return secrets.KeyringBackendInfo{}, err
	}

	options := *runtime.KeyringOptions
	options.Layout = runtime.Layout
	options.Config = runtime.Config
	return secrets.ResolveKeyringBackendInfoWithOptions(options)
}

func hydrateRuntimeLayoutFromConfig(runtime *app.Runtime) error {
	if runtime.Config == nil {
		return nil
	}

	storeLayout := runtime.Config.Layout()
	if runtime.Layout.ConfigDir != "" &&
		storeLayout.ConfigDir != "" &&
		runtime.Layout.ConfigDir != storeLayout.ConfigDir {
		return fmt.Errorf("%w: runtime=%s config_store=%s",
			errRuntimeLayoutMismatch, runtime.Layout.ConfigDir, storeLayout.ConfigDir)
	}

	mergeLayoutKind(&runtime.Layout, storeLayout, config.PathKindConfig)
	mergeLayoutKind(&runtime.Layout, storeLayout, config.PathKindData)
	mergeLayoutKind(&runtime.Layout, storeLayout, config.PathKindState)
	mergeLayoutKind(&runtime.Layout, storeLayout, config.PathKindCache)
	runtime.Layout.UsesXDG = runtime.Layout.UsesXDG || storeLayout.UsesXDG
	runtime.Layout.UsesXDGState = runtime.Layout.UsesXDGState || storeLayout.UsesXDGState
	return nil
}

func mergeLayoutKind(target *config.Layout, source config.Layout, kind config.PathKind) {
	targetDir, _ := target.Dir(kind)
	if targetDir != "" {
		return
	}
	sourceDir, _ := source.Dir(kind)
	if sourceDir == "" {
		return
	}

	switch kind {
	case config.PathKindConfig:
		target.ConfigDir = sourceDir
		target.ExplicitConfig = source.ExplicitConfig
	case config.PathKindData:
		target.DataDir = sourceDir
		target.ExplicitData = source.ExplicitData
	case config.PathKindState:
		target.StateDir = sourceDir
		target.ExplicitState = source.ExplicitState
	case config.PathKindCache:
		target.CacheDir = sourceDir
		target.ExplicitCache = source.ExplicitCache
	}
}

func commandLayout(ctx context.Context, kinds ...config.PathKind) (config.Layout, error) {
	if runtime, ok := app.FromContext(ctx); ok {
		if err := configureRuntimeLayout(runtime, kinds...); err != nil {
			return config.Layout{}, err
		}
		return runtime.Layout, nil
	}
	return config.Layout{}, errRuntimeRequired
}

func commandServiceAccountStore(ctx context.Context) (*config.ServiceAccountStore, error) {
	if runtime, ok := app.FromContext(ctx); ok {
		if runtime.ServiceAccounts != nil {
			return runtime.ServiceAccounts, nil
		}
		if err := configureRuntimeLayout(runtime, config.PathKindConfig, config.PathKindData); err != nil {
			return nil, err
		}
		runtime.ServiceAccounts = config.NewServiceAccountStore(runtime.Layout)
		return runtime.ServiceAccounts, nil
	}

	return nil, errRuntimeRequired
}

func commandUserConfigBase(ctx context.Context) (string, error) {
	runtime, ok := app.FromContext(ctx)
	if !ok || runtime.LayoutResolver == nil {
		return "", errRuntimeRequired
	}

	return runtime.LayoutResolver.UserConfigBase()
}

func resolveRuntimeClient(runtime *app.Runtime, email string, override string) (string, error) {
	if err := configureRuntimeConfig(runtime); err != nil {
		return "", err
	}
	cfg, err := runtime.Config.Read()
	if err != nil {
		return "", err
	}

	return config.ResolveClientForAccountWithCredentials(cfg, email, override, func(client string) (bool, error) {
		if err := configureRuntimeLayout(runtime, config.PathKindConfig, config.PathKindData); err != nil {
			return false, err
		}
		files := config.NewClientCredentialsStore(runtime.Layout)
		_, exists, err := files.ExistingPath(client)
		return exists, err
	})
}

func commandIO(ctx context.Context) app.IO {
	commandIO := newDefaultRuntime().IO
	if runtimeIO, ok := app.IOFromContext(ctx); ok {
		if runtimeIO.In != nil {
			commandIO.In = runtimeIO.In
		}
		if runtimeIO.Out != nil {
			commandIO.Out = runtimeIO.Out
		}
		if runtimeIO.Err != nil {
			commandIO.Err = runtimeIO.Err
		}
	}
	return commandIO
}

func stdoutWriter(ctx context.Context) io.Writer {
	return commandIO(ctx).Out
}

func stderrWriter(ctx context.Context) io.Writer {
	return commandIO(ctx).Err
}

func stdinReader(ctx context.Context) io.Reader {
	return commandIO(ctx).In
}

func stdinIsTerminal(ctx context.Context) bool {
	file, ok := stdinReader(ctx).(*os.File)
	return ok && termutil.IsTerminal(file)
}

func startAuthManageServer(ctx context.Context, options googleauth.ManageServerOptions) error {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Auth.StartManageServer != nil {
		return runtime.Auth.StartManageServer(ctx, options)
	}

	return fmt.Errorf("%w: accounts manager", errRuntimeServiceRequired)
}

func runtimeManageServerStarter(runtime *app.Runtime) app.StartManageServerFunc {
	return func(ctx context.Context, options googleauth.ManageServerOptions) error {
		launcher, err := googleauth.NewManagerLauncher(runtimeManagerLauncherDependencies(runtime))
		if err != nil {
			return err
		}

		return launcher.Start(ctx, options)
	}
}

func runtimeManagerLauncherDependencies(runtime *app.Runtime) googleauth.ManagerLauncherDependencies {
	var output io.Writer
	if runtime != nil {
		output = runtime.IO.Err
	}

	return googleauth.ManagerLauncherDependencies{
		OpenTokens: func(context.Context) (secrets.Store, error) {
			if runtime == nil || runtime.Auth.OpenSecretsStore == nil {
				return nil, fmt.Errorf("%w: accounts manager token store", errRuntimeServiceRequired)
			}

			return runtime.Auth.OpenSecretsStore()
		},
		ReadCredentials:       authclient.ReadCredentials,
		UpdateEmailReferences: authclient.UpdateEmailReferences,
		FetchIdentity:         googleauth.FetchUserIdentity,
		EnsureKeychainAccess:  ensureKeychainAccessIfNeeded,
		OpenBrowser: func(ctx context.Context, url string) error {
			if runtime == nil || runtime.Services.OpenURL == nil {
				return fmt.Errorf("%w: accounts manager browser", errRuntimeServiceRequired)
			}

			return runtime.Services.OpenURL(ctx, url)
		},
		Out: output,
		Listen: func(ctx context.Context, network, address string) (net.Listener, error) {
			return (&net.ListenConfig{}).Listen(ctx, network, address)
		},
		Random:        rand.Reader,
		OAuthEndpoint: google.Endpoint,
	}
}

func checkAuthRefreshToken(ctx context.Context, client, refreshToken string, scopes []string, timeout time.Duration) error {
	if runtime, ok := app.FromContext(ctx); ok && runtime.Auth.CheckRefreshToken != nil {
		return runtime.Auth.CheckRefreshToken(ctx, client, refreshToken, scopes, timeout)
	}
	return fmt.Errorf("%w: refresh token check", errRuntimeServiceRequired)
}

func buildManualAuthURL(ctx context.Context, options googleauth.AuthorizeOptions) (googleauth.ManualAuthURLResult, error) {
	if err := bindManualAuthStateStore(ctx, &options); err != nil {
		return googleauth.ManualAuthURLResult{}, err
	}
	if runtime, ok := app.FromContext(ctx); ok && runtime.Auth.ManualAuthURL != nil {
		return runtime.Auth.ManualAuthURL(ctx, options)
	}
	return googleauth.ManualAuthURLResult{}, fmt.Errorf("%w: manual authorization URL", errRuntimeServiceRequired)
}
