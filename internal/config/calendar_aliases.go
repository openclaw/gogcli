package config

import "strings"

func NormalizeCalendarAlias(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}

func ResolveCalendarAlias(alias string) (string, bool, error) {
	alias = NormalizeCalendarAlias(alias)
	if alias == "" {
		return "", false, nil
	}

	cfg, err := ReadConfig()
	if err != nil {
		return "", false, err
	}

	if cfg.CalendarAliases == nil {
		return "", false, nil
	}

	calendarID, ok := cfg.CalendarAliases[alias]

	return calendarID, ok, nil
}

// ResolveCalendarID resolves a calendar ID, checking aliases first.
// If the input matches an alias, returns the mapped calendar ID.
// Otherwise returns the input unchanged.
func ResolveCalendarID(calendarID string) (string, error) {
	calendarID = strings.TrimSpace(calendarID)
	if calendarID == "" {
		return "primary", nil
	}

	resolved, ok, err := ResolveCalendarAlias(calendarID)
	if err != nil {
		return "", err
	}

	if ok {
		return resolved, nil
	}

	return calendarID, nil
}

func SetCalendarAlias(alias, calendarID string) error {
	alias = NormalizeCalendarAlias(alias)
	calendarID = strings.TrimSpace(calendarID)

	cfg, err := ReadConfig()
	if err != nil {
		return err
	}

	if cfg.CalendarAliases == nil {
		cfg.CalendarAliases = map[string]string{}
	}

	cfg.CalendarAliases[alias] = calendarID

	return WriteConfig(cfg)
}

func DeleteCalendarAlias(alias string) (bool, error) {
	alias = NormalizeCalendarAlias(alias)

	cfg, err := ReadConfig()
	if err != nil {
		return false, err
	}

	if cfg.CalendarAliases == nil {
		return false, nil
	}

	if _, ok := cfg.CalendarAliases[alias]; !ok {
		return false, nil
	}

	delete(cfg.CalendarAliases, alias)

	return true, WriteConfig(cfg)
}

func ListCalendarAliases() (map[string]string, error) {
	cfg, err := ReadConfig()
	if err != nil {
		return nil, err
	}

	if cfg.CalendarAliases == nil {
		return map[string]string{}, nil
	}

	out := make(map[string]string, len(cfg.CalendarAliases))
	for k, v := range cfg.CalendarAliases {
		out[k] = v
	}

	return out, nil
}
