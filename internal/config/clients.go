//nolint:wsl_v5
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const DefaultClientName = "default"

var (
	errInvalidClientName = errors.New("invalid client name")
	errInvalidDomainName = errors.New("invalid domain name")
	errMissingEmail      = errors.New("missing email")
)

func NormalizeClientName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if name == "" {
		return "", fmt.Errorf("%w: empty", errInvalidClientName)
	}

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}

		return "", fmt.Errorf("%w: %q", errInvalidClientName, raw)
	}

	return name, nil
}

func NormalizeClientNameOrDefault(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return DefaultClientName, nil
	}

	return NormalizeClientName(raw)
}

func NormalizeDomain(raw string) (string, error) {
	domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), "@")
	if domain == "" {
		return "", fmt.Errorf("%w: empty", errInvalidDomainName)
	}

	if !strings.Contains(domain, ".") {
		return "", fmt.Errorf("%w: %q", errInvalidDomainName, raw)
	}

	for _, r := range domain {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}

		return "", fmt.Errorf("%w: %q", errInvalidDomainName, raw)
	}

	return domain, nil
}

func DomainFromEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return ""
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

func ResolveClientForAccount(cfg File, email string, override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return NormalizeClientNameOrDefault(override)
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if email != "" {
		if client, ok := cfg.AccountClients[email]; ok && strings.TrimSpace(client) != "" {
			return NormalizeClientNameOrDefault(client)
		}
	}

	domain := DomainFromEmail(email)
	if domain != "" {
		if client, ok := cfg.ClientDomains[domain]; ok && strings.TrimSpace(client) != "" {
			return NormalizeClientNameOrDefault(client)
		}

		if ok, err := ClientCredentialsExists(domain); err == nil && ok {
			if normalized, err := NormalizeClientName(domain); err == nil {
				return normalized, nil
			}
		}
	}

	return DefaultClientName, nil
}

func SetAccountClient(cfg *File, email string, client string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errMissingEmail
	}

	normalized, err := NormalizeClientNameOrDefault(client)
	if err != nil {
		return err
	}

	if cfg.AccountClients == nil {
		cfg.AccountClients = make(map[string]string)
	}
	cfg.AccountClients[email] = normalized

	return nil
}

func AccountClient(cfg File, email string) (string, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", false
	}

	client, ok := cfg.AccountClients[email]
	if !ok || strings.TrimSpace(client) == "" {
		return "", false
	}

	normalized, err := NormalizeClientNameOrDefault(client)
	if err != nil {
		return "", false
	}

	return normalized, true
}

func SetClientDomain(cfg *File, domain string, client string) error {
	normalizedDomain, err := NormalizeDomain(domain)
	if err != nil {
		return err
	}

	normalizedClient, err := NormalizeClientNameOrDefault(client)
	if err != nil {
		return err
	}

	if cfg.ClientDomains == nil {
		cfg.ClientDomains = make(map[string]string)
	}
	cfg.ClientDomains[normalizedDomain] = normalizedClient

	return nil
}

func ClientForDomain(cfg File, domain string) (string, bool) {
	normalizedDomain, err := NormalizeDomain(domain)
	if err != nil {
		return "", false
	}

	client, ok := cfg.ClientDomains[normalizedDomain]
	if !ok || strings.TrimSpace(client) == "" {
		return "", false
	}

	normalizedClient, err := NormalizeClientNameOrDefault(client)
	if err != nil {
		return "", false
	}

	return normalizedClient, true
}

type ClientCredentialsInfo struct {
	Client  string `json:"client"`
	Path    string `json:"path"`
	Default bool   `json:"default"`
}

func ListClientCredentials() ([]ClientCredentialsInfo, error) {
	dataDir, err := DataDir()
	if err != nil {
		return nil, err
	}

	out := make([]ClientCredentialsInfo, 0)
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

			return nil, fmt.Errorf("read credentials dir: %w", err)
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}

			name := e.Name()
			switch {
			case name == "credentials.json":
				if _, ok := seen[DefaultClientName]; ok {
					continue
				}
				seen[DefaultClientName] = struct{}{}
				out = append(out, ClientCredentialsInfo{
					Client:  DefaultClientName,
					Path:    filepath.Join(dir, name),
					Default: true,
				})
			case strings.HasPrefix(name, "credentials-") && strings.HasSuffix(name, ".json"):
				raw := strings.TrimSuffix(strings.TrimPrefix(name, "credentials-"), ".json")

				client, err := NormalizeClientName(raw)
				if err != nil {
					continue
				}
				if _, ok := seen[client]; ok {
					continue
				}
				seen[client] = struct{}{}

				out = append(out, ClientCredentialsInfo{
					Client:  client,
					Path:    filepath.Join(dir, name),
					Default: false,
				})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Client < out[j].Client })

	return out, nil
}
