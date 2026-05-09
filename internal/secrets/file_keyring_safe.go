package secrets

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/99designs/keyring"
)

const fileKeyPrefix = "_gogcli_key_v1_"

var isInvalidFileKeyError = defaultIsInvalidFileKeyError

type fileSafeKeyring struct {
	inner keyring.Keyring
}

func newFileSafeKeyring(inner keyring.Keyring) keyring.Keyring {
	return &fileSafeKeyring{inner: inner}
}

func fileKeyringBackendOnly(backends []keyring.BackendType) bool {
	return len(backends) == 1 && backends[0] == keyring.FileBackend
}

func fileSafeKey(key string) string {
	return fileKeyPrefix + base64.RawURLEncoding.EncodeToString([]byte(key))
}

func decodeFileSafeKey(key string) string {
	encoded, ok := strings.CutPrefix(key, fileKeyPrefix)
	if !ok {
		return key
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return key
	}

	decoded := string(raw)
	if fileSafeKey(decoded) != key {
		return key
	}

	return decoded
}

func fileKeyNotFound(err error) bool {
	return errors.Is(err, keyring.ErrKeyNotFound) ||
		errors.Is(err, fs.ErrNotExist) ||
		isInvalidFileKeyError(err)
}

func (k *fileSafeKeyring) Get(key string) (keyring.Item, error) {
	item, err := k.inner.Get(fileSafeKey(key))
	if err == nil {
		item.Key = key
		return item, nil
	}

	if !fileKeyNotFound(err) {
		return keyring.Item{}, fmt.Errorf("read encoded file keyring item: %w", err)
	}

	item, legacyErr := k.inner.Get(key)
	if legacyErr != nil {
		if fileKeyNotFound(legacyErr) {
			return keyring.Item{}, keyring.ErrKeyNotFound
		}

		return keyring.Item{}, fmt.Errorf("read legacy file keyring item: %w", legacyErr)
	}

	item.Key = key

	return item, nil
}

func (k *fileSafeKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	meta, err := k.inner.GetMetadata(fileSafeKey(key))
	if err == nil {
		return meta, nil
	}

	if !fileKeyNotFound(err) {
		return keyring.Metadata{}, fmt.Errorf("read encoded file keyring metadata: %w", err)
	}

	meta, legacyErr := k.inner.GetMetadata(key)
	if legacyErr != nil {
		if fileKeyNotFound(legacyErr) {
			return keyring.Metadata{}, keyring.ErrKeyNotFound
		}

		return keyring.Metadata{}, fmt.Errorf("read legacy file keyring metadata: %w", legacyErr)
	}

	return meta, nil
}

func (k *fileSafeKeyring) Set(item keyring.Item) error {
	item.Key = fileSafeKey(item.Key)
	if err := k.inner.Set(item); err != nil {
		return fmt.Errorf("store file keyring item: %w", err)
	}

	return nil
}

func (k *fileSafeKeyring) Remove(key string) error {
	encodedErr := k.inner.Remove(fileSafeKey(key))
	legacyErr := k.inner.Remove(key)

	if encodedErr == nil || legacyErr == nil {
		return nil
	}

	if fileKeyNotFound(encodedErr) && fileKeyNotFound(legacyErr) {
		return keyring.ErrKeyNotFound
	}

	if !fileKeyNotFound(encodedErr) {
		return fmt.Errorf("remove encoded file keyring item: %w", encodedErr)
	}

	return fmt.Errorf("remove legacy file keyring item: %w", legacyErr)
}

func (k *fileSafeKeyring) Keys() ([]string, error) {
	keys, err := k.inner.Keys()
	if err != nil {
		return nil, fmt.Errorf("list file keyring keys: %w", err)
	}

	out := make([]string, 0, len(keys))

	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		decoded := decodeFileSafeKey(key)
		if _, ok := seen[decoded]; ok {
			continue
		}
		seen[decoded] = struct{}{}
		out = append(out, decoded)
	}

	sort.Strings(out)

	return out, nil
}
