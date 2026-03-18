package config

import "strings"

func accountAliasesField(cfg *File) *map[string]string {
	return &cfg.AccountAliases
}

func NormalizeAccountAlias(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}

func ResolveAccountAlias(alias string) (string, bool, error) {
	return resolveAliasValue(alias, NormalizeAccountAlias, accountAliasesField)
}

func SetAccountAlias(alias, email string) error {
	return setAliasValue(alias, email, NormalizeAccountAlias, func(in string) string {
		return strings.ToLower(strings.TrimSpace(in))
	}, func(string, string) error {
		return nil
	}, accountAliasesField)
}

func DeleteAccountAlias(alias string) (bool, error) {
	return deleteAliasValue(alias, NormalizeAccountAlias, accountAliasesField)
}

func ListAccountAliases() (map[string]string, error) {
	return listAliasValues(accountAliasesField)
}
