package secrets

import (
	"fmt"
	"time"

	"github.com/99designs/keyring"
)

type timeoutKeyring struct {
	inner   keyring.Keyring
	timeout time.Duration
	hint    string
}

func newTimeoutKeyring(inner keyring.Keyring, timeout time.Duration, hint string) keyring.Keyring {
	return &timeoutKeyring{
		inner:   inner,
		timeout: timeout,
		hint:    hint,
	}
}

func withKeyringTimeout[T any](timeout time.Duration, operation string, hint string, fn func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}

	ch := make(chan result, 1)

	go func() {
		value, err := fn()
		ch <- result{value: value, err: err}
	}()

	select {
	case res := <-ch:
		return res.value, res.err
	case <-time.After(timeout):
		var zero T
		return zero, keyringTimeoutError(operation, timeout, hint)
	}
}

func keyringTimeoutError(operation string, timeout time.Duration, hint string) error {
	return fmt.Errorf("%w after %v while %s (%s); "+
		"set GOG_KEYRING_BACKEND=file and GOG_KEYRING_PASSWORD=<password> to use encrypted file storage instead",
		errKeyringTimeout, timeout, operation, hint)
}

func (k *timeoutKeyring) Get(key string) (keyring.Item, error) {
	return withKeyringTimeout(k.timeout, "reading keyring item", k.hint, func() (keyring.Item, error) {
		return k.inner.Get(key)
	})
}

func (k *timeoutKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	return withKeyringTimeout(k.timeout, "reading keyring metadata", k.hint, func() (keyring.Metadata, error) {
		return k.inner.GetMetadata(key)
	})
}

func (k *timeoutKeyring) Set(item keyring.Item) error {
	_, err := withKeyringTimeout(k.timeout, "storing keyring item", k.hint, func() (struct{}, error) {
		return struct{}{}, k.inner.Set(item)
	})

	return err
}

func (k *timeoutKeyring) Remove(key string) error {
	_, err := withKeyringTimeout(k.timeout, "removing keyring item", k.hint, func() (struct{}, error) {
		return struct{}{}, k.inner.Remove(key)
	})

	return err
}

func (k *timeoutKeyring) Keys() ([]string, error) {
	return withKeyringTimeout(k.timeout, "listing keyring items", k.hint, k.inner.Keys)
}
