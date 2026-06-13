//nolint:wsl_v5
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const AppName = "gogcli"

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
	return currentLayoutDir(PathKindConfig)
}

func HasExplicitDataOverride() bool {
	return currentLayoutEnv().hasExplicit(PathKindData)
}

func DataDir() (string, error) {
	return currentLayoutDir(PathKindData)
}

func StateDir() (string, error) {
	return currentLayoutDir(PathKindState)
}

func CacheDir() (string, error) {
	return currentLayoutDir(PathKindCache)
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
	layout, err := currentLayoutFor(PathKindData)
	if err != nil {
		return "", err
	}

	return layout.EnsureDataDir()
}

// KeyringDir is where the keyring "file" backend stores encrypted entries.
//
// We keep this separate from the main config dir because the file backend creates
// one file per key.
func KeyringDir() (string, error) {
	layout, err := currentLayoutFor(PathKindConfig, PathKindData)
	if err != nil {
		return "", err
	}

	return layout.KeyringDir(), nil
}

func EnsureKeyringDir() (string, error) {
	layout, err := currentLayoutFor(PathKindConfig, PathKindData)
	if err != nil {
		return "", err
	}

	return layout.EnsureKeyringDir()
}

func ClientCredentialsPath() (string, error) {
	return ClientCredentialsPathFor(DefaultClientName)
}

func ClientCredentialsPathFor(client string) (string, error) {
	layout, err := currentLayoutFor(PathKindData)
	if err != nil {
		return "", err
	}
	return layout.ClientCredentialsPathFor(client)
}

func LegacyClientCredentialsPathFor(client string) (string, error) {
	layout, err := currentLayoutFor(PathKindConfig)
	if err != nil {
		return "", err
	}
	return layout.LegacyClientCredentialsPathFor(client)
}

func KeepServiceAccountPath(email string) (string, error) {
	layout, err := currentLayoutFor(PathKindData)
	if err != nil {
		return "", err
	}
	return layout.KeepServiceAccountPath(email), nil
}

func KeepServiceAccountLegacyPath(email string) (string, error) {
	layout, err := currentLayoutFor(PathKindConfig)
	if err != nil {
		return "", err
	}
	return layout.KeepServiceAccountLegacyPath(email), nil
}

func ServiceAccountPath(email string) (string, error) {
	layout, err := currentLayoutFor(PathKindData)
	if err != nil {
		return "", err
	}
	return layout.ServiceAccountPath(email), nil
}

func ServiceAccountLegacyPath(email string) (string, error) {
	layout, err := currentLayoutFor(PathKindConfig)
	if err != nil {
		return "", err
	}
	return layout.ServiceAccountLegacyPath(email), nil
}

func ExistingServiceAccountPath(email string) (string, error) {
	layout, err := currentLayoutFor(PathKindConfig, PathKindData)
	if err != nil {
		return "", err
	}

	return layout.ExistingServiceAccountPath(email)
}

func ExistingKeepServiceAccountPath(email string) (string, error) {
	layout, err := currentLayoutFor(PathKindConfig, PathKindData)
	if err != nil {
		return "", err
	}

	return layout.ExistingKeepServiceAccountPath(email)
}

func RemoveServiceAccountFiles(email string) (bool, error) {
	layout, err := currentLayoutFor(PathKindConfig, PathKindData)
	if err != nil {
		return false, err
	}

	return layout.RemoveServiceAccountFiles(email)
}

func ListServiceAccountEmails() ([]string, error) {
	layout, err := currentLayoutFor(PathKindConfig, PathKindData)
	if err != nil {
		return nil, err
	}

	return layout.ListServiceAccountEmails()
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
