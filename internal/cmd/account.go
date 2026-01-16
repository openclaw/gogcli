package cmd

import (
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

var openSecretsStoreForAccount = secrets.OpenDefault

func requireAccount(flags *RootFlags) (string, error) {
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

	if store, err := openSecretsStoreForAccount(); err == nil {
		if defaultEmail, err := store.GetDefaultAccount(); err == nil {
			defaultEmail = strings.TrimSpace(defaultEmail)
			if defaultEmail != "" {
				return defaultEmail, nil
			}
		}
		if toks, err := store.ListTokens(); err == nil {
			if len(toks) == 1 {
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
