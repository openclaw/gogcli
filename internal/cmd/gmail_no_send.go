package cmd

import (
	"strings"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/config"
)

var gmailSendCommandPaths = map[string]struct{}{
	"send":              {},
	"gmail.send":        {},
	"gmail.autoreply":   {},
	"gmail.forward":     {},
	"gmail.fwd":         {},
	"gmail.drafts.send": {},
}

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
	_, ok := gmailSendCommandPaths[strings.Join(path, ".")]
	return ok
}
