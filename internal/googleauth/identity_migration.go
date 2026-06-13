package googleauth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/secrets"
)

type EmailReferenceUpdater func(oldEmail, newEmail string) error

var errEmailReferenceUpdaterRequired = errors.New("email reference updater is required")

func MigrateStoredSubjectIdentity(store secrets.Store, updater EmailReferenceUpdater, client string, identity Identity) (string, error) {
	oldEmail, err := FindStoredSubjectIdentityEmail(store, client, identity)
	if err != nil || oldEmail == "" {
		return oldEmail, err
	}

	newEmail := normalizeEmail(identity.Email)
	if err := MigrateStoredEmailReferences(store, updater, client, oldEmail, newEmail); err != nil {
		return "", err
	}

	return oldEmail, nil
}

func FindStoredSubjectIdentityEmail(store secrets.Store, client string, identity Identity) (string, error) {
	subject := strings.TrimSpace(identity.Subject)
	newEmail := normalizeEmail(identity.Email)

	if subject == "" || newEmail == "" {
		return "", nil
	}

	tokens, err := tokensForSubjectMigration(store, client)
	if err != nil {
		// Subject migration is best-effort compatibility cleanup. A stale or
		// corrupted token must not make a freshly completed OAuth flow fail
		// before the new refresh token is saved.
		return "", nil //nolint:nilerr // best-effort cleanup must not block saving the new token
	}

	for _, tok := range tokens {
		if tok.Client != client || strings.TrimSpace(tok.Subject) != subject {
			continue
		}

		oldEmail := normalizeEmail(tok.Email)
		if oldEmail == "" || oldEmail == newEmail {
			continue
		}

		return oldEmail, nil
	}

	return "", nil
}

func tokensForSubjectMigration(store secrets.Store, client string) ([]secrets.Token, error) {
	keys, err := store.Keys()
	if err == nil && len(keys) > 0 {
		tokens := make([]secrets.Token, 0, len(keys))
		for _, key := range keys {
			keyClient, email, ok := secrets.ParseTokenKey(key)
			if !ok || keyClient != client || email == "" {
				continue
			}

			tok, getErr := store.GetToken(keyClient, email)
			if getErr != nil {
				continue
			}

			tokens = append(tokens, tok)
		}

		if len(tokens) > 0 {
			return tokens, nil
		}
	}

	tokens, listErr := store.ListTokens()
	if listErr != nil {
		return nil, fmt.Errorf("list tokens for subject migration: %w", listErr)
	}

	return tokens, nil
}

type tokenAliasDeleter interface {
	DeleteTokenAlias(client string, email string) error
}

func DeleteStoredEmailAlias(store secrets.Store, client string, email string) error {
	email = normalizeEmail(email)
	if email == "" {
		return nil
	}

	aliasDeleter, ok := store.(tokenAliasDeleter)
	if !ok {
		return nil
	}

	if err := aliasDeleter.DeleteTokenAlias(client, email); err != nil {
		return fmt.Errorf("delete stale token alias for %s: %w", email, err)
	}

	return nil
}

func MigrateStoredEmailReferences(store secrets.Store, updater EmailReferenceUpdater, client string, oldEmail string, newEmail string) error {
	oldEmail = normalizeEmail(oldEmail)
	newEmail = normalizeEmail(newEmail)

	if oldEmail == "" || newEmail == "" || oldEmail == newEmail {
		return nil
	}

	if updater == nil {
		return errEmailReferenceUpdaterRequired
	}

	if defaultEmail, getErr := store.GetDefaultAccount(client); getErr == nil && normalizeEmail(defaultEmail) == oldEmail {
		if setErr := store.SetDefaultAccount(client, newEmail); setErr != nil {
			return fmt.Errorf("set migrated default account: %w", setErr)
		}
	}

	if err := updater(oldEmail, newEmail); err != nil {
		return fmt.Errorf("update config for subject identity migration: %w", err)
	}

	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
