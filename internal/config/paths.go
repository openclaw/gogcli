package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const AppName = "gogcli"

func Dir() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, AppName), nil
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	return filepath.Join(base, AppName), nil
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

// KeyringDir is where the keyring "file" backend stores encrypted entries.
//
// We keep this separate from the main config dir because the file backend creates
// one file per key.
func KeyringDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "keyring"), nil
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
	dir, err := Dir()
	if err != nil {
		return "", err
	}

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
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "state", "gmail-watch"), nil
}

func KeepServiceAccountPath(email string) (string, error) {
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
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	safeEmail := base64.RawURLEncoding.EncodeToString([]byte(strings.ToLower(strings.TrimSpace(email))))

	return filepath.Join(dir, fmt.Sprintf("sa-%s.json", safeEmail)), nil
}

func ListServiceAccountEmails() ([]string, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read config dir: %w", err)
	}

	out := make([]string, 0, len(entries))

	seen := make(map[string]struct{})

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

	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home dir: %w", err)
		}

		return filepath.Join(home, path[2:]), nil
	}

	return path, nil
}
