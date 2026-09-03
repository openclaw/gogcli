package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	admin "google.golang.org/api/admin/directory/v1"
	adsenseapi "google.golang.org/api/adsense/v2"
	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/chat/v1"
	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/drive/v3"
	driveactivityapi "google.golang.org/api/driveactivity/v2"
	drivelabelsapi "google.golang.org/api/drivelabels/v2"
	formsapi "google.golang.org/api/forms/v1"
	"google.golang.org/api/gmail/v1"
	keepapi "google.golang.org/api/keep/v1"
	meetapi "google.golang.org/api/meet/v2"
	"google.golang.org/api/people/v1"
	scriptapi "google.golang.org/api/script/v1"
	searchconsoleapi "google.golang.org/api/searchconsole/v1"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"
	youtubeapi "google.golang.org/api/youtube/v3"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/googleauth"
	"github.com/openclaw/gogcli/internal/secrets"
)

func TestDefaultRuntimeSnapshotsKeyringOptions(t *testing.T) {
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "")
	t.Setenv("GOG_KEYRING_SERVICE_NAME", "snapshot")
	t.Setenv("GOG_KEYRING_LOCK_TIMEOUT", "250ms")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/snapshot")

	runtime := newDefaultRuntime()
	t.Setenv("GOG_KEYRING_BACKEND", "keychain")
	t.Setenv("GOG_KEYRING_SERVICE_NAME", "changed")

	options := runtime.KeyringOptions
	if options == nil {
		t.Fatal("expected keyring options")
	}
	if options.Backend != "file" || options.ServiceName != "snapshot" {
		t.Fatalf("options = %#v", options)
	}
	if options.Password != "" || !options.PasswordSet {
		t.Fatalf("empty password presence was not captured: %#v", options)
	}
	if options.GOOS != goruntime.GOOS || options.DBusAddress != "unix:path=/snapshot" {
		t.Fatalf("platform options = %#v", options)
	}
	if options.LockTimeout != 250*time.Millisecond {
		t.Fatalf("lock timeout = %v", options.LockTimeout)
	}
}

func TestNormalizedRuntimeRequiresKeyringOptions(t *testing.T) {
	t.Parallel()

	layout := config.Layout{ConfigDir: t.TempDir(), DataDir: t.TempDir()}
	runtime := normalizedRuntime(&app.Runtime{
		Layout: layout,
		Config: config.NewConfigStore(layout),
	})

	if _, err := runtime.Auth.OpenSecretsStore(); !errors.Is(err, errRuntimeKeyringRequired) {
		t.Fatalf("error = %v, want keyring options required", err)
	}
}

func TestNormalizedInjectedRuntimeDoesNotFillServices(t *testing.T) {
	t.Parallel()

	runtime := normalizedRuntime(&app.Runtime{})
	if runtime.Services.Drive != nil || runtime.Services.Gmail != nil || runtime.Services.Zoom != nil {
		t.Fatalf("injected runtime services were completed: %#v", runtime.Services)
	}
}

func TestComposeRuntimeGoogleServicesPreservesOverrides(t *testing.T) {
	t.Parallel()

	wantDrive := &drive.Service{}
	runtime := &app.Runtime{
		ServicesManaged: true,
		Services: app.Services{
			Drive: func(context.Context, string) (*drive.Service, error) {
				return wantDrive, nil
			},
		},
	}
	factory := googleapi.NewFactory(googleapi.AuthDependencies{}, googleapi.FactoryOptions{})

	composeRuntimeGoogleServices(runtime, factory)

	gotDrive, err := runtime.Services.Drive(context.Background(), "user@example.com")
	if err != nil || gotDrive != wantDrive {
		t.Fatalf("Drive override = (%p, %v), want %p", gotDrive, err, wantDrive)
	}
	if runtime.Services.Gmail == nil ||
		runtime.Services.Docs == nil ||
		runtime.Services.Calendar == nil ||
		runtime.Services.ChatSearch == nil ||
		runtime.Services.Photos == nil ||
		runtime.Services.AdSense == nil ||
		runtime.Services.YouTubeWrite == nil {
		t.Fatalf("representative factory services missing: %#v", runtime.Services)
	}
}

func TestComposeRuntimeGoogleServicesSkipsInjectedRuntime(t *testing.T) {
	t.Parallel()

	runtime := &app.Runtime{}
	composeRuntimeGoogleServices(runtime, googleapi.NewFactory(googleapi.AuthDependencies{}, googleapi.FactoryOptions{}))
	if runtime.Services.Drive != nil {
		t.Fatal("injected runtime received a production Drive service")
	}
}

func TestDriveServiceMissingRuntimeServiceFailsClosed(t *testing.T) {
	t.Parallel()

	ctx := app.WithRuntime(context.Background(), &app.Runtime{})
	_, err := driveService(ctx, "user@example.com")
	if !errors.Is(err, errRuntimeServiceRequired) {
		t.Fatalf("driveService() error = %v, want %v", err, errRuntimeServiceRequired)
	}
	if !strings.Contains(err.Error(), "drive") {
		t.Fatalf("driveService() error = %v, want service name", err)
	}
}

func TestCommandDependenciesMissingRuntimeFailClosed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "layout",
			run: func() error {
				_, err := commandLayout(ctx, config.PathKindConfig)
				return err
			},
		},
		{
			name: "config",
			run: func() error {
				_, err := commandConfigStore(ctx)
				return err
			},
		},
		{
			name: "service accounts",
			run: func() error {
				_, err := commandServiceAccountStore(ctx)
				return err
			},
		},
		{
			name: "secrets",
			run: func() error {
				_, err := openAuthSecretsStore(ctx)
				return err
			},
		},
		{
			name: "authorize",
			run: func() error {
				_, err := authorizeGoogleAccount(ctx, googleauth.AuthorizeOptions{})
				return err
			},
		},
		{
			name: "identity",
			run: func() error {
				_, err := fetchAuthIdentity(ctx, "", "", nil, time.Second)
				return err
			},
		},
		{name: "keychain", run: func() error { return ensureKeychainAccessIfNeeded(ctx) }},
		{
			name: "refresh check",
			run: func() error {
				return checkAuthRefreshToken(ctx, "", "", nil, time.Second)
			},
		},
		{
			name: "manual URL",
			run: func() error {
				_, err := buildManualAuthURL(ctx, googleauth.AuthorizeOptions{})
				return err
			},
		},
		{
			name: "zoom",
			run: func() error {
				_, err := commandZoomStore(ctx)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.run()
			if !errors.Is(err, errRuntimeRequired) && !errors.Is(err, errRuntimeServiceRequired) {
				t.Fatalf("error = %v, want runtime dependency error", err)
			}
		})
	}
}

func TestResolveKeyringBackendInfoUsesRuntimeOptions(t *testing.T) {
	t.Setenv("GOG_KEYRING_BACKEND", "keychain")

	layout := config.Layout{ConfigDir: t.TempDir()}
	store := config.NewConfigStore(layout)
	if err := store.Write(config.File{KeyringBackend: "auto"}); err != nil {
		t.Fatalf("write config: %v", err)
	}
	options := secrets.OpenOptions{Backend: "file", GOOS: goruntime.GOOS}
	ctx := app.WithRuntime(context.Background(), &app.Runtime{
		Layout:         layout,
		Config:         store,
		KeyringOptions: &options,
	})

	info, err := resolveKeyringBackendInfo(ctx)
	if err != nil {
		t.Fatalf("resolveKeyringBackendInfo: %v", err)
	}
	if info.Value != "file" || info.Source != "env" {
		t.Fatalf("backend info = %#v", info)
	}
}

func TestConfigureRuntimeConfigUsesInjectedLayout(t *testing.T) {
	t.Parallel()

	layout := config.Layout{ConfigDir: t.TempDir()}
	runtime := &app.Runtime{Layout: layout}
	if err := configureRuntimeConfig(runtime); err != nil {
		t.Fatalf("configureRuntimeConfig: %v", err)
	}
	if runtime.Config == nil {
		t.Fatal("expected config store")
	}
	if runtime.Config.Path() != layout.ConfigPath() {
		t.Fatalf("config path = %q, want %q", runtime.Config.Path(), layout.ConfigPath())
	}
}

func TestConfigureRuntimeConfigPreservesInjectedStore(t *testing.T) {
	t.Parallel()

	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	runtime := &app.Runtime{Config: store}
	if err := configureRuntimeConfig(runtime); err != nil {
		t.Fatalf("configureRuntimeConfig: %v", err)
	}
	if runtime.Config != store {
		t.Fatal("injected config store was replaced")
	}
	if runtime.Layout.ConfigDir != store.Layout().ConfigDir {
		t.Fatalf("runtime config dir = %q, want %q", runtime.Layout.ConfigDir, store.Layout().ConfigDir)
	}
}

func TestConfigureRuntimeLayoutPreservesInjectedExplicitKinds(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dataDir := filepath.Join(root, "injected-data")
	runtime := &app.Runtime{Layout: config.Layout{
		DataDir:      dataDir,
		ExplicitData: true,
	}}
	runtime.LayoutResolver = config.NewSystemResolver(filepath.Join(root, "home"))

	if err := configureRuntimeLayout(runtime, config.PathKindConfig); err != nil {
		t.Fatalf("configureRuntimeLayout: %v", err)
	}
	if runtime.Layout.DataDir != dataDir {
		t.Fatalf("data dir = %q, want %q", runtime.Layout.DataDir, dataDir)
	}
	if !runtime.Layout.ExplicitData {
		t.Fatal("injected ExplicitData flag was cleared")
	}
}

func TestConfigureRuntimeLayoutRejectsAmbientFallbackForInjectedConfig(t *testing.T) {
	t.Parallel()

	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	runtime := &app.Runtime{Config: store}

	err := configureRuntimeLayout(runtime, config.PathKindConfig, config.PathKindData)
	if !errors.Is(err, errIncompleteRuntimeLayout) {
		t.Fatalf("configureRuntimeLayout() error = %v, want incomplete runtime layout", err)
	}
}

func TestManagedRuntimeConfigCanResolveAdditionalKinds(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runtime := &app.Runtime{LayoutResolver: config.NewSystemResolver(root)}
	if err := configureRuntimeConfig(runtime); err != nil {
		t.Fatalf("configureRuntimeConfig: %v", err)
	}
	if !runtime.ConfigManaged {
		t.Fatal("runtime-created config store was not marked managed")
	}

	if err := configureRuntimeLayout(runtime, config.PathKindData); err != nil {
		t.Fatalf("configureRuntimeLayout: %v", err)
	}
	if runtime.Layout.DataDir != filepath.Join(root, "data") {
		t.Fatalf("data dir = %q, want %q", runtime.Layout.DataDir, filepath.Join(root, "data"))
	}
}

func TestResolveRuntimeClientUsesInjectedCredentialStore(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	layout := config.Layout{
		ConfigDir:      filepath.Join(root, "config"),
		DataDir:        filepath.Join(root, "data"),
		ExplicitConfig: true,
		ExplicitData:   true,
	}
	files := config.NewClientCredentialsStore(layout)
	if err := files.WriteMetadata("example.com", config.ClientCredentials{ClientID: "id"}); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	runtime := &app.Runtime{Config: config.NewConfigStore(layout)}
	client, err := resolveRuntimeClient(runtime, "user@example.com", "")
	if err != nil {
		t.Fatalf("resolveRuntimeClient: %v", err)
	}
	if client != "example.com" {
		t.Fatalf("client = %q, want example.com", client)
	}
}

func TestNormalizedRuntimeOpensSecretsWithInjectedConfig(t *testing.T) {
	t.Setenv("GOG_KEYRING_BACKEND", "")

	root := t.TempDir()
	layout := config.Layout{
		ConfigDir: filepath.Join(root, "config"),
		DataDir:   filepath.Join(root, "data"),
	}
	store := config.NewConfigStore(layout)
	if err := store.Write(config.File{KeyringBackend: "invalid"}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	options := testKeyringOptions()
	options.Backend = ""
	runtime := normalizedRuntime(&app.Runtime{
		Layout:         layout,
		Config:         store,
		KeyringOptions: options,
	})
	_, err := runtime.Auth.OpenSecretsStore()
	if err == nil || !strings.Contains(err.Error(), "invalid keyring backend") {
		t.Fatalf("OpenSecretsStore() error = %v, want injected backend validation", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenSecretsStore() read ambient config instead of injected store: %v", err)
	}

	_, err = runtime.Auth.OpenSecretStore()
	if err == nil || !strings.Contains(err.Error(), "invalid keyring backend") {
		t.Fatalf("OpenSecretStore() error = %v, want injected backend validation", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenSecretStore() read ambient config instead of injected store: %v", err)
	}
}

func TestExecuteRuntimeRoutesMigratedCommandOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runtime := &app.Runtime{IO: app.IO{
		In:  strings.NewReader(""),
		Out: &stdout,
		Err: &stderr,
	}}

	if err := executeWithRuntime([]string{"--json", "version"}, runtime); err != nil {
		t.Fatalf("executeWithRuntime() error = %v, stderr = %q", err, stderr.String())
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\nstdout=%q", err, stdout.String())
	}
	if got["version"] == "" {
		t.Fatalf("stdout = %#v, want version", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestExecuteRuntimeCapturesADCModeForAccountSelection(t *testing.T) {
	t.Setenv("GOG_AUTH_MODE", "adc")
	t.Setenv("GOG_ACCOUNT", "")

	wantErr := errors.New("stop after account selection")
	var gotAccount string
	runtime := &app.Runtime{
		IO: app.IO{
			In:  strings.NewReader(""),
			Out: io.Discard,
			Err: io.Discard,
		},
		Services: app.Services{
			Drive: func(_ context.Context, account string) (*drive.Service, error) {
				gotAccount = account
				return nil, wantErr
			},
		},
		Auth: app.AuthOperations{
			OpenSecretsStore: func() (secrets.Store, error) {
				return &fakeSecretsStore{}, nil
			},
		},
	}

	err := executeWithRuntime([]string{"drive", "ls"}, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("executeWithRuntime() error = %v, want %v", err, wantErr)
	}
	if gotAccount != adcPlaceholderAccount {
		t.Fatalf("factory account = %q, want %q", gotAccount, adcPlaceholderAccount)
	}
}

func TestExecuteRuntimeRoutesEarlyErrors(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runtime := &app.Runtime{IO: app.IO{
		In:  strings.NewReader(""),
		Out: &stdout,
		Err: &stderr,
	}}

	err := executeWithRuntime([]string{"--json", "--plain", "version"}, runtime)
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("executeWithRuntime() error = %v, want exit code 2", err)
	}
	if !strings.Contains(stderr.String(), "cannot combine --json and --plain") {
		t.Fatalf("stderr = %q, want output mode error", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestAdminDirectoryServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &admin.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		AdminDirectory: func(_ context.Context, account string) (*admin.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := adminDirectoryService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("adminDirectoryService() error = %v", err)
	}
	if got != want {
		t.Fatalf("adminDirectoryService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestAdminOrgUnitDirectoryServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &admin.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		AdminOrgUnit: func(_ context.Context, account string) (*admin.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := adminOrgUnitDirectoryService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("adminOrgUnitDirectoryService() error = %v", err)
	}
	if got != want {
		t.Fatalf("adminOrgUnitDirectoryService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestAppScriptServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &scriptapi.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		AppScript: func(_ context.Context, account string) (*scriptapi.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := appScriptService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("appScriptService() error = %v", err)
	}
	if got != want {
		t.Fatalf("appScriptService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestCloudIdentityServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &cloudidentity.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		CloudIdentity: func(_ context.Context, account string) (*cloudidentity.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := cloudIdentityService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("cloudIdentityService() error = %v", err)
	}
	if got != want {
		t.Fatalf("cloudIdentityService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestKeepServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &keepapi.Service{}
	var gotPath string
	var gotImpersonate string
	runtime := &app.Runtime{Services: app.Services{
		Keep: func(_ context.Context, path, impersonate string) (*keepapi.Service, error) {
			gotPath = path
			gotImpersonate = impersonate
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := keepServiceWithServiceAccount(ctx, "/tmp/service-account.json", "test@example.com")
	if err != nil {
		t.Fatalf("keepServiceWithServiceAccount() error = %v", err)
	}
	if got != want {
		t.Fatalf("keepServiceWithServiceAccount() = %p, want %p", got, want)
	}
	if gotPath != "/tmp/service-account.json" || gotImpersonate != "test@example.com" {
		t.Fatalf("factory args = (%q, %q)", gotPath, gotImpersonate)
	}
}

func TestMeetServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &meetapi.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		Meet: func(_ context.Context, account string) (*meetapi.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := meetService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("meetService() error = %v", err)
	}
	if got != want {
		t.Fatalf("meetService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestPhotosServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &googleapi.PhotosClient{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		Photos: func(_ context.Context, account string) (*googleapi.PhotosClient, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := photosService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("photosService() error = %v", err)
	}
	if got != want {
		t.Fatalf("photosService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestPhotosPickerServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &googleapi.PhotosPickerClient{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		PhotosPicker: func(_ context.Context, account string) (*googleapi.PhotosPickerClient, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := photosPickerService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("photosPickerService() error = %v", err)
	}
	if got != want {
		t.Fatalf("photosPickerService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestOpenURLUsesRuntimeOperation(t *testing.T) {
	t.Parallel()

	var gotURI string
	runtime := &app.Runtime{Services: app.Services{
		OpenURL: func(_ context.Context, uri string) error {
			gotURI = uri
			return nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	if err := openURL(ctx, "https://example.com/picker"); err != nil {
		t.Fatalf("openURL() error = %v", err)
	}
	if gotURI != "https://example.com/picker" {
		t.Fatalf("URI = %q, want picker URI", gotURI)
	}
}

func TestAnalyticsAdminServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &analyticsadmin.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		AnalyticsAdmin: func(_ context.Context, account string) (*analyticsadmin.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := analyticsAdminService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("analyticsAdminService() error = %v", err)
	}
	if got != want {
		t.Fatalf("analyticsAdminService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestAnalyticsDataServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &analyticsdata.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		AnalyticsData: func(_ context.Context, account string) (*analyticsdata.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := analyticsDataService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("analyticsDataService() error = %v", err)
	}
	if got != want {
		t.Fatalf("analyticsDataService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestAdSenseServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &adsenseapi.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		AdSense: func(_ context.Context, account string) (*adsenseapi.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := adSenseService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("adSenseService() error = %v", err)
	}
	if got != want {
		t.Fatalf("adSenseService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestSearchConsoleServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &searchconsoleapi.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		SearchConsole: func(_ context.Context, account string) (*searchconsoleapi.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := searchConsoleService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("searchConsoleService() error = %v", err)
	}
	if got != want {
		t.Fatalf("searchConsoleService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestDriveServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &drive.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		Drive: func(_ context.Context, account string) (*drive.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := driveService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("driveService() error = %v", err)
	}
	if got != want {
		t.Fatalf("driveService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestDriveActivityServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &driveactivityapi.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		DriveActivity: func(_ context.Context, account string) (*driveactivityapi.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := driveActivityService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("driveActivityService() error = %v", err)
	}
	if got != want {
		t.Fatalf("driveActivityService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestDriveLabelsServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &drivelabelsapi.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		DriveLabels: func(_ context.Context, account string) (*drivelabelsapi.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := driveLabelsService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("driveLabelsService() error = %v", err)
	}
	if got != want {
		t.Fatalf("driveLabelsService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestChatServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &chat.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		Chat: func(_ context.Context, account string) (*chat.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := chatService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("chatService() error = %v", err)
	}
	if got != want {
		t.Fatalf("chatService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestFormsServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &formsapi.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		Forms: func(_ context.Context, account string) (*formsapi.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := formsService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("formsService() error = %v", err)
	}
	if got != want {
		t.Fatalf("formsService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestSlidesServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &slides.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		Slides: func(_ context.Context, account string) (*slides.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := slidesService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("slidesService() error = %v", err)
	}
	if got != want {
		t.Fatalf("slidesService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestRequireGmailServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &gmail.Service{}
	var factoryAccount string
	runtime := &app.Runtime{Services: app.Services{
		Gmail: func(_ context.Context, account string) (*gmail.Service, error) {
			factoryAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	gotAccount, got, err := requireGmailService(ctx, &RootFlags{Account: "test@example.com"})
	if err != nil {
		t.Fatalf("requireGmailService() error = %v", err)
	}
	if got != want {
		t.Fatalf("requireGmailService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("required account = %q, want test@example.com", gotAccount)
	}
	if factoryAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", factoryAccount)
	}
}

func TestPeopleContactsServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &people.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		PeopleContacts: func(_ context.Context, account string) (*people.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := peopleContactsService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("peopleContactsService() error = %v", err)
	}
	if got != want {
		t.Fatalf("peopleContactsService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestSheetsServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &sheets.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		Sheets: func(_ context.Context, account string) (*sheets.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := sheetsService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("sheetsService() error = %v", err)
	}
	if got != want {
		t.Fatalf("sheetsService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestSitesDriveServiceUsesRuntimeFactory(t *testing.T) {
	t.Parallel()

	want := &drive.Service{}
	var gotAccount string
	runtime := &app.Runtime{Services: app.Services{
		SitesDrive: func(_ context.Context, account string) (*drive.Service, error) {
			gotAccount = account
			return want, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	got, err := sitesDriveService(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("sitesDriveService() error = %v", err)
	}
	if got != want {
		t.Fatalf("sitesDriveService() = %p, want %p", got, want)
	}
	if gotAccount != "test@example.com" {
		t.Fatalf("factory account = %q, want test@example.com", gotAccount)
	}
}

func TestYouTubeServiceFactoriesUseRuntime(t *testing.T) {
	t.Setenv("GOG_YOUTUBE_API_KEY", "runtime-key")

	apiKeyService := &youtubeapi.Service{}
	accountService := &youtubeapi.Service{}
	commentsService := &youtubeapi.Service{}
	var gotAPIKey string
	var gotAccount string
	var gotCommentsAccount string
	runtime := &app.Runtime{Services: app.Services{
		YouTubeAPIKey: func(_ context.Context, key string) (*youtubeapi.Service, error) {
			gotAPIKey = key
			return apiKeyService, nil
		},
		YouTubeAccount: func(_ context.Context, account string) (*youtubeapi.Service, error) {
			gotAccount = account
			return accountService, nil
		},
		YouTubeComments: func(_ context.Context, account string) (*youtubeapi.Service, error) {
			gotCommentsAccount = account
			return commentsService, nil
		},
	}}
	ctx := app.WithRuntime(context.Background(), runtime)

	gotAPIKeyService, err := getYouTubeServiceWithAPIKey(ctx)
	if err != nil || gotAPIKeyService != apiKeyService || gotAPIKey != "runtime-key" {
		t.Fatalf("API key service = (%p, %v, %q)", gotAPIKeyService, err, gotAPIKey)
	}
	gotAccountService, err := getYouTubeServiceForAccount(ctx, "account@example.com")
	if err != nil || gotAccountService != accountService || gotAccount != "account@example.com" {
		t.Fatalf("account service = (%p, %v, %q)", gotAccountService, err, gotAccount)
	}
	gotCommentsService, err := getYouTubeCommentsServiceForAccount(ctx, "comments@example.com")
	if err != nil || gotCommentsService != commentsService || gotCommentsAccount != "comments@example.com" {
		t.Fatalf("comments service = (%p, %v, %q)", gotCommentsService, err, gotCommentsAccount)
	}
}

func TestDriveDownloadOperationsUseRuntimeServices(t *testing.T) {
	t.Parallel()

	svc := &drive.Service{}
	downloadResponse := &http.Response{Body: io.NopCloser(strings.NewReader(""))}
	exportResponse := &http.Response{Body: io.NopCloser(strings.NewReader(""))}
	var gotDownloadID string
	var gotExportID string
	var gotExportMIME string
	ctx := app.WithRuntime(context.Background(), &app.Runtime{Services: app.Services{
		DriveDownload: func(_ context.Context, gotSvc *drive.Service, fileID string) (*http.Response, error) {
			if gotSvc != svc {
				t.Fatalf("download service = %p, want %p", gotSvc, svc)
			}
			gotDownloadID = fileID
			return downloadResponse, nil
		},
		DriveExport: func(_ context.Context, gotSvc *drive.Service, fileID, mimeType string) (*http.Response, error) {
			if gotSvc != svc {
				t.Fatalf("export service = %p, want %p", gotSvc, svc)
			}
			gotExportID = fileID
			gotExportMIME = mimeType
			return exportResponse, nil
		},
	}})

	gotDownload, err := driveDownloadRequest(ctx, svc, "download-id")
	t.Cleanup(func() { _ = gotDownload.Body.Close() })
	if err != nil || gotDownload != downloadResponse || gotDownloadID != "download-id" {
		t.Fatalf("driveDownloadRequest() = (%p, %v, %q)", gotDownload, err, gotDownloadID)
	}
	gotExport, err := driveExportRequest(ctx, svc, "export-id", "application/pdf")
	t.Cleanup(func() { _ = gotExport.Body.Close() })
	if err != nil || gotExport != exportResponse || gotExportID != "export-id" || gotExportMIME != "application/pdf" {
		t.Fatalf("driveExportRequest() = (%p, %v, %q, %q)", gotExport, err, gotExportID, gotExportMIME)
	}
}

func TestCommandIOMergesPartialRuntime(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	ctx := app.WithRuntime(context.Background(), &app.Runtime{
		IO: app.IO{Out: &stdout},
	})

	got := commandIO(ctx)
	if got.Out != &stdout {
		t.Fatalf("stdout = %T, want injected buffer", got.Out)
	}
	if got.In == nil || got.Err == nil {
		t.Fatalf("commandIO() = %#v, want default input and stderr", got)
	}
}
