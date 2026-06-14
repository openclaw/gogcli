package gmailwatch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/filelock"
)

const defaultLockTimeout = 5 * time.Second

var (
	ErrMissingPath = errors.New("missing watch state path")
	ErrNotFound    = errors.New("watch state not found; run gmail watch start")
)

type Options struct {
	Now         func() time.Time
	WriteFile   func(string, []byte, os.FileMode) error
	LockTimeout time.Duration
}

type Repository struct {
	path      string
	lock      *filelock.Lock
	mu        sync.Mutex
	state     State
	now       func() time.Time
	writeFile func(string, []byte, os.FileMode) error
	memory    bool
}

func New(path string, options Options) *Repository {
	now := options.Now
	if now == nil {
		now = time.Now
	}

	writeFile := options.WriteFile
	if writeFile == nil {
		writeFile = config.WriteFileAtomic
	}

	lockTimeout := options.LockTimeout
	if lockTimeout <= 0 {
		lockTimeout = defaultLockTimeout
	}

	repository := &Repository{
		path:      path,
		now:       now,
		writeFile: writeFile,
	}
	if path != "" {
		repository.lock = filelock.Shared(filepath.Join(filepath.Dir(path), ".lock"), lockTimeout)
	}

	return repository
}

func NewMemory(state State, options Options) *Repository {
	repository := New("", options)
	repository.memory = true
	repository.state = cloneState(state)

	return repository
}

func Load(path string, options Options) (*Repository, error) {
	repository := New(path, options)

	err := repository.lock.WithExclusive(func() error {
		state, err := repository.readUnlocked()
		if err != nil {
			return err
		}

		repository.mu.Lock()
		repository.state = state
		repository.mu.Unlock()

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load watch state: %w", err)
	}

	return repository, nil
}

func ReadOptional(paths ...string) (State, bool, error) {
	for _, path := range paths {
		data, err := os.ReadFile(path) //nolint:gosec // paths are selected by the runtime layout adapter.
		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		if err != nil {
			return State{}, false, fmt.Errorf("read watch state: %w", err)
		}

		var state State
		if err := json.Unmarshal(data, &state); err != nil {
			return State{}, false, fmt.Errorf("decode watch state: %w", err)
		}

		return cloneState(state), true, nil
	}

	return State{}, false, nil
}

func (r *Repository) Path() string {
	return r.path
}

func (r *Repository) Get() State {
	r.mu.Lock()
	defer r.mu.Unlock()

	return cloneState(r.state)
}

func (r *Repository) Update(fn func(*State) error) error {
	err := r.lock.WithExclusive(func() error {
		state, err := r.readUnlocked()
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}

		candidate := cloneState(state)
		if err := fn(&candidate); err != nil {
			return err
		}

		if err := r.writeUnlocked(candidate); err != nil {
			return err
		}

		r.mu.Lock()
		r.state = cloneState(candidate)
		r.mu.Unlock()

		return nil
	})
	if err != nil {
		return fmt.Errorf("update watch state: %w", err)
	}

	return nil
}

func (r *Repository) Save() error {
	err := r.lock.WithExclusive(func() error {
		r.mu.Lock()
		state := cloneState(r.state)
		r.mu.Unlock()

		return r.writeUnlocked(state)
	})
	if err != nil {
		return fmt.Errorf("save watch state: %w", err)
	}

	return nil
}

func (r *Repository) Remove() error {
	err := r.lock.WithExclusive(func() error {
		if r.path == "" {
			return nil
		}

		if err := os.Remove(r.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove watch state: %w", err)
		}

		r.mu.Lock()
		r.state = State{}
		r.mu.Unlock()

		return nil
	})
	if err != nil {
		return fmt.Errorf("delete watch state: %w", err)
	}

	return nil
}

func (r *Repository) StartHistoryID(pushHistory string) (uint64, error) {
	var startID uint64

	err := r.lock.WithExclusive(func() error {
		state, err := r.readUnlocked()
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}

		pushID, pushOK, pushErr := parseHistoryIDOptional(pushHistory)
		if state.HistoryID == "" {
			if !pushOK {
				return pushErr
			}

			if pushErr != nil {
				return pushErr
			}

			state.HistoryID = FormatHistoryID(pushID)
			state.UpdatedAtMs = r.now().UnixMilli()

			if err := r.writeUnlocked(state); err != nil {
				return err
			}

			r.mu.Lock()
			r.state = cloneState(state)
			r.mu.Unlock()
			startID = pushID

			return nil
		}

		storedID, storedOK, parseErr := parseHistoryIDOptional(state.HistoryID)
		if parseErr != nil {
			return parseErr
		}

		if !storedOK {
			return nil
		}

		if pushErr != nil || !pushOK {
			startID = storedID

			return nil //nolint:nilerr // A malformed optional push falls back to the valid stored cursor.
		}

		if pushID > storedID {
			startID = storedID
		}

		r.mu.Lock()
		r.state = cloneState(state)
		r.mu.Unlock()

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("select watch history: %w", err)
	}

	return startID, nil
}

func (r *Repository) readUnlocked() (State, error) {
	if r.memory {
		r.mu.Lock()
		defer r.mu.Unlock()

		return cloneState(r.state), nil
	}

	if r.path == "" {
		return State{}, ErrNotFound
	}

	data, err := os.ReadFile(r.path)

	if errors.Is(err, os.ErrNotExist) {
		return State{}, ErrNotFound
	}

	if err != nil {
		return State{}, fmt.Errorf("read watch state: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode watch state: %w", err)
	}

	return state, nil
}

func (r *Repository) writeUnlocked(state State) error {
	if r.memory {
		return nil
	}

	if r.path == "" {
		return ErrMissingPath
	}

	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode watch state: %w", err)
	}

	if err := r.writeFile(r.path, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("write watch state: %w", err)
	}

	return nil
}
