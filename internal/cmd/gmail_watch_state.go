package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/steipete/gogcli/internal/config"
)

type gmailWatchStore struct {
	path  string
	mu    sync.Mutex
	state gmailWatchState
}

func gmailWatchStatePath(account string) (string, error) {
	dir, err := config.EnsureGmailWatchDir()
	if err != nil {
		return "", err
	}
	name := sanitizeAccountForPath(account)
	return filepath.Join(dir, name+".json"), nil
}

func sanitizeAccountForPath(account string) string {
	clean := strings.TrimSpace(strings.ToLower(account))
	if clean == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(clean))
	for _, r := range clean {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_' || r == '@':
			b.WriteRune('_')
		case r > unicode.MaxASCII:
			b.WriteRune('_')
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func newGmailWatchStore(account string) (*gmailWatchStore, error) {
	path, err := gmailWatchStatePath(account)
	if err != nil {
		return nil, err
	}
	return &gmailWatchStore{path: path}, nil
}

func loadGmailWatchStore(account string) (*gmailWatchStore, error) {
	store, err := newGmailWatchStore(account)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("watch state not found; run gmail watch start")
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &store.state); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *gmailWatchStore) Get() gmailWatchState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *gmailWatchStore) Update(fn func(*gmailWatchState) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return err
	}
	if err := fn(&s.state); err != nil {
		return err
	}
	return s.Save()
}

func (s *gmailWatchStore) reloadLocked() error {
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reload watch state: %w", err)
	}
	var state gmailWatchState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("reload watch state: %w", err)
	}
	s.state = state
	return nil
}

func (s *gmailWatchStore) Save() error {
	if s.path == "" {
		return errors.New("missing watch state path")
	}
	payload, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(payload, '\n'), 0o600)
}

func (s *gmailWatchStore) StartHistoryID(pushHistory string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pushID, pushOK, pushErr := parseHistoryIDOptional(pushHistory)

	// If no stored state, use push historyId
	if s.state.HistoryID == "" {
		if !pushOK {
			if pushErr != nil {
				return 0, pushErr
			}
			return 0, nil
		}
		if pushErr != nil {
			return 0, pushErr
		}
		s.state.HistoryID = formatHistoryID(pushID)
		s.state.UpdatedAtMs = time.Now().UnixMilli()
		_ = s.Save()
		return pushID, nil
	}

	storedID, storedOK, err := parseHistoryIDOptional(s.state.HistoryID)
	if err != nil {
		return 0, err
	}
	if !storedOK {
		return 0, nil
	}
	if pushErr != nil {
		return storedID, nil
	}
	if !pushOK {
		return storedID, nil
	}
	if pushID <= storedID {
		return 0, nil
	}

	return storedID, nil
}

func parseHistoryIDOptional(raw string) (uint64, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false, nil
	}
	id, err := parseHistoryID(trimmed)
	if err != nil {
		return 0, true, err
	}
	return id, true, nil
}

func compareHistoryIDs(storedRaw, candidateRaw string) (storedID, candidateID uint64, storedOK, candidateOK bool, err error) {
	storedID, storedOK, err = parseHistoryIDOptional(storedRaw)
	if err != nil {
		return 0, 0, false, false, err
	}
	candidateID, candidateOK, err = parseHistoryIDOptional(candidateRaw)
	if err != nil {
		return storedID, 0, storedOK, true, err
	}
	return storedID, candidateID, storedOK, candidateOK, nil
}

func shouldUpdateHistoryID(currentRaw, candidateRaw string) (bool, error) {
	currentID, candidateID, currentOK, candidateOK, err := compareHistoryIDs(currentRaw, candidateRaw)
	if err != nil {
		return false, err
	}
	if !candidateOK {
		return false, nil
	}
	if !currentOK {
		return true, nil
	}
	return candidateID >= currentID, nil
}

func isStaleHistoryID(currentRaw, candidateRaw string) (bool, error) {
	currentID, candidateID, currentOK, candidateOK, err := compareHistoryIDs(currentRaw, candidateRaw)
	if err != nil {
		return false, err
	}
	if !currentOK || !candidateOK {
		return false, nil
	}
	return candidateID <= currentID, nil
}

func parseHistoryID(raw string) (uint64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, errors.New("historyId is required")
	}
	id, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid historyId %q", trimmed)
	}
	return id, nil
}

func formatHistoryID(id uint64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatUint(id, 10)
}
