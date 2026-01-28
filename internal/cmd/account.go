package cmd

import (
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

var openSecretsStoreForAccount = secrets.OpenDefault

// AccessTokenPlaceholderAccount is used when --access-token is provided but no --account.
// The actual account identity is embedded in the token itself.
const AccessTokenPlaceholderAccount = "access-token-user"

func requireAccount(flags *RootFlags) (string, error) {
	client := config.DefaultClientName
	var err error
	if flags != nil {
		client, err = config.NormalizeClientNameOrDefault(flags.Client)
	}
	if err != nil {
		return "", err
	}

	// If access token is provided, account is optional - use placeholder if not specified
	hasAccessToken := flags != nil && strings.TrimSpace(flags.AccessToken) != ""

	if v := strings.TrimSpace(flags.Account); v != "" {
		if resolved, ok, err := resolveAccountAlias(v); err != nil {
			return "", err
		} else if ok {
			return resolved, nil
		}
		if shouldAutoSelectAccount(v) {
			v = ""
		}
		if v != "" {
			return v, nil
		}
	}
	if v := strings.TrimSpace(os.Getenv("GOG_ACCOUNT")); v != "" {
		if resolved, ok, err := resolveAccountAlias(v); err != nil {
			return "", err
		} else if ok {
			return resolved, nil
		}
		if shouldAutoSelectAccount(v) {
			v = ""
		}
		if v != "" {
			return v, nil
		}
	}

	// When using access token, account resolution from keyring isn't needed
	if hasAccessToken {
		return AccessTokenPlaceholderAccount, nil
	}

	if store, err := openSecretsStoreForAccount(); err == nil {
		if defaultEmail, err := store.GetDefaultAccount(client); err == nil {
			defaultEmail = strings.TrimSpace(defaultEmail)
			if defaultEmail != "" {
				return defaultEmail, nil
			}
		}
		if toks, err := store.ListTokens(); err == nil {
			filtered := make([]secrets.Token, 0, len(toks))
			for _, tok := range toks {
				if strings.TrimSpace(tok.Email) == "" {
					continue
				}
				if tok.Client == client {
					filtered = append(filtered, tok)
				}
			}
			if len(filtered) == 1 {
				if v := strings.TrimSpace(filtered[0].Email); v != "" {
					return v, nil
				}
			}
			if len(filtered) == 0 && len(toks) == 1 {
				if v := strings.TrimSpace(toks[0].Email); v != "" {
					return v, nil
				}
			}
		}
	}

	return "", usage("missing --account (or set GOG_ACCOUNT, set default via `gog auth manage`, or store exactly one token)")
}

func resolveAccountAlias(value string) (string, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "@") || shouldAutoSelectAccount(value) {
		return "", false, nil
	}
	return config.ResolveAccountAlias(value)
}

func shouldAutoSelectAccount(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "default":
		return true
	default:
		return false
	}
}
