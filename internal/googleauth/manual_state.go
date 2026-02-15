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
)

var errEmptyManualAuthState = errors.New("empty manual auth state")

// manualStateTTL controls how long a stored manual auth state is considered valid.
// This should be shorter than typical OAuth code expiration windows.
const manualStateTTL = 10 * time.Minute

type manualState struct {
	State        string    `json:"state"`
	Client       string    `json:"client"`
	Scopes       []string  `json:"scopes"`
	ForceConsent bool      `json:"force_consent,omitempty"`
	RedirectURI  string    `json:"redirect_uri,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

var (
	manualStateDirFn = manualStateDir
	manualStateNowFn = time.Now
)

func manualStateDir() (string, error) {
	dir, err := config.EnsureDir()
	if err != nil {
		return "", fmt.Errorf("ensure config dir: %w", err)
	}

	return dir, nil
}

func manualStatePathFor(state string) (string, error) {
	dir, err := manualStateDirFn()
	if err != nil {
		return "", err
	}

	state = strings.TrimSpace(state)
	if state == "" {
		return "", errEmptyManualAuthState
	}

	return filepath.Join(dir, manualStateFilePrefix+state+manualStateFileSuffix), nil
}

func isManualStateFilename(name string) (state string, ok bool) {
	if !strings.HasPrefix(name, manualStateFilePrefix) || !strings.HasSuffix(name, manualStateFileSuffix) {
		return "", false
	}

	state = strings.TrimSuffix(strings.TrimPrefix(name, manualStateFilePrefix), manualStateFileSuffix)
	if strings.TrimSpace(state) == "" {
		return "", false
	}

	return state, true
}

func loadManualState(client string, scopes []string, forceConsent bool) (manualState, bool, error) {
	dir, err := manualStateDirFn()
	if err != nil {
		return manualState{}, false, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
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

		path := filepath.Join(dir, ent.Name())

		st, valid, loadErr := loadManualStateByPath(path)
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

func loadManualStateByPath(path string) (manualState, bool, error) {
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

	if manualStateNowFn().Sub(st.CreatedAt) > manualStateTTL {
		_ = os.Remove(path)
		return manualState{}, false, nil
	}

	return st, true, nil
}

func saveManualState(client string, scopes []string, forceConsent bool, state string, redirectURI string) error {
	path, err := manualStatePathFor(state)
	if err != nil {
		return err
	}

	st := manualState{
		State:        state,
		Client:       client,
		Scopes:       normalizeScopes(scopes),
		ForceConsent: forceConsent,
		RedirectURI:  strings.TrimSpace(redirectURI),
		CreatedAt:    manualStateNowFn().UTC(),
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manual auth state: %w", err)
	}

	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write manual auth state: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit manual auth state: %w", err)
	}

	return nil
}

func clearManualState(state string) error {
	path, err := manualStatePathFor(state)
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
