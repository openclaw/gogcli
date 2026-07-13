package cmd

import (
	"context"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/app"
)

var gmailSendCommandPaths = map[string]struct{}{
	"send":              {},
	"gmail.send":        {},
	"gmail.reply":       {},
	"gmail.reply-all":   {},
	"gmail.replyall":    {},
	"gmail.autoreply":   {},
	"gmail.forward":     {},
	"gmail.fwd":         {},
	"gmail.drafts.send": {},
}

func enforceGmailNoSend(kctx *kong.Context, flags *RootFlags, runtime *app.Runtime) error {
	if !isGmailSendPath(commandPath(kctx.Command())) {
		return nil
	}
	if flags != nil {
		if flags.GmailNoSend {
			return usage("Gmail sending is blocked by --gmail-no-send")
		}
	}
	if err := configureRuntimeConfig(runtime); err != nil {
		return err
	}
	cfg, err := runtime.Config.Read()
	if err != nil {
		return err
	}
	if cfg.GmailNoSend {
		return usage("Gmail sending is blocked by config gmail_no_send")
	}
	// Per-account guard, enforced at the same layer as the flag and the
	// global config key so it also holds under --dry-run, which exits the
	// command before the post-auth checkAccountNoSend call is reached.
	// Skip entirely when no per-account guards exist so a plain dry-run
	// never resolves an account (default-account inference reads the
	// keyring). Account resolution failures are not errors here: commands
	// own that failure mode, and checkAccountNoSend still covers real
	// sends after auth resolves the account.
	if !hasActiveNoSendAccount(cfg.NoSendAccounts) {
		return nil
	}
	if account, accountErr := requireAccount(flags); accountErr == nil {
		blocked, blockedErr := runtime.Config.IsNoSendAccount(account)
		if blockedErr != nil {
			return blockedErr
		}
		if blocked {
			return usagef("Gmail sending is blocked for %s (config no-send)", strings.TrimSpace(account))
		}
	}
	return nil
}

func hasActiveNoSendAccount(accounts map[string]bool) bool {
	for _, blocked := range accounts {
		if blocked {
			return true
		}
	}
	return false
}

func checkAccountNoSend(ctx context.Context, account string) error {
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	disabled, err := store.IsNoSendAccount(account)
	if err != nil {
		return err
	}
	if disabled {
		return usagef("Gmail sending is blocked for %s (config no-send)", strings.TrimSpace(account))
	}
	return nil
}

func isGmailSendPath(path []string) bool {
	if len(path) == 0 {
		return false
	}
	_, ok := gmailSendCommandPaths[strings.Join(path, ".")]
	return ok
}
