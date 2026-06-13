package config

import (
	"errors"
	"strings"
)

func calendarAliasesField(cfg *File) *map[string]string {
	return &cfg.CalendarAliases
}

var (
	errCalendarAliasEmpty           = errors.New("calendar alias must not be empty")
	errCalendarAliasHasWhitespace   = errors.New("calendar alias must not contain whitespace")
	errCalendarAliasCalendarIDEmpty = errors.New("calendar ID must not be empty")
)

func NormalizeCalendarAlias(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}

func (s *ConfigStore) ResolveCalendarAlias(alias string) (string, bool, error) {
	return s.resolveAliasValue(alias, NormalizeCalendarAlias, calendarAliasesField)
}

// ResolveCalendarID resolves a calendar ID, checking aliases first.
// If the input matches an alias, returns the mapped calendar ID.
// Otherwise returns the input unchanged.
func (s *ConfigStore) ResolveCalendarID(calendarID string) (string, error) {
	calendarID = strings.TrimSpace(calendarID)
	if calendarID == "" {
		return "", nil
	}

	resolved, ok, err := s.ResolveCalendarAlias(calendarID)
	if err != nil {
		return "", err
	}

	if ok {
		return resolved, nil
	}

	return calendarID, nil
}

func (s *ConfigStore) SetCalendarAlias(alias, calendarID string) error {
	return s.setAliasValue(alias, calendarID, NormalizeCalendarAlias, strings.TrimSpace, func(alias, calendarID string) error {
		if alias == "" {
			return errCalendarAliasEmpty
		}

		if strings.ContainsAny(alias, " \t\r\n") {
			return errCalendarAliasHasWhitespace
		}

		if calendarID == "" {
			return errCalendarAliasCalendarIDEmpty
		}

		return nil
	}, calendarAliasesField)
}

func (s *ConfigStore) DeleteCalendarAlias(alias string) (bool, error) {
	return s.deleteAliasValue(alias, NormalizeCalendarAlias, calendarAliasesField)
}

func (s *ConfigStore) ListCalendarAliases() (map[string]string, error) {
	return s.listAliasValues(calendarAliasesField)
}
