//nolint:wsl_v5
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const AppName = "gogcli"

var (
	errPathMustBeAbsolute = errors.New("path must be absolute")
	errUnknownPathKind    = errors.New("unknown path kind")
)

type PathKind int

const (
	PathKindConfig PathKind = iota
	PathKindData
	PathKindState
	PathKindCache
)

var homeOverride string

func SetHomeOverride(path string) (func(), error) {
	path = strings.TrimSpace(path)
	previous := homeOverride
	if path == "" {
		homeOverride = ""
		return func() { homeOverride = previous }, nil
	}
	expanded, err := ExpandPath(path)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(expanded) {
		return nil, fmt.Errorf("%w: GOG_HOME/--home=%s", errPathMustBeAbsolute, path)
	}
	homeOverride = expanded
	return func() { homeOverride = previous }, nil
}

func Dir() (string, error) {
	return kindDir(PathKindConfig)
}

func HasExplicitConfigOverride() bool {
	return strings.TrimSpace(homeOverride) != "" ||
		strings.TrimSpace(os.Getenv("GOG_HOME")) != "" ||
		strings.TrimSpace(os.Getenv("GOG_CONFIG_DIR")) != ""
}

func HasExplicitStateOverride() bool {
	return strings.TrimSpace(homeOverride) != "" ||
		strings.TrimSpace(os.Getenv("GOG_HOME")) != "" ||
		strings.TrimSpace(os.Getenv("GOG_STATE_DIR")) != ""
}

func HasExplicitDataOverride() bool {
	return strings.TrimSpace(homeOverride) != "" ||
		strings.TrimSpace(os.Getenv("GOG_HOME")) != "" ||
		strings.TrimSpace(os.Getenv("GOG_DATA_DIR")) != ""
}

func DataDir() (string, error) {
	return kindDir(PathKindData)
}

func StateDir() (string, error) {
	return kindDir(PathKindState)
}

func CacheDir() (string, error) {
	return kindDir(PathKindCache)
}

func kindDir(kind PathKind) (string, error) {
	if override, ok, err := gogKindOverride(kind); ok || err != nil {
		return override, err
	}
	if home, ok, err := gogHomeOverride(); ok || err != nil {
		return filepath.Join(home, kindName(kind)), err
	}
	if xdg, ok := absoluteEnv(xdgEnvVar(kind)); ok {
		return filepath.Join(xdg, AppName), nil
	}
	base, err := xdgDefaultBase(kind)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppName), nil
}

func gogKindOverride(kind PathKind) (string, bool, error) {
	env := gogKindEnvVar(kind)
	if env == "" {
		return "", false, nil
	}
	raw := strings.TrimSpace(os.Getenv(env))
	if raw == "" {
		return "", false, nil
	}
	expanded, err := ExpandPath(raw)
	if err != nil {
		return "", true, err
	}
	if !filepath.IsAbs(expanded) {
		return "", true, fmt.Errorf("%w: %s=%s", errPathMustBeAbsolute, env, raw)
	}
	return expanded, true, nil
}

func gogHomeOverride() (string, bool, error) {
	raw := strings.TrimSpace(homeOverride)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("GOG_HOME"))
	}
	if raw == "" {
		return "", false, nil
	}
	expanded, err := ExpandPath(raw)
	if err != nil {
		return "", true, err
	}
	if !filepath.IsAbs(expanded) {
		return "", true, fmt.Errorf("%w: GOG_HOME=%s", errPathMustBeAbsolute, raw)
	}
	return expanded, true, nil
}

func absoluteEnv(name string) (string, bool) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" || !filepath.IsAbs(value) {
		return "", false
	}
	return value, true
}

func hasAbsoluteEnv(name string) bool {
	_, ok := absoluteEnv(name)
	return ok
}

func xdgEnvVar(kind PathKind) string {
	switch kind {
	case PathKindConfig:
		return "XDG_CONFIG_HOME"
	case PathKindData:
		return "XDG_DATA_HOME"
	case PathKindState:
		return "XDG_STATE_HOME"
	case PathKindCache:
		return "XDG_CACHE_HOME"
	default:
		return ""
	}
}

func gogKindEnvVar(kind PathKind) string {
	switch kind {
	case PathKindConfig:
		return "GOG_CONFIG_DIR"
	case PathKindData:
		return "GOG_DATA_DIR"
	case PathKindState:
		return "GOG_STATE_DIR"
	case PathKindCache:
		return "GOG_CACHE_DIR"
	default:
		return ""
	}
}

func kindName(kind PathKind) string {
	switch kind {
	case PathKindConfig:
		return "config"
	case PathKindData:
		return "data"
	case PathKindState:
		return "state"
	case PathKindCache:
		return "cache"
	default:
		return ""
	}
}

func xdgDefaultBase(kind PathKind) (string, error) {
	switch kind {
	case PathKindConfig:
		return configDefaultBase()
	case PathKindCache:
		return cacheDefaultBase()
	case PathKindData:
		if usesXDGDefaults() {
			return homeJoin(".local", "share")
		}
		return configDefaultBase()
	case PathKindState:
		if usesXDGDefaults() {
			return homeJoin(".local", "state")
		}
		return configDefaultBase()
	default:
		return "", fmt.Errorf("%w: %d", errUnknownPathKind, kind)
	}
}

func configDefaultBase() (string, error) {
	if xdg, ok := absoluteEnv("XDG_CONFIG_HOME"); ok {
		return xdg, nil
	}
	if strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")) != "" && usesXDGDefaults() {
		return homeJoin(".config")
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return base, nil
}

func cacheDefaultBase() (string, error) {
	if strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")) != "" && usesXDGDefaults() {
		return homeJoin(".cache")
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	return base, nil
}

func usesXDGDefaults() bool {
	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		return true
	default:
		return false
	}
}

func homeJoin(parts ...string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}
	return filepath.Join(append([]string{home}, parts...)...), nil
}

func EnsureDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure config dir: %w", err)
	}

	return dir, nil
}

func EnsureDataDir() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure data dir: %w", err)
	}

	return dir, nil
}

func EnsureStateDir() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure state dir: %w", err)
	}

	return dir, nil
}

func BatchDir() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "batches"), nil
}

func EnsureBatchDir() (string, error) {
	dir, err := BatchDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure batch dir: %w", err)
	}

	return dir, nil
}

// KeyringDir is where the keyring "file" backend stores encrypted entries.
//
// We keep this separate from the main config dir because the file backend creates
// one file per key.
func KeyringDir() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}
	primary := filepath.Join(dataDir, "keyring")
	if explicitDataPath() {
		return primary, nil
	}

	legacy, legacyErr := legacyKeyringDir()
	if legacyErr != nil {
		return "", legacyErr
	}
	if st, legacyErr := os.Stat(legacy); legacyErr == nil && st.IsDir() {
		return legacy, nil
	}

	return primary, nil
}

func legacyKeyringDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "keyring"), nil
}

func explicitDataPath() bool {
	return HasExplicitDataOverride()
}

func EnsureKeyringDir() (string, error) {
	dir, err := KeyringDir()
	if err != nil {
		return "", err
	}
	// keyring's file backend uses 0700 by default; match that.

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure keyring dir: %w", err)
	}

	return dir, nil
}

func ClientCredentialsPath() (string, error) {
	return ClientCredentialsPathFor(DefaultClientName)
}

func ClientCredentialsPathFor(client string) (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return clientCredentialsPathInDir(dir, client)
}

func LegacyClientCredentialsPathFor(client string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return clientCredentialsPathInDir(dir, client)
}

func clientCredentialsPathInDir(dir string, client string) (string, error) {
	normalized, err := NormalizeClientNameOrDefault(client)
	if err != nil {
		return "", err
	}

	if normalized == DefaultClientName {
		return filepath.Join(dir, "credentials.json"), nil
	}

	return filepath.Join(dir, fmt.Sprintf("credentials-%s.json", normalized)), nil
}

func DriveDownloadsDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "drive-downloads"), nil
}

func EnsureDriveDownloadsDir() (string, error) {
	dir, err := DriveDownloadsDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure drive downloads dir: %w", err)
	}

	return dir, nil
}

func GmailAttachmentsDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "gmail-attachments"), nil
}

func EnsureGmailAttachmentsDir() (string, error) {
	dir, err := GmailAttachmentsDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure gmail attachments dir: %w", err)
	}

	return dir, nil
}

func GmailWatchDir() (string, error) {
	if !usesXDGDefaults() && !explicitStatePath() && !hasAbsoluteEnv("XDG_STATE_HOME") {
		return LegacyGmailWatchDir()
	}

	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	primary := filepath.Join(dir, "gmail-watch")
	if explicitStatePath() {
		return primary, nil
	}

	legacy, legacyErr := LegacyGmailWatchDir()
	if legacyErr != nil {
		return "", legacyErr
	}
	if _, primaryErr := os.Stat(primary); os.IsNotExist(primaryErr) {
		if st, legacyErr := os.Stat(legacy); legacyErr == nil && st.IsDir() {
			return legacy, nil
		}
	}
	return primary, nil
}

func LegacyGmailWatchDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "state", "gmail-watch"), nil
}

func explicitStatePath() bool {
	return HasExplicitStateOverride()
}

func KeepServiceAccountPath(email string) (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}

	safeEmail := base64.RawURLEncoding.EncodeToString([]byte(strings.ToLower(strings.TrimSpace(email))))

	return filepath.Join(dir, fmt.Sprintf("keep-sa-%s.json", safeEmail)), nil
}

func KeepServiceAccountLegacySafePath(email string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	safeEmail := base64.RawURLEncoding.EncodeToString([]byte(strings.ToLower(strings.TrimSpace(email))))

	return filepath.Join(dir, fmt.Sprintf("keep-sa-%s.json", safeEmail)), nil
}

func KeepServiceAccountLegacyPath(email string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, fmt.Sprintf("keep-sa-%s.json", email)), nil
}

func ServiceAccountPath(email string) (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}

	safeEmail := base64.RawURLEncoding.EncodeToString([]byte(strings.ToLower(strings.TrimSpace(email))))

	return filepath.Join(dir, fmt.Sprintf("sa-%s.json", safeEmail)), nil
}

func ServiceAccountLegacyPath(email string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	safeEmail := base64.RawURLEncoding.EncodeToString([]byte(strings.ToLower(strings.TrimSpace(email))))

	return filepath.Join(dir, fmt.Sprintf("sa-%s.json", safeEmail)), nil
}

func ExistingServiceAccountPath(email string) (string, error) {
	if HasExplicitDataOverride() {
		return firstExistingPath(ServiceAccountPath)(email)
	}
	return firstExistingPath(ServiceAccountPath, ServiceAccountLegacyPath)(email)
}

func ExistingKeepServiceAccountPath(email string) (string, error) {
	if HasExplicitDataOverride() {
		return firstExistingPath(KeepServiceAccountPath)(email)
	}
	return firstExistingPath(KeepServiceAccountPath, KeepServiceAccountLegacySafePath, KeepServiceAccountLegacyPath)(email)
}

func RemoveServiceAccountFiles(email string) (bool, error) {
	paths := make([]string, 0, 4)
	pathFns := []func(string) (string, error){
		ServiceAccountPath,
		KeepServiceAccountPath,
	}
	if !HasExplicitDataOverride() {
		pathFns = append(pathFns, ServiceAccountLegacyPath, KeepServiceAccountLegacySafePath)
	}
	for _, fn := range pathFns {
		path, err := fn(email)
		if err != nil {
			return false, fmt.Errorf("resolve service account path: %w", err)
		}
		paths = append(paths, path)
	}
	if !HasExplicitDataOverride() {
		if path, ok, err := keepServiceAccountLegacyDeletePath(email); err != nil {
			return false, err
		} else if ok {
			paths = append(paths, path)
		}
	}

	removed := false
	for _, path := range uniquePaths(paths...) {
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, fmt.Errorf("remove service account file: %w", err)
		}
		removed = true
	}
	return removed, nil
}

func keepServiceAccountLegacyDeletePath(email string) (string, bool, error) {
	if strings.ContainsAny(email, `/\`) {
		return "", false, nil
	}

	path, err := KeepServiceAccountLegacyPath(email)
	if err != nil {
		return "", false, fmt.Errorf("resolve service account path: %w", err)
	}

	dir, err := Dir()
	if err != nil {
		return "", false, fmt.Errorf("resolve service account path: %w", err)
	}

	cleanPath := filepath.Clean(path)
	base := filepath.Base(cleanPath)
	if filepath.Dir(cleanPath) != filepath.Clean(dir) || !strings.HasPrefix(base, "keep-sa-") || !strings.HasSuffix(base, ".json") {
		return "", false, nil
	}

	return cleanPath, true, nil
}

func firstExistingPath(fns ...func(string) (string, error)) func(string) (string, error) {
	return func(email string) (string, error) {
		var first string
		for _, fn := range fns {
			path, err := fn(email)
			if err != nil {
				return "", fmt.Errorf("resolve service account path: %w", err)
			}
			if first == "" {
				first = path
			}
			if _, statErr := os.Stat(path); statErr == nil {
				return path, nil
			} else if !os.IsNotExist(statErr) {
				return "", fmt.Errorf("stat service account path: %w", statErr)
			}
		}
		return first, nil
	}
}

func ListServiceAccountEmails() ([]string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return nil, err
	}

	out := make([]string, 0)
	seen := make(map[string]struct{})
	dirs := []string{dataDir}
	if !HasExplicitDataOverride() {
		configDir, err := Dir()
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, configDir)
	}
	for _, dir := range uniquePaths(dirs...) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read service account dir: %w", err)
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			email := ""

			switch {
			case strings.HasPrefix(name, "sa-") && strings.HasSuffix(name, ".json"):
				enc := strings.TrimSuffix(strings.TrimPrefix(name, "sa-"), ".json")
				if b, err := base64.RawURLEncoding.DecodeString(enc); err == nil {
					email = strings.TrimSpace(string(b))
				}
			case strings.HasPrefix(name, "keep-sa-") && strings.HasSuffix(name, ".json"):
				enc := strings.TrimSuffix(strings.TrimPrefix(name, "keep-sa-"), ".json")
				if b, err := base64.RawURLEncoding.DecodeString(enc); err == nil {
					email = strings.TrimSpace(string(b))
				} else {
					// Legacy (pre-safe-filename) format stored the raw email in the filename.
					email = strings.TrimSpace(enc)
				}
			default:
				continue
			}

			email = strings.ToLower(strings.TrimSpace(email))
			if email == "" {
				continue
			}
			if _, ok := seen[email]; ok {
				continue
			}
			seen[email] = struct{}{}
			out = append(out, email)
		}
	}

	sort.Strings(out)

	return out, nil
}

func EnsureGmailWatchDir() (string, error) {
	dir, err := GmailWatchDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure gmail watch dir: %w", err)
	}

	return dir, nil
}

func uniquePaths(paths ...string) []string {
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{})
	for _, path := range paths {
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

// ExpandPath expands ~ at the beginning of a path to the user's home directory.
// This is needed because ~ is a shell feature and is not expanded when paths
// are quoted (e.g., --out "~/Downloads/file.pdf").
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home dir: %w", err)
		}

		return home, nil
	}

	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home dir: %w", err)
		}

		return filepath.Join(home, strings.TrimLeft(path[2:], `/\`)), nil
	}

	return path, nil
}
