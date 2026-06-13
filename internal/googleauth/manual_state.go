package googleauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/config"
)

const (
	manualStateFilePrefix = "oauth-manual-state-"
	manualStateFileSuffix = ".json"
	manualStateMaxLength  = 256
)

var (
	errEmptyManualAuthState   = errors.New("empty manual auth state")
	errInvalidManualAuthState = errors.New("invalid manual auth state")
	errManualStateDirRequired = errors.New("manual auth state directory is required")
	errManualStateDirAbsolute = errors.New("manual auth state directory must be absolute")
)

// manualStateTTL controls how long a stored manual auth state is considered valid.
// This should be shorter than typical OAuth code expiration windows.
const manualStateTTL = 10 * time.Minute

type manualState struct {
	State        string    `json:"state"`
	Client       string    `json:"client"`
	Scopes       []string  `json:"scopes"`
	ForceConsent bool      `json:"force_consent,omitempty"`
	RedirectURI  string    `json:"redirect_uri,omitempty"`
	CodeVerifier string    `json:"code_verifier,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type ManualStateStore struct {
	dir string
	now func() time.Time
}

func NewManualStateStore(layout config.Layout) (*ManualStateStore, error) {
	dir := strings.TrimSpace(layout.ConfigDir)
	if dir == "" {
		return nil, errManualStateDirRequired
	}

	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("%w: %s", errManualStateDirAbsolute, dir)
	}

	return &ManualStateStore{
		dir: filepath.Clean(dir),
		now: time.Now,
	}, nil
}

func (s *ManualStateStore) Dir() string {
	if s == nil {
		return ""
	}

	return s.dir
}

func (s *ManualStateStore) pathFor(state string) (string, error) {
	state = strings.TrimSpace(state)
	if state == "" {
		return "", errEmptyManualAuthState
	}

	if !isValidManualState(state) {
		return "", errInvalidManualAuthState
	}

	return filepath.Join(s.dir, manualStateFilePrefix+state+manualStateFileSuffix), nil
}

func isValidManualState(state string) bool {
	if len(state) == 0 || len(state) > manualStateMaxLength {
		return false
	}

	for _, r := range state {
		if r >= 'a' && r <= 'z' ||
			r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' ||
			r == '-' ||
			r == '_' {
			continue
		}

		return false
	}

	return true
}

func isManualStateFilename(name string) (state string, ok bool) {
	if !strings.HasPrefix(name, manualStateFilePrefix) || !strings.HasSuffix(name, manualStateFileSuffix) {
		return "", false
	}

	state = strings.TrimSuffix(strings.TrimPrefix(name, manualStateFilePrefix), manualStateFileSuffix)
	if !isValidManualState(state) {
		return "", false
	}

	return state, true
}

func (s *ManualStateStore) load(client string, scopes []string, forceConsent bool) (manualState, bool, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return manualState{}, false, nil
		}

		return manualState{}, false, fmt.Errorf("read manual auth state dir: %w", err)
	}

	var (
		bestState   manualState
		bestCreated time.Time
	)

	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}

		_, ok := isManualStateFilename(ent.Name())
		if !ok {
			continue
		}

		path := filepath.Join(s.dir, ent.Name())

		st, valid, loadErr := s.loadByPath(path)
		if loadErr != nil {
			return manualState{}, false, loadErr
		}

		if !valid {
			continue
		}

		if st.Client != client || st.ForceConsent != forceConsent || !scopesEqual(st.Scopes, scopes) {
			continue
		}

		// RedirectURI is required for step 1 URL generation and step 2 Exchange.
		// Older cache entries (pre-redirect tracking) should not be reused.
		if strings.TrimSpace(st.RedirectURI) == "" {
			continue
		}

		// CodeVerifier is required for PKCE-bound step 2 exchanges.
		// Older cache entries (pre-PKCE) should not be reused.
		if strings.TrimSpace(st.CodeVerifier) == "" {
			continue
		}

		if bestState.State == "" || st.CreatedAt.After(bestCreated) {
			bestState = st
			bestCreated = st.CreatedAt
		}
	}

	if bestState.State == "" {
		return manualState{}, false, nil
	}

	return bestState, true, nil
}

func (s *ManualStateStore) loadState(state string) (manualState, bool, error) {
	path, err := s.pathFor(state)
	if err != nil {
		return manualState{}, false, err
	}

	return s.loadByPath(path)
}

func (s *ManualStateStore) loadByPath(path string) (manualState, bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // config path
	if err != nil {
		if os.IsNotExist(err) {
			return manualState{}, false, nil
		}

		return manualState{}, false, fmt.Errorf("read manual auth state: %w", err)
	}

	var st manualState
	if err := json.Unmarshal(data, &st); err != nil {
		_ = os.Remove(path)
		return manualState{}, false, nil //nolint:nilerr // invalid state should be treated as a cache miss
	}

	if st.State == "" {
		_ = os.Remove(path)
		return manualState{}, false, nil
	}

	if s.now().Sub(st.CreatedAt) > manualStateTTL {
		_ = os.Remove(path)
		return manualState{}, false, nil
	}

	return st, true, nil
}

func (s *ManualStateStore) save(
	client string,
	scopes []string,
	forceConsent bool,
	state string,
	redirectURI string,
	codeVerifier string,
) error {
	path, err := s.pathFor(state)
	if err != nil {
		return err
	}

	if mkdirErr := os.MkdirAll(s.dir, 0o700); mkdirErr != nil {
		return fmt.Errorf("create manual auth state dir: %w", mkdirErr)
	}

	st := manualState{
		State:        state,
		Client:       client,
		Scopes:       normalizeScopes(scopes),
		ForceConsent: forceConsent,
		RedirectURI:  strings.TrimSpace(redirectURI),
		CodeVerifier: strings.TrimSpace(codeVerifier),
		CreatedAt:    s.now().UTC(),
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manual auth state: %w", err)
	}

	data = append(data, '\n')

	if err := config.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("write manual auth state: %w", err)
	}

	return nil
}

func (s *ManualStateStore) clear(state string) error {
	path, err := s.pathFor(state)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("remove manual auth state: %w", err)
	}

	return nil
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}

	out := append([]string(nil), scopes...)
	sort.Strings(out)

	return out
}

func scopesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	na := normalizeScopes(a)
	nb := normalizeScopes(b)

	for i := range na {
		if na[i] != nb[i] {
			return false
		}
	}

	return true
}
