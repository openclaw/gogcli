package cmd

import (
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/config"
)

const gmailSendCommand = "send"

func enforceGmailNoSend(kctx *kong.Context, flags *RootFlags) error {
	if !isGmailSendPath(commandPath(kctx.Command())) {
		return nil
	}
	if flags != nil && flags.GmailNoSend {
		return usage("Gmail sending is blocked by --gmail-no-send")
	}
	cfg, err := config.ReadConfig()
	if err != nil {
		return err
	}
	if cfg.GmailNoSend {
		return usage("Gmail sending is blocked by config gmail_no_send")
	}
	return nil
}

func checkAccountNoSend(account string) error {
	disabled, err := config.IsNoSendAccount(account)
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
	if path[0] == gmailSendCommand {
		return true
	}
	if len(path) < 2 || path[0] != "gmail" {
		return false
	}
	switch path[1] {
	case gmailSendCommand, "autoreply", "forward":
		return true
	case "drafts":
		return len(path) >= 3 && path[2] == gmailSendCommand
	default:
		return false
	}
}
