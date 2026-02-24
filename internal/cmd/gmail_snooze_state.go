package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/steipete/gogcli/internal/config"
)

type snoozeEntry struct {
	ThreadID       string `json:"threadId"`
	Subject        string `json:"subject"`
	Account        string `json:"account"`
	UntilMs        int64  `json:"untilMs"`
	SnoozedLabelID string `json:"snoozedLabelId"`
	CreatedAtMs    int64  `json:"createdAtMs"`
}

type snoozeState struct {
	Entries []snoozeEntry `json:"entries"`
}

type snoozeStore struct {
	path  string
	mu    sync.Mutex
	state snoozeState
}

func gmailSnoozeStatePath(account string) (string, error) {
	dir, err := config.EnsureGmailSnoozeDir()
	if err != nil {
		return "", err
	}
	name := sanitizeAccountForPath(account)
	return filepath.Join(dir, name+".json"), nil
}

func newSnoozeStore(account string) (*snoozeStore, error) {
	path, err := gmailSnoozeStatePath(account)
	if err != nil {
		return nil, err
	}
	return &snoozeStore{path: path}, nil
}

func loadSnoozeStore(account string) (*snoozeStore, error) {
	store, err := newSnoozeStore(account)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Valid to start fresh — snooze store may not exist yet.
			return store, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &store.state); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *snoozeStore) Get() snoozeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *snoozeStore) Save() error {
	if s.path == "" {
		return errors.New("missing snooze state path")
	}
	payload, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(payload, '\n'), 0o600)
}

// Upsert adds or replaces an entry matched by ThreadID + Account.
func (s *snoozeStore) Upsert(entry snoozeEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.state.Entries {
		if e.ThreadID == entry.ThreadID && e.Account == entry.Account {
			s.state.Entries[i] = entry
			return s.Save()
		}
	}
	s.state.Entries = append(s.state.Entries, entry)
	return s.Save()
}

// Remove deletes the entry matching threadID + account.
func (s *snoozeStore) Remove(threadID, account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.state.Entries[:0]
	for _, e := range s.state.Entries {
		if e.ThreadID == threadID && e.Account == account {
			continue
		}
		filtered = append(filtered, e)
	}
	s.state.Entries = filtered
	return s.Save()
}

// DueEntries returns entries whose UntilMs <= now.UnixMilli().
// If account is non-empty, only entries for that account are considered.
func (s *snoozeStore) DueEntries(now time.Time, account string) []snoozeEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	nowMs := now.UnixMilli()
	var out []snoozeEntry
	for _, e := range s.state.Entries {
		if account != "" && e.Account != account {
			continue
		}
		if e.UntilMs <= nowMs {
			out = append(out, e)
		}
	}
	return out
}

// AllEntries returns all entries, optionally filtered by account.
// An empty account string returns entries for all accounts.
func (s *snoozeStore) AllEntries(account string) []snoozeEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []snoozeEntry
	for _, e := range s.state.Entries {
		if account != "" && e.Account != account {
			continue
		}
		out = append(out, e)
	}
	return out
}
