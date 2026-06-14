package docsbatch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/filelock"
)

const (
	ServiceDocs        = "docs"
	defaultLockTimeout = 5 * time.Second
)

var (
	ErrEmptyRevision      = errors.New("document revision is empty")
	ErrRevisionChanged    = errors.New("document revision changed")
	ErrRequireEmpty       = errors.New("operation requires an empty batch")
	ErrTransactionDeleted = errors.New("batch transaction is already deleted")
	ErrInvalidID          = errors.New("invalid batch ID")
	ErrIdentityMismatch   = errors.New("batch identity mismatch")
	ErrNotFound           = errors.New("batch not found")
	ErrStoredIDMismatch   = errors.New("stored batch ID does not match filename")
)

type RequestEntry struct {
	AppendedAt time.Time       `json:"appended_at"`
	Command    string          `json:"command"`
	Request    json.RawMessage `json:"request"`
}

type State struct {
	BatchID            string         `json:"batch_id"`
	Name               string         `json:"name,omitempty"`
	Service            string         `json:"service"`
	DocumentID         string         `json:"doc_id"`
	Account            string         `json:"account"`
	Client             string         `json:"client"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	RequiredRevisionID string         `json:"required_revision_id,omitempty"`
	Requests           []RequestEntry `json:"requests"`
}

type Summary struct {
	BatchID    string    `json:"batch_id"`
	Name       string    `json:"name,omitempty"`
	Service    string    `json:"service"`
	DocumentID string    `json:"doc_id"`
	Account    string    `json:"account"`
	Client     string    `json:"client"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Requests   int       `json:"requests"`
}

type Identity struct {
	Service    string
	DocumentID string
	Account    string
	Client     string
}

type AppendOptions struct {
	BatchID      string
	Command      string
	Identity     Identity
	RevisionID   string
	Requests     []json.RawMessage
	RequireEmpty bool
}

type Options struct {
	Now         func() time.Time
	NewID       func() (string, error)
	WriteFile   func(string, []byte, os.FileMode) error
	LockTimeout time.Duration
}

type Repository struct {
	dir       string
	lock      *filelock.Lock
	now       func() time.Time
	newID     func() (string, error)
	writeFile func(string, []byte, os.FileMode) error
}

type Transaction struct {
	repository *Repository
	state      *State
	deleted    bool
}

func New(dir string, options Options) *Repository {
	now := options.Now
	if now == nil {
		now = time.Now
	}

	newID := options.NewID
	if newID == nil {
		newID = func() (string, error) {
			id, err := uuid.NewV7()
			if err != nil {
				return "", fmt.Errorf("new UUID v7: %w", err)
			}

			return id.String(), nil
		}
	}

	writeFile := options.WriteFile
	if writeFile == nil {
		writeFile = config.WriteFileAtomic
	}

	lockTimeout := options.LockTimeout
	if lockTimeout <= 0 {
		lockTimeout = defaultLockTimeout
	}

	return &Repository{
		dir:       dir,
		lock:      filelock.Shared(filepath.Join(dir, ".lock"), lockTimeout),
		now:       now,
		newID:     newID,
		writeFile: writeFile,
	}
}

func (r *Repository) Ensure() error {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return fmt.Errorf("ensure batch dir: %w", err)
	}

	return nil
}

func (r *Repository) Create(state State) (*State, error) {
	var created *State

	err := r.lock.WithExclusive(func() error {
		id, err := r.newID()
		if err != nil {
			return fmt.Errorf("create batch ID: %w", err)
		}

		if err := ValidateID(id); err != nil {
			return fmt.Errorf("create batch ID: %w", err)
		}

		now := r.now().UTC()
		state.BatchID = id
		state.CreatedAt = now
		state.UpdatedAt = now
		state.Requests = []RequestEntry{}

		if err := r.writeUnlocked(&state); err != nil {
			return err
		}
		created = &state

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}

	return created, nil
}

func (r *Repository) List() ([]Summary, error) {
	states, err := r.listStatesUnlocked()
	if err != nil {
		return nil, err
	}

	summaries := make([]Summary, 0, len(states))
	for _, state := range states {
		summaries = append(summaries, summarize(state))
	}

	return summaries, nil
}

func (r *Repository) Get(batchID string) (*State, error) {
	return r.readUnlocked(batchID)
}

func (r *Repository) Append(options AppendOptions) (int, error) {
	total := 0

	err := r.lock.WithExclusive(func() error {
		state, err := r.readUnlocked(options.BatchID)
		if err != nil {
			return err
		}

		if err := ValidateIdentity(state, options.Identity); err != nil {
			return err
		}

		if options.RevisionID == "" {
			return ErrEmptyRevision
		}

		if state.RequiredRevisionID != "" && state.RequiredRevisionID != options.RevisionID {
			return fmt.Errorf(
				"document revision changed since the first request was queued (batch=%s current=%s): %w",
				state.RequiredRevisionID,
				options.RevisionID,
				ErrRevisionChanged,
			)
		}

		if options.RequireEmpty && len(state.Requests) > 0 {
			return fmt.Errorf("this operation must be the first request in a batch: %w", ErrRequireEmpty)
		}

		now := r.now().UTC()
		for _, request := range options.Requests {
			state.Requests = append(state.Requests, RequestEntry{
				AppendedAt: now,
				Command:    options.Command,
				Request:    append(json.RawMessage(nil), request...),
			})
		}
		state.RequiredRevisionID = options.RevisionID
		state.UpdatedAt = now

		if err := r.writeUnlocked(state); err != nil {
			return err
		}
		total = len(state.Requests)

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("append batch: %w", err)
	}

	return total, nil
}

func (r *Repository) Delete(batchID string) (*State, error) {
	var deleted *State

	err := r.lock.WithExclusive(func() error {
		state, err := r.readUnlocked(batchID)
		if err != nil {
			return err
		}

		if err := os.Remove(r.path(state.BatchID)); err != nil {
			return fmt.Errorf("remove batch: %w", err)
		}
		deleted = state

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("delete batch: %w", err)
	}

	return deleted, nil
}

func (r *Repository) Prune(olderThan time.Duration) ([]Summary, error) {
	var removed []Summary

	err := r.lock.WithExclusive(func() error {
		states, err := r.listStatesUnlocked()
		if err != nil {
			return err
		}

		cutoff := r.now().UTC().Add(-olderThan)
		for _, state := range states {
			if state.UpdatedAt.After(cutoff) {
				continue
			}

			if err := os.Remove(r.path(state.BatchID)); err != nil {
				return fmt.Errorf("remove batch %s: %w", state.BatchID, err)
			}
			removed = append(removed, summarize(state))
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("prune batches: %w", err)
	}

	return removed, nil
}

func (r *Repository) WithState(batchID string, fn func(*Transaction) error) error {
	err := r.lock.WithExclusive(func() error {
		state, err := r.readUnlocked(batchID)
		if err != nil {
			return err
		}

		return fn(&Transaction{repository: r, state: state})
	})
	if err != nil {
		return fmt.Errorf("batch transaction: %w", err)
	}

	return nil
}

func (r *Repository) Path(batchID string) (string, error) {
	if err := ValidateID(batchID); err != nil {
		return "", err
	}

	return r.path(batchID), nil
}

func (t *Transaction) State() *State {
	return t.state
}

func (t *Transaction) PersistOrDelete() error {
	if t.deleted {
		return ErrTransactionDeleted
	}

	if len(t.state.Requests) == 0 {
		if err := os.Remove(t.repository.path(t.state.BatchID)); err != nil {
			return fmt.Errorf("remove completed batch: %w", err)
		}
		t.deleted = true

		return nil
	}

	t.state.UpdatedAt = t.repository.now().UTC()

	return t.repository.writeUnlocked(t.state)
}

func ValidateID(batchID string) error {
	batchID = strings.TrimSpace(batchID)

	parsed, err := uuid.Parse(batchID)
	if err != nil || parsed.String() != batchID {
		return fmt.Errorf("%w: %s", ErrInvalidID, batchID)
	}

	return nil
}

func ValidateIdentity(state *State, identity Identity) error {
	switch {
	case state.Service != identity.Service:
		return fmt.Errorf("batch service is %s, not %s: %w", state.Service, identity.Service, ErrIdentityMismatch)
	case state.DocumentID != identity.DocumentID:
		return fmt.Errorf("batch targets doc %s, not %s: %w", state.DocumentID, identity.DocumentID, ErrIdentityMismatch)
	case !strings.EqualFold(state.Account, identity.Account):
		return fmt.Errorf("batch uses account %s, not %s: %w", state.Account, identity.Account, ErrIdentityMismatch)
	case state.Client != identity.Client:
		return fmt.Errorf("batch uses OAuth client %s, not %s: %w", state.Client, identity.Client, ErrIdentityMismatch)
	default:
		return nil
	}
}

func summarize(state *State) Summary {
	return Summary{
		BatchID:    state.BatchID,
		Name:       state.Name,
		Service:    state.Service,
		DocumentID: state.DocumentID,
		Account:    state.Account,
		Client:     state.Client,
		CreatedAt:  state.CreatedAt,
		UpdatedAt:  state.UpdatedAt,
		Requests:   len(state.Requests),
	}
}

func (r *Repository) listStatesUnlocked() ([]*State, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*State{}, nil
		}

		return nil, fmt.Errorf("read batch directory: %w", err)
	}

	states := make([]*State, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(r.dir, entry.Name())

		state, readErr := r.readPathUnlocked(path)
		if readErr != nil {
			if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
				continue
			}

			return nil, readErr
		}

		states = append(states, state)
	}

	sort.Slice(states, func(i, j int) bool {
		return states[i].UpdatedAt.After(states[j].UpdatedAt)
	})

	return states, nil
}

func (r *Repository) readUnlocked(batchID string) (*State, error) {
	if err := ValidateID(batchID); err != nil {
		return nil, err
	}

	return r.readPathUnlocked(r.path(batchID))
}

func (r *Repository) readPathUnlocked(path string) (*State, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the private batch directory and a validated UUID.
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, strings.TrimSuffix(filepath.Base(path), ".json"))
		}

		return nil, fmt.Errorf("read batch: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode batch %s: %w", filepath.Base(path), err)
	}

	if err := ValidateID(state.BatchID); err != nil {
		return nil, fmt.Errorf("invalid stored batch: %w", err)
	}

	expectedID := strings.TrimSuffix(filepath.Base(path), ".json")
	if state.BatchID != expectedID {
		return nil, fmt.Errorf(
			"stored batch ID %s does not match file %s: %w",
			state.BatchID,
			expectedID,
			ErrStoredIDMismatch,
		)
	}

	return &state, nil
}

func (r *Repository) writeUnlocked(state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode batch: %w", err)
	}

	data = append(data, '\n')
	if err := r.writeFile(r.path(state.BatchID), data, 0o600); err != nil {
		return fmt.Errorf("write batch: %w", err)
	}

	return nil
}

func (r *Repository) path(batchID string) string {
	return filepath.Join(r.dir, batchID+".json")
}
