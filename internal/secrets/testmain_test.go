package secrets

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Tests isolate storage via per-test temp directories (HOME by default),
	// but the layout resolver (internal/config/layout.go) honors ambient GOG_*
	// path overrides and XDG base directories ahead of HOME-derived defaults,
	// leaking state across tests and into the developer's real gogcli
	// directories. Unset rather than redirect: a single shared override
	// directory still cross-contaminates tests. Per-test t.Setenv values are
	// unaffected.
	for _, name := range []string{
		"GOG_HOME", "GOG_CONFIG_DIR", "GOG_DATA_DIR", "GOG_STATE_DIR", "GOG_CACHE_DIR",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME",
	} {
		_ = os.Unsetenv(name)
	}

	os.Exit(m.Run())
}
