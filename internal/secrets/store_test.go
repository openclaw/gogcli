package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

var errKeyringOpenBlocked = errors.New("keyring open blocked")

// keyringConfig creates a keyring.Config for testing.
// KeychainTrustApplication is false to match production config (see store.go).
func keyringConfig(keyringDir string) keyring.Config {
	return keyring.Config{
		ServiceName:              config.AppName,
		KeychainTrustApplication: false,
		AllowedBackends:          []keyring.BackendType{keyring.FileBackend},
		FileDir:                  keyringDir,
		FilePasswordFunc:         fileKeyringPasswordFuncFrom("testpass", true, false),
	}
}

func TestKeyringServiceName(t *testing.T) {
	if got := serviceNameFor(OpenOptions{}); got != config.AppName {
		t.Fatalf("expected default service name %q, got %q", config.AppName, got)
	}

	if got := serviceNameFor(OpenOptions{ServiceName: " custom-gog "}); got != "custom-gog" {
		t.Fatalf("expected env service name, got %q", got)
	}
}

func TestOpenOptionsFromLookupCapturesEnvironment(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		keyringBackendEnv:          " file ",
		keyringPasswordEnv:         "",
		keyringServiceNameEnv:      " custom-gog ",
		"DBUS_SESSION_BUS_ADDRESS": "unix:path=/tmp/dbus",
		keyringLockTimeoutEnv:      "125ms",
	}
	options := OpenOptionsFromLookup(
		config.Layout{ConfigDir: "/config", DataDir: "/data"},
		config.NewConfigStore(config.Layout{ConfigDir: "/config"}),
		func(key string) (string, bool) {
			value, ok := values[key]
			return value, ok
		},
		"linux",
		true,
	)

	if options.Backend != " file " || options.ServiceName != "custom-gog" {
		t.Fatalf("options = %#v", options)
	}

	if options.Password != "" || !options.PasswordSet {
		t.Fatalf("empty password presence was not preserved: %#v", options)
	}

	if options.GOOS != "linux" || options.DBusAddress != "unix:path=/tmp/dbus" || !options.IsTTY {
		t.Fatalf("platform options = %#v", options)
	}

	if options.OpenTimeout != keyringOpenTimeout || options.LockTimeout != 125*time.Millisecond {
		t.Fatalf("timeouts = open %v lock %v", options.OpenTimeout, options.LockTimeout)
	}
}

func TestOpenOptionsFromLookupOpenTimeout(t *testing.T) {
	t.Parallel()

	values := map[string]string{keyringOpenTimeoutEnv: "45s"}
	options := OpenOptionsFromLookup(
		config.Layout{ConfigDir: "/config", DataDir: "/data"},
		config.NewConfigStore(config.Layout{ConfigDir: "/config"}),
		func(key string) (string, bool) {
			value, ok := values[key]
			return value, ok
		},
		"darwin",
		true,
	)

	if options.OpenTimeout != 45*time.Second {
		t.Fatalf("OpenTimeout = %v, want 45s", options.OpenTimeout)
	}
}

func TestParseKeyringOpenTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		goos string
		want time.Duration
	}{
		{name: "darwin default", goos: "darwin", want: darwinKeyringOpenTimeout},
		{name: "linux default", goos: "linux", want: keyringOpenTimeout},
		{name: "other default", goos: "windows", want: keyringOpenTimeout},
		{name: "valid duration overrides darwin", raw: "1m", goos: "darwin", want: time.Minute},
		{name: "valid duration overrides linux", raw: "45s", goos: "linux", want: 45 * time.Second},
		{name: "invalid uses darwin default", raw: "nonsense", goos: "darwin", want: darwinKeyringOpenTimeout},
		{name: "non-positive uses linux default", raw: "-5s", goos: "linux", want: keyringOpenTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := parseKeyringOpenTimeout(tt.raw, tt.goos); got != tt.want {
				t.Fatalf("parseKeyringOpenTimeout(%q, %q) = %v, want %v", tt.raw, tt.goos, got, tt.want)
			}
		})
	}
}

func TestOpenUsesInjectedOptions(t *testing.T) {
	t.Parallel()

	layout := config.Layout{ConfigDir: t.TempDir(), DataDir: t.TempDir()}
	var opened keyring.Config
	options := OpenOptions{
		Layout:      layout,
		Config:      config.NewConfigStore(layout),
		Backend:     "file",
		Password:    "pw",
		PasswordSet: true,
		ServiceName: "isolated",
		GOOS:        "linux",
		DBusAddress: "ignored",
		OpenTimeout: time.Second,
		LockTimeout: 250 * time.Millisecond,
		openKeyringFn: func(cfg keyring.Config) (keyring.Keyring, error) {
			opened = cfg
			return keyring.NewArrayKeyring(nil), nil
		},
	}

	repository, err := Open(options)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	store, ok := repository.(*KeyringStore)
	if !ok {
		t.Fatalf("repository = %T", repository)
	}

	if opened.ServiceName != "isolated" ||
		len(opened.AllowedBackends) != 1 ||
		opened.AllowedBackends[0] != keyring.FileBackend {
		t.Fatalf("keyring config = %#v", opened)
	}

	password, err := opened.FilePasswordFunc("prompt")
	if err != nil || password != "pw" {
		t.Fatalf("password = %q, err = %v", password, err)
	}

	if store.lock == nil || store.lock.path != filepath.Join(layout.KeyringDir(), keyringLockFilename) {
		t.Fatalf("lock = %#v", store.lock)
	}

	if store.lock.timeout != 250*time.Millisecond {
		t.Fatalf("lock timeout = %v", store.lock.timeout)
	}
}

func TestOpenKeepsRuntimeHomesIndependent(t *testing.T) {
	t.Parallel()

	open := func(root string) *KeyringStore {
		t.Helper()

		layout := config.Layout{
			ConfigDir: filepath.Join(root, "config"),
			DataDir:   filepath.Join(root, "data"),
		}

		repository, err := Open(OpenOptions{
			Layout:      layout,
			Config:      config.NewConfigStore(layout),
			Backend:     "file",
			PasswordSet: true,
			GOOS:        "linux",
			openKeyringFn: func(keyring.Config) (keyring.Keyring, error) {
				return keyring.NewArrayKeyring(nil), nil
			},
		})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}

		return repository.(*KeyringStore)
	}

	first := open(t.TempDir())
	second := open(t.TempDir())

	if first.lock == nil || second.lock == nil || first.lock.path == second.lock.path {
		t.Fatalf("locks must be independent: first=%#v second=%#v", first.lock, second.lock)
	}
}

func TestResolveKeyringBackendInfo_Default(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")

	layout := testSystemLayout(t, config.PathKindConfig)
	store := config.NewConfigStore(layout)

	info, err := ResolveKeyringBackendInfoWithOptions(systemTestOpenOptions(layout, store))
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfo: %v", err)
	}

	if info.Value != "auto" {
		t.Fatalf("expected auto, got %q", info.Value)
	}

	if info.Source != keyringBackendSourceDefault {
		t.Fatalf("expected source default, got %q", info.Source)
	}
}

func TestResolveKeyringBackendInfo_Config(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")

	layout := testSystemLayout(t, config.PathKindConfig)
	store := config.NewConfigStore(layout)
	path := store.Path()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte(`{ keyring_backend: "file" }`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	info, err := ResolveKeyringBackendInfoWithOptions(systemTestOpenOptions(layout, store))
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfo: %v", err)
	}

	if info.Value != "file" {
		t.Fatalf("expected file, got %q", info.Value)
	}

	if info.Source != keyringBackendSourceConfig {
		t.Fatalf("expected source config, got %q", info.Source)
	}
}

func TestResolveKeyringBackendInfo_EnvOverridesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "keychain")

	layout := testSystemLayout(t, config.PathKindConfig)
	store := config.NewConfigStore(layout)
	path := store.Path()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte(`{ keyring_backend: "file" }`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	info, err := ResolveKeyringBackendInfoWithOptions(systemTestOpenOptions(layout, store))
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfo: %v", err)
	}

	if info.Value != "keychain" {
		t.Fatalf("expected keychain, got %q", info.Value)
	}

	if info.Source != keyringBackendSourceEnv {
		t.Fatalf("expected source env, got %q", info.Source)
	}
}

func TestResolveKeyringBackendInfoUsesInjectedStore(t *testing.T) {
	t.Setenv("GOG_KEYRING_BACKEND", "")

	layout := config.Layout{ConfigDir: t.TempDir()}

	store := config.NewConfigStore(layout)
	if err := store.Write(config.File{KeyringBackend: "file"}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	info, err := ResolveKeyringBackendInfoWithOptions(OpenOptions{Layout: layout, Config: store})
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfoWithOptions: %v", err)
	}

	if info.Value != "file" || info.Source != keyringBackendSourceConfig {
		t.Fatalf("backend info = %#v, want file/config", info)
	}
}

func TestResolveKeyringBackendInfoWithOptionsUsesCapturedOverride(t *testing.T) {
	t.Parallel()

	layout := config.Layout{ConfigDir: t.TempDir()}
	store := config.NewConfigStore(layout)

	if err := store.Write(config.File{KeyringBackend: "file"}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	info, err := ResolveKeyringBackendInfoWithOptions(OpenOptions{
		Config:  store,
		Backend: "keychain",
	})
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfoWithOptions: %v", err)
	}

	if info.Value != "keychain" || info.Source != keyringBackendSourceEnv {
		t.Fatalf("backend info = %#v", info)
	}
}

func TestAllowedBackends_Invalid(t *testing.T) {
	_, err := allowedBackends(KeyringBackendInfo{Value: "nope"})
	if err == nil {
		t.Fatalf("expected error")
	}

	if !errors.Is(err, errInvalidKeyringBackend) {
		t.Fatalf("expected invalid backend error, got %v", err)
	}
}

func TestKeyringDbusGuards(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		backend     string
		dbusAddr    string
		wantForce   bool
		wantTimeout bool
	}{
		{
			name:        "linux auto no dbus",
			goos:        "linux",
			backend:     "auto",
			dbusAddr:    "",
			wantForce:   true,
			wantTimeout: false,
		},
		{
			name:        "linux auto with dbus",
			goos:        "linux",
			backend:     "auto",
			dbusAddr:    "unix:path=/run/user/1000/bus",
			wantForce:   false,
			wantTimeout: true,
		},
		{
			name:        "windows auto no dbus",
			goos:        "windows",
			backend:     "auto",
			dbusAddr:    "",
			wantForce:   false,
			wantTimeout: false,
		},
		{
			name:        "darwin auto no open timeout",
			goos:        "darwin",
			backend:     "auto",
			dbusAddr:    "",
			wantForce:   false,
			wantTimeout: false,
		},
		{
			name:        "linux explicit file no dbus",
			goos:        "linux",
			backend:     "file",
			dbusAddr:    "",
			wantForce:   false,
			wantTimeout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := KeyringBackendInfo{Value: tt.backend}
			if got := shouldForceFileBackend(tt.goos, info, tt.dbusAddr); got != tt.wantForce {
				t.Fatalf("shouldForceFileBackend=%v, want %v", got, tt.wantForce)
			}

			if got := shouldUseKeyringTimeout(tt.goos, info, tt.dbusAddr); got != tt.wantTimeout {
				t.Fatalf("shouldUseKeyringTimeout=%v, want %v", got, tt.wantTimeout)
			}
		})
	}
}

func TestOpenKeyringWithTimeout_Success(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "testpass")

	keyringDir, err := testSystemLayout(t, config.PathKindConfig, config.PathKindData).EnsureKeyringDir()
	if err != nil {
		t.Fatalf("EnsureKeyringDir: %v", err)
	}

	cfg := keyringConfig(keyringDir)

	// Should complete well within the timeout
	ring, err := openKeyringWithTimeoutFunc(cfg, 5*time.Second, keyringTimeoutHint(runtime.GOOS), keyring.Open)
	if err != nil {
		t.Fatalf("openKeyringWithTimeout: %v", err)
	}

	if ring == nil {
		t.Fatal("expected non-nil keyring")
	}
}

func TestOpenKeyringWithTimeout_Timeout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "testpass")

	keyringDir, err := testSystemLayout(t, config.PathKindConfig, config.PathKindData).EnsureKeyringDir()
	if err != nil {
		t.Fatalf("EnsureKeyringDir: %v", err)
	}

	cfg := keyringConfig(keyringDir)

	blockCh := make(chan struct{})
	open := func(_ keyring.Config) (keyring.Keyring, error) {
		<-blockCh
		return nil, errKeyringOpenBlocked
	}

	_, err = openKeyringWithTimeoutFunc(cfg, 10*time.Millisecond, keyringTimeoutHint(runtime.GOOS), open)

	close(blockCh)

	if err == nil {
		t.Fatalf("expected timeout error")
	}

	if !errors.Is(err, errKeyringTimeout) {
		t.Fatalf("expected keyring timeout error, got: %v", err)
	}

	if !strings.Contains(err.Error(), "GOG_KEYRING_BACKEND=file") {
		t.Fatalf("expected timeout error with GOG_KEYRING_BACKEND guidance, got: %v", err)
	}
}

func TestOpenKeyring_NoDBus_ForcesFileBackend(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("D-Bus detection only applies on Linux")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")        // auto
	t.Setenv("GOG_KEYRING_PASSWORD", "testpw") // for file backend
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")   // no D-Bus

	// Should succeed using file backend (not hang on D-Bus)
	store := openSystemTestStore(t)

	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestKeyringStoreSetToken_RoundtripPreservesServices(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	client := config.DefaultClientName

	tok := Token{
		Email:        "import@example.com",
		Services:     []string{"gmail", "drive"},
		RefreshToken: "imported-rt",
	}
	if err := store.SetToken(client, tok.Email, tok); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	got, err := store.GetToken(client, tok.Email)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}

	if got.Email != tok.Email {
		t.Fatalf("email mismatch: got %q want %q", got.Email, tok.Email)
	}

	if got.RefreshToken != tok.RefreshToken {
		t.Fatalf("refresh token mismatch: got %q want %q", got.RefreshToken, tok.RefreshToken)
	}

	if strings.Join(got.Services, ",") != "gmail,drive" {
		t.Fatalf("services mismatch: got %v", got.Services)
	}

	if got.CreatedAt.IsZero() {
		t.Fatalf("expected CreatedAt to be auto-populated")
	}
}

func TestKeyringStoreSetToken_OverwritesExistingEntry(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	store := &KeyringStore{ring: ring}
	client := config.DefaultClientName
	email := "overwrite@example.com"

	if err := store.SetToken(client, email, Token{RefreshToken: "rt-old"}); err != nil {
		t.Fatalf("SetToken old: %v", err)
	}

	if err := store.SetToken(client, email, Token{RefreshToken: "rt-new"}); err != nil {
		t.Fatalf("SetToken new: %v", err)
	}

	got, err := store.GetToken(client, email)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}

	if got.RefreshToken != "rt-new" {
		t.Fatalf("expected overwritten token, got %q", got.RefreshToken)
	}
}

func TestOpenKeyring_ExplicitBackend_IgnoresDBusDetection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file") // explicit file
	t.Setenv("GOG_KEYRING_PASSWORD", "testpw")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "") // no D-Bus (shouldn't matter)

	// Should succeed with explicit file backend
	store := openSystemTestStore(t)

	if store == nil {
		t.Fatal("expected non-nil store")
	}
}
