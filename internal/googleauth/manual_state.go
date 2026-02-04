package googleauth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/steipete/gogcli/internal/config"
)

const manualStateFilename = "oauth-manual-state.json"

// manualStateTTL controls how long a stored manual auth state is considered valid.
// This should be shorter than typical OAuth code expiration windows.
const manualStateTTL = 10 * time.Minute

type manualState struct {
	State        string    `json:"state"`
	Client       string    `json:"client"`
	Scopes       []string  `json:"scopes"`
	ForceConsent bool      `json:"force_consent,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

var (
	manualStatePathFn = manualStatePath
	manualStateNowFn  = time.Now
)

func manualStatePath() (string, error) {
	dir, err := config.EnsureDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, manualStateFilename), nil
}

func loadManualState(client string, scopes []string, forceConsent bool) (string, bool, error) {
	path, err := manualStatePathFn()
	if err != nil {
		return "", false, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // config path
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read manual auth state: %w", err)
	}

	var st manualState
	if err := json.Unmarshal(data, &st); err != nil {
		_ = os.Remove(path)
		return "", false, nil
	}
	if st.State == "" {
		_ = os.Remove(path)
		return "", false, nil
	}
	if manualStateNowFn().Sub(st.CreatedAt) > manualStateTTL {
		_ = os.Remove(path)
		return "", false, nil
	}
	if st.Client != client || st.ForceConsent != forceConsent || !scopesEqual(st.Scopes, scopes) {
		return "", false, nil
	}

	return st.State, true, nil
}

func loadManualStateStrict(client string, scopes []string, forceConsent bool) (string, error) {
	path, err := manualStatePathFn()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path) //nolint:gosec // config path
	if err != nil {
		if os.IsNotExist(err) {
			return "", errManualStateMissing
		}
		return "", fmt.Errorf("read manual auth state: %w", err)
	}

	var st manualState
	if err := json.Unmarshal(data, &st); err != nil {
		_ = os.Remove(path)
		return "", errManualStateMissing
	}
	if st.State == "" {
		_ = os.Remove(path)
		return "", errManualStateMissing
	}
	if manualStateNowFn().Sub(st.CreatedAt) > manualStateTTL {
		_ = os.Remove(path)
		return "", errManualStateMissing
	}
	if st.Client != client || st.ForceConsent != forceConsent || !scopesEqual(st.Scopes, scopes) {
		return "", errManualStateMismatch
	}

	return st.State, nil
}

func saveManualState(client string, scopes []string, forceConsent bool, state string) error {
	path, err := manualStatePathFn()
	if err != nil {
		return err
	}

	st := manualState{
		State:        state,
		Client:       client,
		Scopes:       normalizeScopes(scopes),
		ForceConsent: forceConsent,
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

func clearManualState() error {
	path, err := manualStatePathFn()
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
