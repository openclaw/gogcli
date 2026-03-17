package cmd

import (
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/config"
)

// gmailSendCommands lists command-path prefixes that transmit email.
var gmailSendCommands = [][]string{
	{"send"},                    // top-level alias → GmailSendCmd
	{"gmail", "send"},           // gog gmail send
	{"gmail", "drafts", "send"}, // gog gmail drafts send
	{"gmail", "autoreply"},      // gog gmail autoreply
}

func enforceGmailNoSend(kctx *kong.Context, flagNoSend bool) error {
	noSend := flagNoSend
	source := "--gmail-no-send (GOG_GMAIL_NO_SEND)"

	if !noSend {
		if cfg, err := config.ReadConfig(); err == nil && cfg.GmailNoSend {
			noSend = true
			source = "gmail_no_send config"
		}
	}

	if !noSend {
		return nil
	}

	parts := strings.Fields(kctx.Command())
	if len(parts) == 0 {
		return nil
	}

	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}

	for _, prefix := range gmailSendCommands {
		if hasCmdPrefix(parts, prefix) {
			return usagef(
				"send blocked by %s: use \"gog gmail drafts create\" to compose without sending",
				source,
			)
		}
	}

	return nil
}

func hasCmdPrefix(parts, prefix []string) bool {
	if len(parts) < len(prefix) {
		return false
	}

	for i, p := range prefix {
		if parts[i] != p {
			return false
		}
	}

	return true
}

// checkAccountNoSend reads the config and blocks send if the resolved account
// is in the no_send_accounts list. Call this after requireGmailService().
func checkAccountNoSend(account string) error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return err
	}

	if config.IsNoSendAccount(cfg, account) {
		return usagef(
			"send blocked for account %q (no_send_accounts): use \"gog gmail drafts create\" to compose without sending",
			account,
		)
	}

	return nil
}

// isGmailSendCommand reports whether the kong command string is a send path.
// Exported for testing.
func isGmailSendCommand(command string) bool {
	parts := strings.Fields(strings.ToLower(command))
	if len(parts) == 0 {
		return false
	}

	for _, prefix := range gmailSendCommands {
		if hasCmdPrefix(parts, prefix) {
			return true
		}
	}

	return false
}
