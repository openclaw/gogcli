package secrets

import (
	"os"
	"runtime"
	"testing"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/termutil"
)

func testSystemLayout(tb testing.TB, kinds ...config.PathKind) config.Layout {
	tb.Helper()

	layout, err := config.NewSystemResolver("").Resolve(kinds...)
	if err != nil {
		tb.Fatalf("resolve test layout: %v", err)
	}

	return layout
}

func openSystemTestStore(tb testing.TB) Repository {
	tb.Helper()

	layout := testSystemLayout(tb, config.PathKindConfig, config.PathKindData)

	store, err := Open(systemTestOpenOptions(layout, config.NewConfigStore(layout)))
	if err != nil {
		tb.Fatalf("open test store: %v", err)
	}

	return store
}

func systemTestOpenOptions(layout config.Layout, store *config.ConfigStore) OpenOptions {
	return OpenOptionsFromLookup(layout, store, os.LookupEnv, runtime.GOOS, termutil.IsTerminal(os.Stdin))
}
