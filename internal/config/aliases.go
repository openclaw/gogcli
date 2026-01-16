package config

import "strings"

func NormalizeAccountAlias(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}

func ResolveAccountAlias(alias string) (string, bool, error) {
	alias = NormalizeAccountAlias(alias)
	if alias == "" {
		return "", false, nil
	}

	cfg, err := ReadConfig()
	if err != nil {
		return "", false, err
	}

	if cfg.AccountAliases == nil {
		return "", false, nil
	}

	email, ok := cfg.AccountAliases[alias]

	return email, ok, nil
}

func SetAccountAlias(alias, email string) error {
	alias = NormalizeAccountAlias(alias)
	email = strings.ToLower(strings.TrimSpace(email))

	cfg, err := ReadConfig()
	if err != nil {
		return err
	}

	if cfg.AccountAliases == nil {
		cfg.AccountAliases = map[string]string{}
	}

	cfg.AccountAliases[alias] = email

	return WriteConfig(cfg)
}

func DeleteAccountAlias(alias string) (bool, error) {
	alias = NormalizeAccountAlias(alias)

	cfg, err := ReadConfig()
	if err != nil {
		return false, err
	}

	if cfg.AccountAliases == nil {
		return false, nil
	}

	if _, ok := cfg.AccountAliases[alias]; !ok {
		return false, nil
	}

	delete(cfg.AccountAliases, alias)

	return true, WriteConfig(cfg)
}

func ListAccountAliases() (map[string]string, error) {
	cfg, err := ReadConfig()
	if err != nil {
		return nil, err
	}

	if cfg.AccountAliases == nil {
		return map[string]string{}, nil
	}

	out := make(map[string]string, len(cfg.AccountAliases))
	for k, v := range cfg.AccountAliases {
		out[k] = v
	}

	return out, nil
}
