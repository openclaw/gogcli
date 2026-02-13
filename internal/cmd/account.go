package cmd

import (
	"strings"

	"github.com/degree-analytics/ratatosk/internal/config"
	"github.com/degree-analytics/ratatosk/internal/secrets"
)

var openSecretsStoreForAccount = secrets.OpenDefault

func requireAccount(flags *RootFlags) (string, error) {
	client := config.DefaultClientName
	var err error
	if flags != nil {
		client, err = config.NormalizeClientNameOrDefault(flags.Client)
	}
	if err != nil {
		return "", err
	}
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
	if v := strings.TrimSpace(config.EnvWithFallback("RATA_ACCOUNT", "GOG_ACCOUNT")); v != "" {
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

	return "", usage("missing --account (or set RATA_ACCOUNT, set default via `rata auth manage`, or store exactly one token)")
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
