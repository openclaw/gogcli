package cmd

import (
	"sort"
	"strings"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/people/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
)

type calendarAliasRow struct {
	Alias      string
	CalendarID string
}

type calendarFreeBusyRow struct {
	CalendarID string
	Start      string
	End        string
}

type calendarUserRow struct {
	Email string
	Name  string
}

type calendarTeamBusyResult struct {
	Email  string   `json:"email"`
	Busy   []string `json:"busy"`
	Errors []string `json:"errors,omitempty"`
}

func calendarListColumns() []outfmt.Column[*calendar.CalendarListEntry] {
	return []outfmt.Column[*calendar.CalendarListEntry]{
		{Header: "ID", Value: func(entry *calendar.CalendarListEntry) string { return entry.Id }},
		{Header: "NAME", Value: func(entry *calendar.CalendarListEntry) string { return entry.Summary }},
		{Header: "ROLE", Value: func(entry *calendar.CalendarListEntry) string { return entry.AccessRole }},
	}
}

func calendarACLColumns() []outfmt.Column[*calendar.AclRule] {
	return []outfmt.Column[*calendar.AclRule]{
		{Header: "SCOPE_TYPE", Value: func(rule *calendar.AclRule) string {
			if rule.Scope == nil {
				return ""
			}
			return rule.Scope.Type
		}},
		{Header: "SCOPE_VALUE", Value: func(rule *calendar.AclRule) string {
			if rule.Scope == nil {
				return ""
			}
			return rule.Scope.Value
		}},
		{Header: "ROLE", Value: func(rule *calendar.AclRule) string { return rule.Role }},
	}
}

func calendarAliasColumns() []outfmt.Column[calendarAliasRow] {
	return []outfmt.Column[calendarAliasRow]{
		{Header: "ALIAS", Value: func(row calendarAliasRow) string { return row.Alias }},
		{Header: "CALENDAR_ID", Value: func(row calendarAliasRow) string { return row.CalendarID }},
	}
}

func calendarAliasRows(aliases map[string]string) []calendarAliasRow {
	keys := make([]string, 0, len(aliases))
	for alias := range aliases {
		keys = append(keys, alias)
	}
	sort.Strings(keys)

	rows := make([]calendarAliasRow, 0, len(keys))
	for _, alias := range keys {
		rows = append(rows, calendarAliasRow{Alias: alias, CalendarID: aliases[alias]})
	}
	return rows
}

func calendarFreeBusyColumns() []outfmt.Column[calendarFreeBusyRow] {
	return []outfmt.Column[calendarFreeBusyRow]{
		{Header: "CALENDAR", Value: func(row calendarFreeBusyRow) string { return row.CalendarID }},
		{Header: "START", Value: func(row calendarFreeBusyRow) string { return row.Start }},
		{Header: "END", Value: func(row calendarFreeBusyRow) string { return row.End }},
	}
}

func calendarFreeBusyRows(calendars map[string]calendar.FreeBusyCalendar) []calendarFreeBusyRow {
	rows := make([]calendarFreeBusyRow, 0)
	for calendarID, data := range calendars {
		for _, busy := range data.Busy {
			if busy != nil {
				rows = append(rows, calendarFreeBusyRow{
					CalendarID: calendarID,
					Start:      busy.Start,
					End:        busy.End,
				})
			}
		}
	}
	return rows
}

func calendarUserColumns() []outfmt.Column[calendarUserRow] {
	return []outfmt.Column[calendarUserRow]{
		{Header: "EMAIL", Value: func(row calendarUserRow) string { return sanitizeTab(row.Email) }},
		{Header: "NAME", Value: func(row calendarUserRow) string { return sanitizeTab(row.Name) }},
	}
}

func calendarUserRows(peopleList []*people.Person) []calendarUserRow {
	rows := make([]calendarUserRow, 0, len(peopleList))
	for _, person := range peopleList {
		email := primaryEmail(person)
		if email != "" {
			rows = append(rows, calendarUserRow{Email: email, Name: primaryName(person)})
		}
	}
	return rows
}

func calendarTeamBusyColumns() []outfmt.Column[calendarTeamBusyResult] {
	return []outfmt.Column[calendarTeamBusyResult]{
		{Header: "WHO", Value: func(result calendarTeamBusyResult) string {
			return sanitizeTab(result.Email)
		}},
		{Header: "BUSY BLOCKS", Value: func(result calendarTeamBusyResult) string {
			value := strings.Join(result.Busy, ", ")
			if value == "" {
				value = "(free)"
			}
			if len(result.Errors) > 0 {
				value = "error: " + strings.Join(result.Errors, ", ")
			}
			return sanitizeTab(value)
		}},
	}
}

func calendarTeamEventColumns() []outfmt.Column[teamEvent] {
	return []outfmt.Column[teamEvent]{
		{Header: "WHO", Value: func(event teamEvent) string { return sanitizeTab(event.Who) }},
		{Header: "START", Value: func(event teamEvent) string { return sanitizeTab(event.Start) }},
		{Header: "END", Value: func(event teamEvent) string { return sanitizeTab(event.End) }},
		{Header: "SUMMARY", Value: func(event teamEvent) string {
			return sanitizeTab(truncate(event.Summary, 40))
		}},
	}
}

func calendarEventColumns(includeCalendar, showWeekday, showLocation bool) []outfmt.Column[*eventWithCalendar] {
	columns := make([]outfmt.Column[*eventWithCalendar], 0, 8)
	if includeCalendar {
		columns = append(columns, outfmt.Column[*eventWithCalendar]{
			Header: "CALENDAR",
			Value:  func(event *eventWithCalendar) string { return event.CalendarID },
		})
	}
	columns = append(columns,
		outfmt.Column[*eventWithCalendar]{
			Header: "ID",
			Value: func(event *eventWithCalendar) string {
				return calendarEvent(event).Id
			},
		},
		outfmt.Column[*eventWithCalendar]{
			Header: "START",
			Value:  eventDisplayStart,
		},
	)
	if showWeekday {
		columns = append(columns, outfmt.Column[*eventWithCalendar]{
			Header: "START_DOW",
			Value: func(event *eventWithCalendar) string {
				startDay, _ := calendarEventWeekdays(event, includeCalendar)
				return startDay
			},
		})
	}
	columns = append(columns, outfmt.Column[*eventWithCalendar]{
		Header: "END",
		Value:  eventDisplayEnd,
	})
	if showWeekday {
		columns = append(columns, outfmt.Column[*eventWithCalendar]{
			Header: "END_DOW",
			Value: func(event *eventWithCalendar) string {
				_, endDay := calendarEventWeekdays(event, includeCalendar)
				return endDay
			},
		})
	}
	columns = append(columns, outfmt.Column[*eventWithCalendar]{
		Header: "SUMMARY",
		Value: func(event *eventWithCalendar) string {
			return calendarEvent(event).Summary
		},
	})
	if showLocation {
		columns = append(columns, outfmt.Column[*eventWithCalendar]{
			Header: "LOCATION",
			Value:  eventDisplayLocation,
		})
	}
	return columns
}

func compactCalendarRows[T any](rows []*T) []*T {
	filtered := make([]*T, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func calendarEvent(event *eventWithCalendar) *calendar.Event {
	if event == nil || event.Event == nil {
		return &calendar.Event{}
	}
	return event.Event
}

func calendarEventWeekdays(event *eventWithCalendar, includeCalendar bool) (string, string) {
	if event == nil {
		return "", ""
	}
	startDay, endDay := event.StartDayOfWeek, event.EndDayOfWeek
	if !includeCalendar && startDay == "" && endDay == "" {
		return eventDaysOfWeek(event.Event)
	}
	return startDay, endDay
}
