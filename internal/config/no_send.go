package config

import (
	"sort"
	"strings"
)

// IsNoSendAccount reports whether send is blocked for the given account.
func IsNoSendAccount(cfg File, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))

	if email == "" {
		return false
	}

	return cfg.NoSendAccounts[email]
}

// SetNoSendAccount adds or removes an account from the no-send list.
func SetNoSendAccount(cfg *File, email string, block bool) error {
	email = strings.ToLower(strings.TrimSpace(email))

	if email == "" {
		return errMissingEmail
	}

	if block {
		if cfg.NoSendAccounts == nil {
			cfg.NoSendAccounts = make(map[string]bool)
		}

		cfg.NoSendAccounts[email] = true
	} else {
		delete(cfg.NoSendAccounts, email)

		if len(cfg.NoSendAccounts) == 0 {
			cfg.NoSendAccounts = nil
		}
	}

	return nil
}

// ListNoSendAccounts returns sorted email addresses with send blocked.
func ListNoSendAccounts(cfg File) []string {
	out := make([]string, 0, len(cfg.NoSendAccounts))

	for email, blocked := range cfg.NoSendAccounts {
		if blocked {
			out = append(out, email)
		}
	}

	sort.Strings(out)

	return out
}
