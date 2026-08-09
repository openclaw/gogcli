package secrets

import (
	"errors"
	"fmt"

	"github.com/99designs/keyring"

	"github.com/openclaw/gogcli/internal/config"
)

const defaultAccountKey = "default_account"

func defaultAccountKeyForClient(client string) string {
	return fmt.Sprintf("default_account:%s", client)
}

func (s *KeyringStore) GetDefaultAccount(client string) (string, error) {
	var account string

	err := s.withReadLock(func() error {
		var getErr error
		account, getErr = s.getDefaultAccountNoLock(client)

		return getErr
	})
	if err != nil {
		return "", err
	}

	return account, nil
}

func (s *KeyringStore) getDefaultAccountNoLock(client string) (string, error) {
	normalizedClient, err := normalizeClient(client)
	if err != nil {
		return "", err
	}

	if normalizedClient == config.DefaultClientName {
		if it, getErr := s.ring.Get(defaultAccountKeyForClient(normalizedClient)); getErr == nil {
			return string(it.Data), nil
		} else if !errors.Is(getErr, keyring.ErrKeyNotFound) {
			return "", fmt.Errorf("read default account: %w", getErr)
		}
	}

	if normalizedClient != config.DefaultClientName {
		if it, getErr := s.ring.Get(defaultAccountKeyForClient(normalizedClient)); getErr == nil {
			return string(it.Data), nil
		} else if !errors.Is(getErr, keyring.ErrKeyNotFound) {
			return "", fmt.Errorf("read default account: %w", getErr)
		}

		return "", nil
	}

	it, err := s.ring.Get(defaultAccountKey)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return "", nil
		}

		return "", fmt.Errorf("read default account: %w", err)
	}

	return string(it.Data), nil
}

func (s *KeyringStore) SetDefaultAccount(client string, email string) error {
	return s.withWriteLock(func() error {
		return s.setDefaultAccountNoLock(client, email)
	})
}

func (s *KeyringStore) setDefaultAccountNoLock(client string, email string) error {
	email = normalize(email)
	if email == "" {
		return errMissingEmail
	}

	normalizedClient, err := normalizeClient(client)
	if err != nil {
		return err
	}

	if normalizedClient != config.DefaultClientName {
		if err := verifiedSet(s.ring, defaultAccountKeyForClient(normalizedClient), []byte(email), "default account"); err != nil {
			return fmt.Errorf("store default account: %w", err)
		}

		return nil
	}

	if err := verifiedSet(s.ring, defaultAccountKeyForClient(normalizedClient), []byte(email), "default account"); err != nil {
		return fmt.Errorf("store default account: %w", err)
	}

	if err := verifiedSet(s.ring, defaultAccountKey, []byte(email), "legacy default account"); err != nil {
		return fmt.Errorf("store default account: %w", err)
	}

	return nil
}

func (s *KeyringStore) DeleteDefaultAccount(client string) error {
	return s.withWriteLock(func() error {
		return s.deleteDefaultAccountNoLock(client)
	})
}

func (s *KeyringStore) deleteDefaultAccountNoLock(client string) error {
	normalizedClient, err := normalizeClient(client)
	if err != nil {
		return err
	}

	if normalizedClient != config.DefaultClientName {
		if err := s.ring.Remove(defaultAccountKeyForClient(normalizedClient)); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
			return fmt.Errorf("delete default account: %w", err)
		}

		return nil
	}

	if err := s.ring.Remove(defaultAccountKeyForClient(normalizedClient)); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		return fmt.Errorf("delete default account: %w", err)
	}

	if err := s.ring.Remove(defaultAccountKey); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		return fmt.Errorf("delete default account: %w", err)
	}

	return nil
}
