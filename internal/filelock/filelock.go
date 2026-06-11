package filelock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var ErrTimeout = errors.New("file lock timeout")

var (
	sharedLocksMu sync.Mutex
	sharedLocks   = make(map[string]*Lock)
)

type Lock struct {
	path    string
	timeout time.Duration
	mu      sync.Mutex
}

func Shared(path string, timeout time.Duration) *Lock {
	path = filepath.Clean(path)

	sharedLocksMu.Lock()
	defer sharedLocksMu.Unlock()

	if existing := sharedLocks[path]; existing != nil {
		return existing
	}

	lock := &Lock{path: path, timeout: timeout}
	sharedLocks[path] = lock

	return lock
}

func (l *Lock) WithExclusive(fn func() error) error {
	if l == nil {
		return fn()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	defer file.Close()

	if err := l.acquire(file); err != nil {
		return err
	}

	fnErr := fn()
	unlockErr := unlockFile(file)

	if fnErr != nil {
		return fnErr
	}

	if unlockErr != nil {
		return fmt.Errorf("unlock file: %w", unlockErr)
	}

	return nil
}

func (l *Lock) acquire(file *os.File) error {
	timeout := l.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	deadline := time.Now().Add(timeout)

	for {
		err := lockFile(file)
		if err == nil {
			return nil
		}

		if !wouldBlock(err) {
			return fmt.Errorf("lock file: %w", err)
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("%w after %v: %s", ErrTimeout, timeout, l.path)
		}

		sleep := 10 * time.Millisecond
		if remaining < sleep {
			sleep = remaining
		}

		time.Sleep(sleep)
	}
}
