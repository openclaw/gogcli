package secrets

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

const (
	keyringPasswordEnv          = "GOG_KEYRING_PASSWORD" //nolint:gosec // env var name, not a credential
	keyringBackendEnv           = "GOG_KEYRING_BACKEND"  //nolint:gosec // env var name, not a credential
	keyringServiceNameEnv       = "GOG_KEYRING_SERVICE_NAME"
	keyringOpenTimeoutEnv       = "GOG_KEYRING_OPEN_TIMEOUT"
	keychainTrustApplicationEnv = "GOG_KEYCHAIN_TRUST_APPLICATION"
)

var (
	errNoTTY                 = errors.New("no TTY available for keyring file backend password prompt")
	errInvalidKeyringBackend = errors.New("invalid keyring backend")
	errKeyringTimeout        = errors.New("keyring connection timed out")
	errNilConfigStore        = errors.New("config store is nil")
)

type KeyringBackendInfo struct {
	Value  string
	Source string
}

type OpenOptions struct {
	Layout                   config.Layout
	Config                   *config.ConfigStore
	Backend                  string
	Password                 string
	PasswordSet              bool
	ServiceName              string
	GOOS                     string
	DBusAddress              string
	IsTTY                    bool
	OpenTimeout              time.Duration
	LockTimeout              time.Duration
	KeychainTrustApplication string
	openKeyringFn            func(keyring.Config) (keyring.Keyring, error)
	codesignRunner           func(string) ([]byte, error)
}

const (
	keyringBackendSourceEnv     = "env"
	keyringBackendSourceConfig  = "config"
	keyringBackendSourceDefault = "default"
	keyringBackendAuto          = "auto"
	keyringBackendKeychain      = "keychain"
)

func OpenOptionsFromLookup(
	layout config.Layout,
	store *config.ConfigStore,
	lookup func(string) (string, bool),
	goos string,
	isTTY bool,
) OpenOptions {
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}

	backend, _ := lookup(keyringBackendEnv)
	password, passwordSet := lookup(keyringPasswordEnv)
	serviceName, _ := lookup(keyringServiceNameEnv)
	dbusAddress, _ := lookup("DBUS_SESSION_BUS_ADDRESS")
	openTimeoutRaw, _ := lookup(keyringOpenTimeoutEnv)
	lockTimeoutRaw, _ := lookup(keyringLockTimeoutEnv)
	keychainTrustApplication, _ := lookup(keychainTrustApplicationEnv)

	return OpenOptions{
		Layout:                   layout,
		Config:                   store,
		Backend:                  backend,
		Password:                 password,
		PasswordSet:              passwordSet,
		ServiceName:              strings.TrimSpace(serviceName),
		GOOS:                     goos,
		DBusAddress:              dbusAddress,
		IsTTY:                    isTTY,
		OpenTimeout:              parseKeyringOpenTimeout(openTimeoutRaw, goos),
		LockTimeout:              parseKeyringLockTimeout(lockTimeoutRaw),
		KeychainTrustApplication: keychainTrustApplication,
	}
}

func ResolveKeyringBackendInfoWithOptions(options OpenOptions) (KeyringBackendInfo, error) {
	if v := normalizeKeyringBackend(options.Backend); v != "" {
		return KeyringBackendInfo{Value: v, Source: keyringBackendSourceEnv}, nil
	}

	if options.Config == nil {
		return KeyringBackendInfo{}, errNilConfigStore
	}

	cfg, err := options.Config.Read()
	if err != nil {
		return KeyringBackendInfo{}, fmt.Errorf("resolve keyring backend: %w", err)
	}

	if cfg.KeyringBackend != "" {
		if v := normalizeKeyringBackend(cfg.KeyringBackend); v != "" {
			return KeyringBackendInfo{Value: v, Source: keyringBackendSourceConfig}, nil
		}
	}

	return KeyringBackendInfo{Value: keyringBackendAuto, Source: keyringBackendSourceDefault}, nil
}

func allowedBackends(info KeyringBackendInfo) ([]keyring.BackendType, error) {
	switch info.Value {
	case "", keyringBackendAuto:
		return nil, nil
	case keyringBackendKeychain:
		return []keyring.BackendType{keyring.KeychainBackend}, nil
	case "file":
		return []keyring.BackendType{keyring.FileBackend}, nil
	default:
		return nil, fmt.Errorf("%w: %q (expected %s, keychain, or file)", errInvalidKeyringBackend, info.Value, keyringBackendAuto)
	}
}

// wrapKeychainError wraps keychain errors with helpful guidance on macOS.
func wrapKeychainError(err error) error {
	if err == nil {
		return nil
	}

	if IsKeychainLockedError(err.Error()) {
		return fmt.Errorf("%w\n\nYour macOS keychain is locked. To unlock it, run:\n  security unlock-keychain ~/Library/Keychains/login.keychain-db", err)
	}

	return err
}

func fileKeyringPasswordFuncFrom(password string, passwordSet bool, isTTY bool) keyring.PromptFunc {
	// Treat "set to empty string" as intentional; empty passphrase is valid.
	if passwordSet {
		return keyring.FixedStringPrompt(password)
	}

	if isTTY {
		return keyring.TerminalPrompt
	}

	return func(_ string) (string, error) {
		return "", fmt.Errorf("%w; set %s", errNoTTY, keyringPasswordEnv)
	}
}

func normalizeKeyringBackend(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func serviceNameFor(options OpenOptions) string {
	if serviceName := strings.TrimSpace(options.ServiceName); serviceName != "" {
		return serviceName
	}

	return config.AppName
}

// Keyring timeouts guard against unresponsive backends. macOS gets longer for
// interactive permission prompts; other platforms retain the existing limit.
const (
	keyringOpenTimeout       = 10 * time.Second
	darwinKeyringOpenTimeout = 30 * time.Second
	goosDarwin               = "darwin"
	goosLinux                = "linux"
)

func defaultKeyringOpenTimeout(goos string) time.Duration {
	if goos == goosDarwin {
		return darwinKeyringOpenTimeout
	}

	return keyringOpenTimeout
}

// parseKeyringOpenTimeout resolves GOG_KEYRING_OPEN_TIMEOUT, falling back to
// the platform default when unset, unparseable, or non-positive.
func parseKeyringOpenTimeout(raw, goos string) time.Duration {
	fallback := defaultKeyringOpenTimeout(goos)
	if raw == "" {
		return fallback
	}

	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return fallback
	}

	return timeout
}

func shouldForceFileBackend(goos string, backendInfo KeyringBackendInfo, dbusAddr string) bool {
	return goos == goosLinux && backendInfo.Value == keyringBackendAuto && dbusAddr == ""
}

func shouldUseKeyringTimeout(goos string, backendInfo KeyringBackendInfo, dbusAddr string) bool {
	return goos == goosLinux && backendInfo.Value == keyringBackendAuto && dbusAddr != ""
}

func shouldUseKeyringOperationTimeout(goos string, backendInfo KeyringBackendInfo, dbusAddr string) bool {
	if goos == goosDarwin {
		return backendInfo.Value == keyringBackendAuto || backendInfo.Value == keyringBackendKeychain
	}

	return goos == goosLinux && backendInfo.Value == keyringBackendAuto && dbusAddr != ""
}

func keyringTimeoutHint(goos string) string {
	switch goos {
	case goosDarwin:
		return "macOS Keychain may be waiting for a permission prompt; run `gog auth list` from a terminal and click \"Always Allow\" when prompted"
	case goosLinux:
		return "D-Bus SecretService may be unresponsive"
	default:
		return "keyring backend may be unresponsive"
	}
}

func isFileKeyring(ring keyring.Keyring) bool {
	if ring == nil {
		return false
	}

	return reflect.TypeOf(ring).String() == "*keyring.fileKeyring"
}

func openKeyringWithOptions(options OpenOptions) (keyring.Keyring, error) {
	// On Linux/WSL/containers, OS keychains (secret-service/kwallet) may be unavailable.
	// In that case github.com/99designs/keyring falls back to the "file" backend,
	// which *requires* both a directory and a password prompt function.
	keyringDir, err := options.Layout.EnsureKeyringDir()
	if err != nil {
		return nil, fmt.Errorf("ensure keyring dir: %w", err)
	}

	backendInfo, err := ResolveKeyringBackendInfoWithOptions(options)
	if err != nil {
		return nil, err
	}

	backends, err := allowedBackends(backendInfo)
	if err != nil {
		return nil, err
	}
	wrapFileKeys := fileKeyringBackendOnly(backends)

	// On Linux with "auto" backend and no D-Bus session, force file backend.
	// Without DBUS_SESSION_BUS_ADDRESS, SecretService will hang indefinitely
	// trying to connect (common on headless systems like Raspberry Pi).
	if shouldForceFileBackend(options.GOOS, backendInfo, options.DBusAddress) {
		backends = []keyring.BackendType{keyring.FileBackend}
		wrapFileKeys = true
	}

	cfg := keyring.Config{
		ServiceName: serviceNameFor(options),
		// Trust application access only for binaries with a stable signing identity,
		// so Developer-ID releases keep access across upgrades. Ad-hoc/source builds
		// retain false because their designated requirement changes each build, the
		// Homebrew upgrade failure mode documented in issue #86.
		KeychainTrustApplication: ResolveKeychainTrustApplication(options, backendInfo).Enabled,
		AllowedBackends:          backends,
		FileDir:                  keyringDir,
		FilePasswordFunc:         fileKeyringPasswordFuncFrom(options.Password, options.PasswordSet, options.IsTTY),
	}

	openTimeout := options.OpenTimeout
	if openTimeout <= 0 {
		openTimeout = defaultKeyringOpenTimeout(options.GOOS)
	}

	open := options.openKeyringFn
	if open == nil {
		open = keyring.Open
	}

	// On Linux with D-Bus present, keyring.Open() can still hang if SecretService
	// is unresponsive (e.g., gnome-keyring installed but not running).
	// Use a timeout as a safety net.
	if shouldUseKeyringTimeout(options.GOOS, backendInfo, options.DBusAddress) {
		timeoutRing, timeoutErr := openKeyringWithTimeoutFunc(
			cfg,
			openTimeout,
			keyringTimeoutHint(options.GOOS),
			open,
		)
		if timeoutErr != nil {
			return nil, timeoutErr
		}

		return prepareKeyring(timeoutRing, backendInfo, wrapFileKeys, options), nil
	}

	ring, err := open(cfg)
	if err != nil {
		return nil, fmt.Errorf("open keyring: %w", err)
	}

	return prepareKeyring(ring, backendInfo, wrapFileKeys, options), nil
}

func prepareKeyring(
	ring keyring.Keyring,
	backendInfo KeyringBackendInfo,
	wrapFileKeys bool,
	options OpenOptions,
) keyring.Keyring {
	if wrapFileKeys || isFileKeyring(ring) {
		ring = newFileSafeKeyring(ring)
	}

	if shouldUseKeyringOperationTimeout(options.GOOS, backendInfo, options.DBusAddress) {
		timeout := options.OpenTimeout
		if timeout <= 0 {
			timeout = defaultKeyringOpenTimeout(options.GOOS)
		}
		ring = newTimeoutKeyring(ring, timeout, keyringTimeoutHint(options.GOOS))
	}

	return ring
}

type keyringResult struct {
	ring keyring.Keyring
	err  error
}

// openKeyringWithTimeoutFunc prevents an unresponsive SecretService open from
// blocking the CLI indefinitely. The worker goroutine may remain blocked until
// process exit after a timeout.
func openKeyringWithTimeoutFunc(
	cfg keyring.Config,
	timeout time.Duration,
	hint string,
	open func(keyring.Config) (keyring.Keyring, error),
) (keyring.Keyring, error) {
	ch := make(chan keyringResult, 1)

	go func() {
		ring, err := open(cfg)
		ch <- keyringResult{ring, err}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("open keyring: %w", res.err)
		}

		return res.ring, nil
	case <-time.After(timeout):
		return nil, keyringTimeoutError("opening keyring", timeout, hint)
	}
}

func Open(options OpenOptions) (Repository, error) {
	ring, err := openKeyringWithOptions(options)
	if err != nil {
		return nil, err
	}

	lock, _, err := keyringLockForRingInDir(ring, options.Layout.KeyringDir(), options.LockTimeout)
	if err != nil {
		return nil, err
	}

	return &KeyringStore{ring: ring, lock: lock}, nil
}
