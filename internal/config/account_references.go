package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var errMCPPolicyDestinationExists = errors.New("destination MCP policy already exists")

func (s *ConfigStore) MigrateAccountEmailReferences(oldEmail, newEmail string) error {
	oldEmail, newEmail = strings.ToLower(strings.TrimSpace(oldEmail)), strings.ToLower(strings.TrimSpace(newEmail))

	if oldEmail == "" || newEmail == "" || oldEmail == newEmail {
		return nil
	}

	return s.Update(func(cfg *File) error {
		for alias, target := range cfg.AccountAliases {
			if strings.EqualFold(target, oldEmail) {
				cfg.AccountAliases[alias] = newEmail
			}
		}

		if client, ok := cfg.AccountClients[oldEmail]; ok {
			cfg.AccountClients[newEmail] = client
			delete(cfg.AccountClients, oldEmail)
		}

		if cfg.MCP != nil {
			if err := migrateMCPAccountPolicy(cfg.MCP.Accounts, oldEmail, newEmail); err != nil {
				return err
			}
		}

		return nil
	})
}

func migrateMCPAccountPolicy(accounts map[string]MCPPolicy, oldEmail, newEmail string) error {
	var oldKey, newKey string

	for account := range accounts {
		normalized := strings.TrimSpace(account)

		if strings.EqualFold(normalized, oldEmail) {
			oldKey = account
		}

		if strings.EqualFold(normalized, newEmail) {
			newKey = account
		}
	}

	if oldKey == "" {
		return nil
	}

	oldPolicy := accounts[oldKey]

	if newKey != "" {
		newPolicy := accounts[newKey]
		if oldPolicy.AllowWrite != newPolicy.AllowWrite || !slices.Equal(oldPolicy.AllowTools, newPolicy.AllowTools) {
			return fmt.Errorf("migrate MCP policy from %s to %s: %w", oldEmail, newEmail, errMCPPolicyDestinationExists)
		}

		delete(accounts, oldKey)

		return nil
	}

	accounts[newEmail] = oldPolicy
	delete(accounts, oldKey)

	return nil
}
