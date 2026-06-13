package config

import "strings"

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

		return nil
	})
}
