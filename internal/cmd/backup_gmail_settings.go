package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/backup"
)

type gmailSettingsBackup struct {
	Filters             []*gmail.Filter            `json:"filters,omitempty"`
	ForwardingAddresses []*gmail.ForwardingAddress `json:"forwardingAddresses,omitempty"`
	AutoForwarding      *gmail.AutoForwarding      `json:"autoForwarding,omitempty"`
	SendAs              []*gmail.SendAs            `json:"sendAs,omitempty"`
	Vacation            *gmail.VacationSettings    `json:"vacation,omitempty"`
	Delegates           []*gmail.Delegate          `json:"delegates,omitempty"`
	POP                 *gmail.PopSettings         `json:"pop,omitempty"`
	IMAP                *gmail.ImapSettings        `json:"imap,omitempty"`
	Language            *gmail.LanguageSettings    `json:"language,omitempty"`
	Errors              []gmailSettingsBackupError `json:"errors,omitempty"`
}

type gmailSettingsBackupError struct {
	Kind  string `json:"kind"`
	Error string `json:"error"`
}

func buildGmailSettingsBackupSnapshot(ctx context.Context, flags *RootFlags, _ int) (backup.Snapshot, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	svc, err := newGmailService(ctx, account)
	if err != nil {
		return backup.Snapshot{}, err
	}
	accountHash := backupAccountHash(account)
	settings := fetchGmailSettingsBackup(ctx, svc)
	shard, err := backup.NewJSONLShard(backupServiceGmailSettings, "settings", accountHash, fmt.Sprintf("data/gmail-settings/%s/settings.jsonl.gz.age", accountHash), []gmailSettingsBackup{settings})
	if err != nil {
		return backup.Snapshot{}, err
	}
	return backup.Snapshot{
		Services: []string{backupServiceGmailSettings},
		Accounts: []string{accountHash},
		Counts: map[string]int{
			"gmail-settings.settings":             1,
			"gmail-settings.filters":              len(settings.Filters),
			"gmail-settings.forwarding-addresses": len(settings.ForwardingAddresses),
			"gmail-settings.send-as":              len(settings.SendAs),
			"gmail-settings.delegates":            len(settings.Delegates),
			"gmail-settings.errors":               len(settings.Errors),
		},
		Shards: []backup.PlainShard{shard},
	}, nil
}

func fetchGmailSettingsBackup(ctx context.Context, svc *gmail.Service) gmailSettingsBackup {
	var out gmailSettingsBackup
	record := func(kind string, err error) {
		if err == nil {
			return
		}
		out.Errors = append(out.Errors, gmailSettingsBackupError{Kind: kind, Error: err.Error()})
	}
	if resp, err := svc.Users.Settings.Filters.List("me").Context(ctx).Do(); err == nil {
		out.Filters = resp.Filter
	} else {
		record("filters", err)
	}
	if resp, err := svc.Users.Settings.ForwardingAddresses.List("me").Context(ctx).Do(); err == nil {
		out.ForwardingAddresses = resp.ForwardingAddresses
	} else {
		record("forwarding-addresses", err)
	}
	if resp, err := svc.Users.Settings.GetAutoForwarding("me").Context(ctx).Do(); err == nil {
		out.AutoForwarding = resp
	} else {
		record("auto-forwarding", err)
	}
	if resp, err := svc.Users.Settings.SendAs.List("me").Context(ctx).Do(); err == nil {
		out.SendAs = resp.SendAs
	} else {
		record("send-as", err)
	}
	if resp, err := svc.Users.Settings.GetVacation("me").Context(ctx).Do(); err == nil {
		out.Vacation = resp
	} else {
		record("vacation", err)
	}
	if resp, err := svc.Users.Settings.Delegates.List("me").Context(ctx).Do(); err == nil {
		out.Delegates = resp.Delegates
	} else if !strings.Contains(strings.ToLower(err.Error()), "forbidden") {
		record("delegates", err)
	}
	if resp, err := svc.Users.Settings.GetPop("me").Context(ctx).Do(); err == nil {
		out.POP = resp
	} else {
		record("pop", err)
	}
	if resp, err := svc.Users.Settings.GetImap("me").Context(ctx).Do(); err == nil {
		out.IMAP = resp
	} else {
		record("imap", err)
	}
	if resp, err := svc.Users.Settings.GetLanguage("me").Context(ctx).Do(); err == nil {
		out.Language = resp
	} else {
		record("language", err)
	}
	return out
}
