package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

type AuthKeyringCmd struct {
	Backend  string `arg:"" optional:"" name:"backend" help:"Keyring backend: auto|keychain|file|1password"`
	Backend2 string `arg:"" optional:"" name:"backend2" help:"(compat) Use: gog auth keyring set <backend>"`
}

func (c *AuthKeyringCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	const keyringPasswordEnv = "GOG_KEYRING_PASSWORD" //nolint:gosec // env var name, not a credential

	backend := strings.ToLower(strings.TrimSpace(c.Backend))
	backend2 := strings.ToLower(strings.TrimSpace(c.Backend2))

	// Backwards compat for earlier suggestion: `gog auth keyring set <backend>`.
	if backend == "set" {
		backend = backend2
		backend2 = ""
	}

	// No args: show current config.
	if backend == "" {
		store, err := commandConfigStore(ctx)
		if err != nil {
			return err
		}
		path := store.Path()
		info, err := resolveKeyringBackendInfo(ctx)
		if err != nil {
			return err
		}

		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
				"keyring_backend": info.Value,
				"source":          info.Source,
				"path":            path,
			})
		}

		if u == nil {
			return nil
		}
		u.Out().Linef("path\t%s", path)
		u.Out().Linef("keyring_backend\t%s", info.Value)
		u.Out().Linef("source\t%s", info.Source)
		u.Err().Println("Hint: gog auth keyring <auto|keychain|file|1password>")
		return nil
	}

	if backend2 != "" {
		return usagef("too many args: %q %q", c.Backend, c.Backend2)
	}

	if backend == literalDefault {
		backend = literalAuto
	}

	allowed := map[string]struct{}{
		literalAuto:                       {},
		"keychain":                        {},
		strFile:                           {},
		secrets.KeyringBackendOnePassword: {},
	}
	if _, ok := allowed[backend]; !ok {
		return usagef("invalid backend: %q (expected auto, keychain, file, or 1password)", c.Backend)
	}

	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	path := store.Path()
	if dryRunErr := dryRunExit(ctx, flags, "auth.keyring.set", map[string]any{
		"path":            path,
		"keyring_backend": backend,
	}); dryRunErr != nil {
		return dryRunErr
	}

	cfg, err := store.Read()
	if err != nil {
		return err
	}
	cfg.KeyringBackend = backend
	if err := store.Write(cfg); err != nil {
		return err
	}

	// Env var wins; warn so it doesn't look "broken".
	if v := strings.TrimSpace(os.Getenv("GOG_KEYRING_BACKEND")); v != "" &&
		u != nil &&
		!outfmt.IsJSON(ctx) &&
		!outfmt.IsPlain(ctx) {
		u.Err().Linef("NOTE: GOG_KEYRING_BACKEND=%s overrides config.json", v)
	}

	if backend == strFile &&
		u != nil &&
		!outfmt.IsJSON(ctx) &&
		!outfmt.IsPlain(ctx) {
		if v := strings.TrimSpace(os.Getenv(keyringPasswordEnv)); v != "" {
			u.Err().Println("GOG_KEYRING_PASSWORD found in environment.")
		} else if !stdinIsTerminal(ctx) {
			u.Err().Linef("NOTE: file keyring backend in non-interactive context requires %s", keyringPasswordEnv)
		} else {
			u.Err().Linef("Hint: set %s for non-interactive use (CI/ssh)", keyringPasswordEnv)
		}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"written":         true,
			"path":            path,
			"keyring_backend": backend,
		})
	}

	if u == nil {
		return nil
	}

	u.Out().Linef("written\ttrue")
	u.Out().Linef("path\t%s", path)
	u.Out().Linef("keyring_backend\t%s", backend)
	return nil
}
