package config

type aliasMapField func(*File) *map[string]string

func resolveAliasValue(alias string, normalizeAlias func(string) string, field aliasMapField) (string, bool, error) {
	alias = normalizeAlias(alias)
	if alias == "" {
		return "", false, nil
	}

	cfg, err := ReadConfig()
	if err != nil {
		return "", false, err
	}

	aliases := *field(&cfg)
	if aliases == nil {
		return "", false, nil
	}

	value, ok := aliases[alias]

	return value, ok, nil
}

func setAliasValue(alias, value string, normalizeAlias func(string) string, normalizeValue func(string) string, validate func(string, string) error, field aliasMapField) error {
	alias = normalizeAlias(alias)
	value = normalizeValue(value)

	if err := validate(alias, value); err != nil {
		return err
	}

	return UpdateConfig(func(cfg *File) error {
		aliases := field(cfg)
		if *aliases == nil {
			*aliases = map[string]string{}
		}

		(*aliases)[alias] = value

		return nil
	})
}

func deleteAliasValue(alias string, normalizeAlias func(string) string, field aliasMapField) (bool, error) {
	alias = normalizeAlias(alias)

	deleted := false
	err := UpdateConfig(func(cfg *File) error {
		aliases := field(cfg)
		if *aliases == nil {
			return nil
		}

		if _, ok := (*aliases)[alias]; !ok {
			return nil
		}

		delete(*aliases, alias)
		deleted = true

		return nil
	})

	return deleted, err
}

func listAliasValues(field aliasMapField) (map[string]string, error) {
	cfg, err := ReadConfig()
	if err != nil {
		return nil, err
	}

	aliases := *field(&cfg)
	if aliases == nil {
		return map[string]string{}, nil
	}

	out := make(map[string]string, len(aliases))
	for k, v := range aliases {
		out[k] = v
	}

	return out, nil
}
