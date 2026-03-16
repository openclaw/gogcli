package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
)

// gmailSendCommands lists command-path prefixes that transmit email.
var gmailSendCommands = [][]string{
	{"send"},                    // top-level alias → GmailSendCmd
	{"gmail", "send"},           // gog gmail send
	{"gmail", "drafts", "send"}, // gog gmail drafts send
	{"gmail", "autoreply"},      // gog gmail autoreply
}

func enforceGmailNoSend(kctx *kong.Context, noSend bool) error {
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
				"send blocked by --gmail-no-send (GOG_GMAIL_NO_SEND): use \"gog gmail drafts create\" to compose without sending",
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
