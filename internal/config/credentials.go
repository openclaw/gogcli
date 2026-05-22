//nolint:wsl_v5
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	errInvalidCredentials      = errors.New("invalid credentials.json (expected installed/web client_id and client_secret)")
	errMissingClientID         = errors.New("stored credentials.json is missing client_id/client_secret")
	errUnterminatedPlaceholder = errors.New("unterminated env placeholder")
	errUnsetEnvPlaceholder     = errors.New("environment variable is not set")
	errInvalidEnvPlaceholder   = errors.New("invalid env placeholder")
)

type ClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type ParseGoogleOAuthClientJSONOptions struct {
	ExpandEnv bool
}

type googleCredentialsFile struct {
	Installed *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"installed"`
	Web *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"web"`
}

func ParseGoogleOAuthClientJSON(b []byte) (ClientCredentials, error) {
	return ParseGoogleOAuthClientJSONWithOptions(b, ParseGoogleOAuthClientJSONOptions{})
}

func ParseGoogleOAuthClientJSONWithOptions(b []byte, opts ParseGoogleOAuthClientJSONOptions) (ClientCredentials, error) {
	var f googleCredentialsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return ClientCredentials{}, fmt.Errorf("decode credentials json: %w", err)
	}

	var clientID, clientSecret string
	if f.Installed != nil {
		clientID, clientSecret = f.Installed.ClientID, f.Installed.ClientSecret
	} else if f.Web != nil {
		clientID, clientSecret = f.Web.ClientID, f.Web.ClientSecret
	}

	if opts.ExpandEnv {
		var err error
		clientID, err = expandEnvPlaceholders(clientID)
		if err != nil {
			return ClientCredentials{}, fmt.Errorf("expand client_id: %w", err)
		}
		clientSecret, err = expandEnvPlaceholders(clientSecret)
		if err != nil {
			return ClientCredentials{}, fmt.Errorf("expand client_secret: %w", err)
		}
	}

	if clientID == "" || clientSecret == "" {
		return ClientCredentials{}, errInvalidCredentials
	}

	return ClientCredentials{ClientID: clientID, ClientSecret: clientSecret}, nil
}

func expandEnvPlaceholders(s string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(s); {
		start := strings.Index(s[i:], "${")
		if start < 0 {
			out.WriteString(s[i:])
			break
		}
		start += i
		out.WriteString(s[i:start])
		end := strings.IndexByte(s[start+2:], '}')
		if end < 0 {
			return "", errUnterminatedPlaceholder
		}
		end += start + 2
		expr := s[start+2 : end]
		name, fallback, hasFallback, err := parseEnvPlaceholder(expr)
		if err != nil {
			return "", err
		}
		if value, ok := os.LookupEnv(name); ok {
			out.WriteString(value)
		} else if hasFallback {
			out.WriteString(fallback)
		} else {
			return "", fmt.Errorf("%w: %s", errUnsetEnvPlaceholder, name)
		}
		i = end + 1
	}
	return out.String(), nil
}

func parseEnvPlaceholder(expr string) (name string, fallback string, hasFallback bool, err error) {
	name = expr
	if before, after, ok := strings.Cut(expr, ":-"); ok {
		name = before
		fallback = after
		hasFallback = true
	}
	if !validEnvName(name) {
		return "", "", false, fmt.Errorf("%w: %q", errInvalidEnvPlaceholder, expr)
	}
	return name, fallback, hasFallback, nil
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func WriteClientCredentials(c ClientCredentials) error {
	return WriteClientCredentialsFor(DefaultClientName, c)
}

func WriteClientCredentialsFor(client string, c ClientCredentials) error {
	_, err := EnsureDataDir()
	if err != nil {
		return fmt.Errorf("ensure data dir: %w", err)
	}

	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return fmt.Errorf("resolve credentials path: %w", err)
	}

	b, err := json.MarshalIndent(c, "", "  ") //nolint:gosec // required OAuth client credentials payload
	if err != nil {
		return fmt.Errorf("encode credentials json: %w", err)
	}

	b = append(b, '\n')

	if err := WriteFileAtomic(path, b, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	return nil
}

func WriteClientCredentialsMetadataFor(client string, c ClientCredentials) error {
	_, err := EnsureDataDir()
	if err != nil {
		return fmt.Errorf("ensure data dir: %w", err)
	}

	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return fmt.Errorf("resolve credentials path: %w", err)
	}

	metadata := struct {
		ClientID string `json:"client_id"`
	}{
		ClientID: c.ClientID,
	}
	b, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials metadata: %w", err)
	}
	b = append(b, '\n')

	if err := WriteFileAtomic(path, b, 0o600); err != nil {
		return fmt.Errorf("write credentials metadata: %w", err)
	}

	return nil
}

func ReadClientCredentials() (ClientCredentials, error) {
	return ReadClientCredentialsFor(DefaultClientName)
}

func ReadClientCredentialsFor(client string) (ClientCredentials, error) {
	c, err := ReadClientCredentialsMetadataFor(client)
	if err != nil {
		return ClientCredentials{}, err
	}
	if c.ClientSecret == "" {
		return ClientCredentials{}, errMissingClientID
	}
	return c, nil
}

func ReadClientCredentialsMetadataFor(client string) (ClientCredentials, error) {
	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return ClientCredentials{}, fmt.Errorf("resolve credentials path: %w", err)
	}
	var b []byte

	if b, err = os.ReadFile(path); err != nil { //nolint:gosec // user-provided path
		if os.IsNotExist(err) {
			if !HasExplicitDataOverride() {
				legacyPath, legacyErr := LegacyClientCredentialsPathFor(client)
				if legacyErr != nil {
					return ClientCredentials{}, fmt.Errorf("resolve legacy credentials path: %w", legacyErr)
				}
				if b, err = os.ReadFile(legacyPath); err == nil { //nolint:gosec // legacy path derived from user config dir
					// Continue with the legacy bytes; next write goes to the primary data path.
				} else if os.IsNotExist(err) {
					return ClientCredentials{}, &CredentialsMissingError{Path: path, Cause: err}
				}
			} else {
				return ClientCredentials{}, &CredentialsMissingError{Path: path, Cause: err}
			}
		}
		if err != nil {
			return ClientCredentials{}, fmt.Errorf("read credentials: %w", err)
		}
	}

	var c ClientCredentials
	if err := json.Unmarshal(b, &c); err != nil {
		return ClientCredentials{}, fmt.Errorf("decode credentials: %w", err)
	}

	if c.ClientID == "" {
		return ClientCredentials{}, errMissingClientID
	}

	return c, nil
}

func DeleteClientCredentialsFor(client string) error {
	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return fmt.Errorf("resolve credentials path: %w", err)
	}

	removed := false
	candidates := []string{path}
	if !HasExplicitDataOverride() {
		legacyPath, legacyErr := LegacyClientCredentialsPathFor(client)
		if legacyErr != nil {
			return fmt.Errorf("resolve legacy credentials path: %w", legacyErr)
		}
		candidates = append(candidates, legacyPath)
	}
	for _, candidate := range uniquePaths(candidates...) {
		if err := os.Remove(candidate); err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return fmt.Errorf("delete credentials: %w", err)
		}
		removed = true
	}
	if !removed {
		return &CredentialsMissingError{Path: path, Cause: os.ErrNotExist}
	}
	return nil
}

func ClientCredentialsExists(client string) (bool, error) {
	_, ok, err := ExistingClientCredentialsPathFor(client)
	return ok, err
}

func ExistingClientCredentialsPathFor(client string) (string, bool, error) {
	path, err := ClientCredentialsPathFor(client)
	if err != nil {
		return "", false, err
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if HasExplicitDataOverride() {
				return path, false, nil
			}
			legacyPath, legacyErr := LegacyClientCredentialsPathFor(client)
			if legacyErr != nil {
				return "", false, legacyErr
			}
			if legacyPath == path {
				return path, false, nil
			}
			if _, legacyStatErr := os.Stat(legacyPath); legacyStatErr != nil {
				if os.IsNotExist(legacyStatErr) {
					return path, false, nil
				}
				return "", false, fmt.Errorf("stat legacy credentials: %w", legacyStatErr)
			}
			return legacyPath, true, nil
		}

		return "", false, fmt.Errorf("stat credentials: %w", err)
	}

	return path, true, nil
}

type CredentialsMissingError struct {
	Path  string
	Cause error
}

func (e *CredentialsMissingError) Error() string {
	return "oauth credentials missing"
}

func (e *CredentialsMissingError) Unwrap() error {
	return e.Cause
}
