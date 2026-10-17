package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/secrets"
	"github.com/openclaw/gogcli/internal/ui"
)

const (
	rekeyPasswordEnv    = "GOG_KEYRING_PASSWORD"     //nolint:gosec // env var name, not a credential
	rekeyNewPasswordEnv = "GOG_KEYRING_NEW_PASSWORD" //nolint:gosec // env var name, not a credential
)

// AuthRekeyCmd writes a verified, re-encrypted copy to a separate directory.
// It never modifies the live keyring; installation is a later user action.
type AuthRekeyCmd struct {
	OutDir         string `arg:"" name:"outdir" help:"Separate output directory for the re-encrypted keyring; --force replaces nonempty output"`
	SkipUnreadable bool   `name:"skip-unreadable" help:"Omit undecryptable items from output and report their keys; live items remain untouched"`
}

func (c *AuthRekeyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	layout, err := commandLayout(ctx, config.PathKindConfig, config.PathKindData)
	if err != nil {
		return err
	}

	backendInfo, err := resolveKeyringBackendInfo(ctx)
	if err != nil {
		return err
	}

	configStore, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	openOptions := systemKeyringOpenOptions(layout, configStore)
	if err = secrets.CheckRekeyBackend(backendInfo.Value, openOptions.GOOS, openOptions.DBusAddress); err != nil {
		return err
	}
	if c.OutDir == "" {
		return usagef("output directory is required; pass it as the positional argument")
	}
	// Only output replacement is destructive, and it requires explicit --force
	// in Rekey. A fresh output needs no confirmation and never opens the live
	// keyring through the normal (directory-creating) backend resolver.
	if flags.DryRun {
		return dryRunExit(ctx, flags, "auth.rekey", map[string]any{"outdir": c.OutDir, "skip_unreadable": c.SkipUnreadable, "force": flags.Force})
	}

	oldPassword, oldSet, err := rekeyReadPassword(ctx, rekeyPasswordEnv, "Current keyring password: ", flags.NoInput)
	if err != nil {
		return err
	}
	if !oldSet {
		return usagef("no current password: set %s, or run on a TTY without --no-input", rekeyPasswordEnv)
	}

	newPassword, newSet, err := rekeyReadNewPassword(ctx, flags.NoInput)
	if err != nil {
		return err
	}
	if !newSet {
		return usagef("no new password: set %s, or run on a TTY without --no-input", rekeyNewPasswordEnv)
	}

	res, err := secrets.Rekey(secrets.RekeyOptions{
		Layout:         layout,
		OutDir:         c.OutDir,
		Force:          flags.Force,
		Backend:        backendInfo.Value,
		OldPassword:    oldPassword,
		OldPasswordSet: oldSet,
		NewPassword:    newPassword,
		NewPasswordSet: newSet,
		GOOS:           openOptions.GOOS,
		DBusAddress:    openOptions.DBusAddress,
		SkipUnreadable: c.SkipUnreadable,
	})
	if err != nil {
		if len(res.Replaced) > 0 {
			_, _ = fmt.Fprintf(stderrWriter(ctx), "Removed previous output entries in %s: %s. Live keyring untouched; output was not verified.\n", res.OutDir, strings.Join(res.Replaced, ", "))
		}
		return err
	}

	return writeRekeyResult(ctx, u, res)
}

// rekeyPromptSecret reads a password without trimming it. A keyring password may
// legitimately begin or end with whitespace, and an environment-supplied value is
// used verbatim, so trimming only the prompted side would rotate the keyring to a
// password the user never set.
func rekeyPromptSecret(ctx context.Context, prompt string) (string, error) {
	_, _ = fmt.Fprint(stderrWriter(ctx), prompt)

	secret, err := readSecret(stdinReader(ctx))
	_, _ = fmt.Fprintln(stderrWriter(ctx))

	if err != nil {
		return "", err
	}

	return string(secret), nil
}

// rekeyReadPassword resolves a password from the environment, falling back to a
// no-echo terminal prompt. Passwords never come from argv, where they would be
// visible to every process on the box.
func rekeyReadPassword(ctx context.Context, env, prompt string, noInput bool) (string, bool, error) {
	if value, ok := os.LookupEnv(env); ok {
		return value, true, nil
	}

	if noInput || !stdinIsTerminal(ctx) {
		return "", false, nil
	}

	value, err := rekeyPromptSecret(ctx, prompt)
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", env, err)
	}

	return value, true, nil
}

// rekeyReadNewPassword asks twice when prompting interactively: a typo here
// would encrypt the output with a password the user never intended.
func rekeyReadNewPassword(ctx context.Context, noInput bool) (string, bool, error) {
	if value, ok := os.LookupEnv(rekeyNewPasswordEnv); ok {
		return value, true, nil
	}

	if noInput || !stdinIsTerminal(ctx) {
		return "", false, nil
	}

	first, err := rekeyPromptSecret(ctx, "New keyring password: ")
	if err != nil {
		return "", false, fmt.Errorf("read new keyring password: %w", err)
	}
	if first == "" {
		return "", false, usagef("new keyring password cannot be empty; set %s= to force one", rekeyNewPasswordEnv)
	}

	second, err := rekeyPromptSecret(ctx, "Repeat new keyring password: ")
	if err != nil {
		return "", false, fmt.Errorf("read new keyring password: %w", err)
	}
	if first != second {
		return "", false, usagef("new keyring passwords do not match")
	}

	return first, true, nil
}

func writeRekeyResult(ctx context.Context, u *ui.UI, res secrets.RekeyResult) error {
	next := "Inspect the output, stop running gog processes before installing, and keep " + res.RetainedDir + " until the new keyring is proven. Update GOG_KEYRING_PASSWORD only after installation."
	if res.Items == 0 {
		next = "Empty output: no password was changed or established. " + next
	}
	if len(res.Unreadable) > 0 {
		next = "Output omits unreadable keys; inspect these before installing. " + next
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"keyring_dir": res.Dir, "output_dir": res.OutDir, "items": res.Items,
			"keys": res.Keys, "unreadable": res.Unreadable, "replaced": res.Replaced,
			"password_changed": false, "move_command": res.MoveCommand, "next_step": next,
			"retained_dir": res.RetainedDir,
		})
	}
	if u == nil {
		return fmt.Errorf("cannot report rekey output without a UI")
	}
	if outfmt.IsPlain(ctx) {
		u.Out().Linef("keyring_dir\t%s", res.Dir)
		u.Out().Linef("output_dir\t%s", res.OutDir)
		u.Out().Linef("retained_dir\t%s", res.RetainedDir)
		u.Out().Linef("items\t%d", res.Items)
		u.Out().Linef("keys\t%s", strings.Join(res.Keys, ","))
		u.Out().Linef("password_changed\tfalse")
		u.Out().Linef("replaced\t%s", strings.Join(res.Replaced, ","))
		u.Out().Linef("unreadable\t%s", strings.Join(res.Unreadable, ","))
		u.Out().Linef("move_command\t%s", res.MoveCommand)
		u.Out().Linef("next_step\t%s", next)
		return nil
	}
	u.Out().Linef("Wrote and verified %d item(s) in %s. Live keyring untouched: %s", res.Items, res.OutDir, res.Dir)
	u.Out().Linef("Keys: %s", strings.Join(res.Keys, ", "))
	if len(res.Replaced) > 0 {
		u.Out().Linef("Replaced output entries in %s: %s", res.OutDir, strings.Join(res.Replaced, ", "))
	}
	if len(res.Unreadable) > 0 {
		u.Err().Linef("Omitted from output, cannot be decrypted: %s", strings.Join(res.Unreadable, ", "))
	}
	u.Out().Println(next)
	u.Out().Linef("Install with: %s", res.MoveCommand)
	return nil
}
