package config

import "strings"

func accountAliasesField(cfg *File) *map[string]string {
	return &cfg.AccountAliases
}

func NormalizeAccountAlias(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}

func (s *ConfigStore) ResolveAccountAlias(alias string) (string, bool, error) {
	return s.resolveAliasValue(alias, NormalizeAccountAlias, accountAliasesField)
}

func (s *ConfigStore) SetAccountAlias(alias, email string) error {
	return s.setAliasValue(alias, email, NormalizeAccountAlias, func(in string) string {
		return strings.ToLower(strings.TrimSpace(in))
	}, func(string, string) error {
		return nil
	}, accountAliasesField)
}

func (s *ConfigStore) DeleteAccountAlias(alias string) (bool, error) {
	return s.deleteAliasValue(alias, NormalizeAccountAlias, accountAliasesField)
}

func (s *ConfigStore) ListAccountAliases() (map[string]string, error) {
	return s.listAliasValues(accountAliasesField)
}
