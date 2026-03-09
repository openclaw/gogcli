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

func ResolveCalendarAlias(alias string) (string, bool, error) {
	return resolveAliasValue(alias, NormalizeCalendarAlias, calendarAliasesField)
}

// ResolveCalendarID resolves a calendar ID, checking aliases first.
// If the input matches an alias, returns the mapped calendar ID.
// Otherwise returns the input unchanged.
func ResolveCalendarID(calendarID string) (string, error) {
	calendarID = strings.TrimSpace(calendarID)
	if calendarID == "" {
		return "", nil
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
	return setAliasValue(alias, calendarID, NormalizeCalendarAlias, strings.TrimSpace, func(alias, calendarID string) error {
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

func DeleteCalendarAlias(alias string) (bool, error) {
	return deleteAliasValue(alias, NormalizeCalendarAlias, calendarAliasesField)
}

func ListCalendarAliases() (map[string]string, error) {
	return listAliasValues(calendarAliasesField)
}
