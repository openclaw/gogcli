package cmd

import (
	"os"
	"path/filepath"
	"testing"

	_ "github.com/openclaw/gogcli/internal/tzembed" // Embed IANA timezone database for Windows test support
)

func TestMain(m *testing.M) {
	contactsSearchWarmupDelay = 0

	root, err := os.MkdirTemp("", "gogcli-tests-*")
	if err != nil {
		panic(err)
	}

	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")

	home := filepath.Join(root, "home")
	xdg := filepath.Join(root, "xdg")
	_ = os.MkdirAll(home, 0o755)
	_ = os.MkdirAll(xdg, 0o755)
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("XDG_CONFIG_HOME", xdg)

	// Ambient GOG_* path overrides and XDG data/state/cache directories escape
	// this sandbox entirely: the layout resolver (internal/config/layout.go)
	// honors them ahead of the HOME- and XDG_CONFIG_HOME-derived defaults set
	// above, pointing tests at shared real directories. Unset rather than
	// redirect: a single shared override directory still cross-contaminates
	// tests. Per-test t.Setenv values are unaffected.
	oldPathEnv := map[string]string{}
	for _, name := range []string{
		"GOG_HOME", "GOG_CONFIG_DIR", "GOG_DATA_DIR", "GOG_STATE_DIR", "GOG_CACHE_DIR",
		"XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME",
	} {
		if value, ok := os.LookupEnv(name); ok {
			oldPathEnv[name] = value
		}
		_ = os.Unsetenv(name)
	}

	code := m.Run()

	if oldHome == "" {
		_ = os.Unsetenv("HOME")
	} else {
		_ = os.Setenv("HOME", oldHome)
	}
	if oldXDG == "" {
		_ = os.Unsetenv("XDG_CONFIG_HOME")
	} else {
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
	}

	for name, value := range oldPathEnv {
		_ = os.Setenv(name, value)
	}
	_ = os.RemoveAll(root)
	os.Exit(code)
}
