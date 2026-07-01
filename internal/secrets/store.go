package secrets

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

type Store interface {
	Keys() ([]string, error)
	SetToken(client string, email string, tok Token) error
	GetToken(client string, email string) (Token, error)
	DeleteToken(client string, email string) error
	ListTokens() ([]Token, error)
	GetDefaultAccount(client string) (string, error)
	SetDefaultAccount(client string, email string) error
}

type SecretStore interface {
	SetSecret(key string, value []byte) error
	GetSecret(key string) ([]byte, error)
	DeleteSecret(key string) error
}

type Repository interface {
	Store
	SecretStore
}

type KeyringStore struct {
	ring keyring.Keyring
	lock *keyringLock
}

var errTokenVerifyFailed = errors.New("token verification failed: keyring wrote 0 bytes")

func keyringItem(key string, data []byte) keyring.Item {
	return keyring.Item{
		Key:   key,
		Data:  data,
		Label: config.AppName, // to show "gogcli" in security dialog instead of "" (empty string)
	}
}

func (s *KeyringStore) Keys() ([]string, error) {
	var keys []string

	err := s.withReadLock(func() error {
		var keysErr error
		keys, keysErr = s.keysNoLock()

		return keysErr
	})
	if err != nil {
		return nil, err
	}

	return keys, nil
}

func (s *KeyringStore) keysNoLock() ([]string, error) {
	keys, err := s.ring.Keys()
	if err != nil {
		return nil, fmt.Errorf("list keyring keys: %w", err)
	}

	if s.lock != nil {
		keys = withoutInternalKeyringKeys(keys)
	}

	return keys, nil
}

func withoutInternalKeyringKeys(keys []string) []string {
	out := keys[:0]
	for _, key := range keys {
		if key == keyringLockFilename {
			continue
		}
		out = append(out, key)
	}

	return out
}

func (s *KeyringStore) withReadLock(fn func() error) error {
	if s.lock == nil {
		return fn()
	}

	return s.lock.withReadLock(fn)
}

func (s *KeyringStore) withWriteLock(fn func() error) error {
	if s.lock == nil {
		return fn()
	}

	return s.lock.withWriteLock(fn)
}

func verifiedSet(ring keyring.Keyring, key string, data []byte, label string) error {
	item := keyringItem(key, data)

	if trusted, ok := ring.(trustedSetKeyring); ok {
		if err := trusted.SetTrusted(item); err != nil {
			return fmt.Errorf("set %s: %w", label, err)
		}

		return nil
	}

	if err := ring.Set(item); err != nil {
		return fmt.Errorf("set %s: %w", label, err)
	}

	item, err := ring.Get(key)
	if err != nil {
		return fmt.Errorf("%w: could not read back %s after write: %w\n\n"+
			"Workaround: switch to file-based keyring with: gog auth keyring file", errTokenVerifyFailed, label, err)
	}

	if !bytes.Equal(item.Data, data) {
		if len(item.Data) == 0 {
			return fmt.Errorf("%w\n\n"+
				"This usually happens when the macOS Keychain is locked in a headless environment.\n"+
				"Workaround: switch to file-based keyring with: gog auth keyring file", errTokenVerifyFailed)
		}

		return fmt.Errorf("%w: read-back mismatch for %s\n\n"+
			"Workaround: switch to file-based keyring with: gog auth keyring file", errTokenVerifyFailed, label)
	}

	return nil
}

type trustedSetKeyring interface {
	SetTrusted(keyring.Item) error
}

func verifiedSetAlias(ring keyring.Keyring, key string, data []byte, label string) error {
	if err := verifiedSet(ring, key, data, label); err != nil {
		if !isDuplicateKeyringItemError(err) {
			return err
		}

		if removeErr := ring.Remove(key); removeErr != nil && !errors.Is(removeErr, keyring.ErrKeyNotFound) {
			return fmt.Errorf("replace duplicate %s: remove stale item: %w", label, removeErr)
		}

		if retryErr := verifiedSet(ring, key, data, label); retryErr != nil {
			return fmt.Errorf("replace duplicate %s: %w", label, retryErr)
		}
	}

	return nil
}

func isDuplicateKeyringItemError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "-25299") ||
		strings.Contains(msg, "errsecduplicateitem") ||
		strings.Contains(msg, "specified item already exists")
}
