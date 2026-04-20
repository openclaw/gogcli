package config

import (
	"errors"
	"sort"
	"strings"
)

var errMissingAccount = errors.New("missing account")

func SetNoSendAccount(account string, disabled bool) error {
	account = normalizeNoSendAccount(account)
	if account == "" {
		return errMissingAccount
	}

	return UpdateConfig(func(cfg *File) error {
		if disabled {
			if cfg.NoSendAccounts == nil {
				cfg.NoSendAccounts = make(map[string]bool)
			}
			cfg.NoSendAccounts[account] = true

			return nil
		}

		delete(cfg.NoSendAccounts, account)

		if len(cfg.NoSendAccounts) == 0 {
			cfg.NoSendAccounts = nil
		}

		return nil
	})
}

func IsNoSendAccount(account string) (bool, error) {
	cfg, err := ReadConfig()
	if err != nil {
		return false, err
	}

	account = normalizeNoSendAccount(account)
	if account == "" {
		return false, nil
	}

	return cfg.GmailNoSend || cfg.NoSendAccounts[account], nil
}

func NoSendAccountList(cfg File) []string {
	accounts := make([]string, 0, len(cfg.NoSendAccounts))
	for account, disabled := range cfg.NoSendAccounts {
		if disabled {
			accounts = append(accounts, account)
		}
	}

	sort.Strings(accounts)

	return accounts
}

func normalizeNoSendAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}
