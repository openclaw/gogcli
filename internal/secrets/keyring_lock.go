package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/byteness/keyring"
)

const (
	keyringLockTimeoutEnv     = "GOG_KEYRING_LOCK_TIMEOUT"
	keyringLockFilename       = ".lock"
	defaultKeyringLockTimeout = 5 * time.Second
)

var (
	keyringLocksMu sync.Mutex
	keyringLocks   = make(map[string]*sync.RWMutex)
)

type keyringLock struct {
	path    string
	timeout time.Duration
	mu      *sync.RWMutex
}

func keyringLockForRingInDir(ring keyring.Keyring, dir string, timeout time.Duration) (*keyringLock, bool, error) {
	if !isFileBackedKeyring(ring) {
		return nil, false, nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, fmt.Errorf("ensure keyring lock dir: %w", err)
	}

	if timeout <= 0 {
		timeout = defaultKeyringLockTimeout
	}

	return sharedKeyringLock(filepath.Join(dir, keyringLockFilename), timeout), true, nil
}

func sharedKeyringLock(path string, timeout time.Duration) *keyringLock {
	path = filepath.Clean(path)

	keyringLocksMu.Lock()
	defer keyringLocksMu.Unlock()

	mu := keyringLocks[path]
	if mu == nil {
		mu = &sync.RWMutex{}
		keyringLocks[path] = mu
	}

	return &keyringLock{path: path, timeout: timeout, mu: mu}
}

func parseKeyringLockTimeout(raw string) time.Duration {
	if raw == "" {
		return defaultKeyringLockTimeout
	}

	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return defaultKeyringLockTimeout
	}

	return timeout
}

func isFileBackedKeyring(ring keyring.Keyring) bool {
	switch r := ring.(type) {
	case nil:
		return false
	case *fileSafeKeyring:
		return true
	case *timeoutKeyring:
		return isFileBackedKeyring(r.inner)
	default:
		return isFileKeyring(ring)
	}
}

func (l *keyringLock) withReadLock(fn func() error) error {
	if l == nil {
		return fn()
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.withFileLock(false, fn)
}

func (l *keyringLock) withWriteLock(fn func() error) error {
	if l == nil {
		return fn()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.withFileLock(true, fn)
}

func (l *keyringLock) withFileLock(exclusive bool, fn func() error) error {
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open keyring lock: %w", err)
	}
	defer file.Close()

	if err := l.acquireFileLock(file, exclusive); err != nil {
		return err
	}

	fnErr := fn()
	unlockErr := unlockKeyringFile(file)

	if fnErr != nil {
		return fnErr
	}

	if unlockErr != nil {
		return fmt.Errorf("unlock keyring: %w", unlockErr)
	}

	return nil
}

func (l *keyringLock) acquireFileLock(file *os.File, exclusive bool) error {
	timeout := l.timeout
	if timeout <= 0 {
		timeout = defaultKeyringLockTimeout
	}

	deadline := time.Now().Add(timeout)

	for {
		err := lockKeyringFile(file, exclusive)
		if err == nil {
			return nil
		}

		if !keyringLockWouldBlock(err) {
			return fmt.Errorf("lock keyring: %w", err)
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("%w after %v while locking keyring; set %s to adjust the wait",
				errKeyringTimeout, timeout, keyringLockTimeoutEnv)
		}

		sleep := 10 * time.Millisecond
		if remaining < sleep {
			sleep = remaining
		}

		time.Sleep(sleep)
	}
}
