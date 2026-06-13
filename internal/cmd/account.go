package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/secrets"
)

const (
	accessTokenPlaceholderAccount = "access-token-user"
	adcPlaceholderAccount         = "adc"
	directAccessTokenWarning      = "Note: Using direct access token (expires in ~1 hour; no auto-refresh)" //nolint:gosec // user-facing warning text, not a credential
)

func requireAccount(flags *RootFlags) (string, error) {
	// In ADC mode the service account authenticates as itself — no user email
	// or keyring lookup is needed. We still accept --account/GOG_ACCOUNT as an
	// optional label (e.g. for logging), but it is not required.
	if googleapi.IsADCMode() {
		if v := flagAccount(flags); v != "" {
			if shouldAutoSelectAccount(v) {
				return adcPlaceholderAccount, nil
			}
			return v, nil
		}
		if v := strings.TrimSpace(os.Getenv("GOG_ACCOUNT")); v != "" {
			if shouldAutoSelectAccount(v) {
				return adcPlaceholderAccount, nil
			}
			return v, nil
		}
		return adcPlaceholderAccount, nil
	}

	client := config.DefaultClientName
	var err error
	if flags != nil {
		client, err = config.NormalizeClientNameOrDefault(flags.Client)
	}
	if err != nil {
		return "", err
	}
	if account, ok, err := configuredAccount(flags); err != nil {
		return "", err
	} else if ok {
		return finalizeRequiredAccount(flags, account), nil
	}

	if hasDirectAccessToken(flags) {
		return finalizeRequiredAccount(flags, accessTokenPlaceholderAccount), nil
	}

	if account, ok := inferredStoredAccount(client, flags); ok {
		return account, nil
	}

	return "", usage("missing --account (or set GOG_ACCOUNT, set default via `gog auth manage`, or store exactly one token)")
}

func configuredAccount(flags *RootFlags) (string, bool, error) {
	if candidate := flagAccount(flags); candidate != "" {
		account, ok, err := selectConfiguredAccount(flags, candidate)
		if err != nil {
			return "", false, err
		}
		return account, ok, nil
	}

	if candidate := strings.TrimSpace(os.Getenv("GOG_ACCOUNT")); candidate != "" {
		account, ok, err := selectConfiguredAccount(flags, candidate)
		if err != nil {
			return "", false, err
		}
		return account, ok, nil
	}

	return "", false, nil
}

func flagAccount(flags *RootFlags) string {
	if flags == nil {
		return ""
	}

	return strings.TrimSpace(flags.Account)
}

func selectConfiguredAccount(flags *RootFlags, value string) (string, bool, error) {
	if resolved, ok, err := resolveAccountAlias(flags, value); err != nil {
		return "", false, err
	} else if ok {
		return resolved, true, nil
	}

	value = strings.TrimSpace(value)
	if value == "" || shouldAutoSelectAccount(value) {
		return "", false, nil
	}

	return value, true, nil
}

func inferredStoredAccount(client string, flags *RootFlags) (string, bool) {
	store, err := openAccountSecretsStore(flags)
	if err != nil {
		return "", false
	}

	if defaultEmail, getErr := store.GetDefaultAccount(client); getErr == nil {
		if defaultEmail = strings.TrimSpace(defaultEmail); defaultEmail != "" {
			return defaultEmail, true
		}
	}

	tokens, err := store.ListTokens()
	if err != nil {
		return "", false
	}

	filtered := make([]secrets.Token, 0, len(tokens))
	for _, tok := range tokens {
		if strings.TrimSpace(tok.Email) == "" {
			continue
		}
		if tok.Client == client {
			filtered = append(filtered, tok)
		}
	}
	if len(filtered) == 1 {
		if email := strings.TrimSpace(filtered[0].Email); email != "" {
			return email, true
		}
	}
	if len(filtered) == 0 && len(tokens) == 1 {
		if email := strings.TrimSpace(tokens[0].Email); email != "" {
			return email, true
		}
	}

	return "", false
}

func openAccountSecretsStore(flags *RootFlags) (secrets.Store, error) {
	if flags != nil {
		if openStore := flags.authOperations.OpenSecretsStore; openStore != nil {
			return openStore()
		}
	}
	return secrets.OpenDefault()
}

func directAccessToken(flags *RootFlags) string {
	if flags == nil {
		return ""
	}

	return strings.TrimSpace(flags.AccessToken)
}

func hasDirectAccessToken(flags *RootFlags) bool {
	return directAccessToken(flags) != ""
}

func finalizeRequiredAccount(flags *RootFlags, account string) string {
	if hasDirectAccessToken(flags) {
		_, _ = fmt.Fprintln(accountDiagnostics(flags), directAccessTokenWarning)
	}

	return account
}

func accountDiagnostics(flags *RootFlags) io.Writer {
	if flags == nil || flags.diagnostics == nil {
		return io.Discard
	}
	return flags.diagnostics
}

func resolveAccountAlias(flags *RootFlags, value string) (string, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "@") || shouldAutoSelectAccount(value) {
		return "", false, nil
	}

	store, err := accountConfigStore(flags)
	if err != nil {
		return "", false, err
	}
	return store.ResolveAccountAlias(value)
}

func accountConfigStore(flags *RootFlags) (*config.ConfigStore, error) {
	if flags != nil && flags.configStoreResolver != nil {
		return flags.configStoreResolver()
	}
	return config.DefaultConfigStore()
}

func shouldAutoSelectAccount(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", eventTypeDefault:
		return true
	default:
		return false
	}
}
