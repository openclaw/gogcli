package googleauth

import (
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

func MigrateStoredSubjectIdentity(store secrets.Store, client string, identity Identity) (string, error) {
	subject := strings.TrimSpace(identity.Subject)
	newEmail := normalizeEmail(identity.Email)

	if subject == "" || newEmail == "" {
		return "", nil
	}

	tokens, err := store.ListTokens()
	if err != nil {
		return "", fmt.Errorf("list tokens for subject identity migration: %w", err)
	}

	for _, tok := range tokens {
		if tok.Client != client || strings.TrimSpace(tok.Subject) != subject {
			continue
		}

		oldEmail := normalizeEmail(tok.Email)
		if oldEmail == "" || oldEmail == newEmail {
			return "", nil
		}

		if err := store.DeleteToken(client, oldEmail); err != nil {
			return "", fmt.Errorf("delete stale token for %s: %w", oldEmail, err)
		}

		if defaultEmail, getErr := store.GetDefaultAccount(client); getErr == nil && normalizeEmail(defaultEmail) == oldEmail {
			if setErr := store.SetDefaultAccount(client, newEmail); setErr != nil {
				return "", fmt.Errorf("set migrated default account: %w", setErr)
			}
		}

		if err := migrateStoredSubjectConfig(oldEmail, newEmail); err != nil {
			return "", err
		}

		return oldEmail, nil
	}

	return "", nil
}

func migrateStoredSubjectConfig(oldEmail string, newEmail string) error {
	if err := config.UpdateConfig(func(cfg *config.File) error {
		for alias, target := range cfg.AccountAliases {
			if strings.EqualFold(target, oldEmail) {
				cfg.AccountAliases[alias] = newEmail
			}
		}

		if cfg.AccountClients != nil {
			if client, ok := cfg.AccountClients[oldEmail]; ok {
				cfg.AccountClients[newEmail] = client
				delete(cfg.AccountClients, oldEmail)
			}

			if client, ok := cfg.AccountClients[strings.ToLower(oldEmail)]; ok {
				cfg.AccountClients[newEmail] = client
				delete(cfg.AccountClients, strings.ToLower(oldEmail))
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("update config for subject identity migration: %w", err)
	}

	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
