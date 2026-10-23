package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

// CalendarUpdateCalendarCmd patches settings on a calendar resource itself
// (not on an event). The calendar's own timeZone (calendars.get.timeZone,
// owner-only) drives the UTC offset the Calendar API serialises for every
// event on the calendar. The calendarList entry mirrors these fields read-only,
// so only the calendar resource is patched; `--check` reports both side by
// side.
type CalendarUpdateCalendarCmd struct {
	CalendarID  string  `arg:"" name:"calendarId" help:"Calendar ID or alias whose settings should be updated"`
	Summary     *string `name:"summary" help:"Calendar display name (nonempty; omit to preserve)"`
	Description *string `name:"description" help:"Calendar description (omit to preserve; pass '' to clear)"`
	Location    *string `name:"location" help:"Calendar location (omit to preserve; pass '' to clear)"`
	TimeZone    *string `name:"timezone" aliases:"tz" help:"IANA timezone for the calendar, e.g. America/Los_Angeles (omit to preserve)"`
	Check       bool    `name:"check" help:"Read-only: report the calendar and calendarList timezones and whether they diverge"`
}

type calendarSettingsPatch struct {
	summary     string
	description string
	location    string
	timeZone    string

	setSummary     bool
	setDescription bool
	setLocation    bool
	setTimeZone    bool
}

func (p calendarSettingsPatch) changed() bool {
	return p.setSummary || p.setDescription || p.setLocation || p.setTimeZone
}

func (p calendarSettingsPatch) applyCalendar(cal *calendar.Calendar) {
	if p.setSummary {
		cal.Summary = p.summary
		if p.summary == "" {
			cal.ForceSendFields = appendForceSendField(cal.ForceSendFields, "Summary")
		}
	}
	if p.setDescription {
		cal.Description = p.description
		if p.description == "" {
			cal.ForceSendFields = appendForceSendField(cal.ForceSendFields, "Description")
		}
	}
	if p.setLocation {
		cal.Location = p.location
		if p.location == "" {
			cal.ForceSendFields = appendForceSendField(cal.ForceSendFields, "Location")
		}
	}
	if p.setTimeZone {
		cal.TimeZone = p.timeZone
	}
}

type calendarUpdateCalendarResult struct {
	CalendarID string             `json:"calendarId"`
	Calendar   *calendar.Calendar `json:"calendar"`
}

type calendarUpdateCalendarCheckResult struct {
	CalendarID           string `json:"calendarId"`
	CalendarSummary      string `json:"calendarSummary"`
	CalendarTimeZone     string `json:"calendarTimeZone"`
	CalendarListSummary  string `json:"calendarListSummary"`
	CalendarListTimeZone string `json:"calendarListTimeZone"`
	TimeZonesMatch       bool   `json:"timeZonesMatch"`
}

func (c *CalendarUpdateCalendarCmd) settingsFromFlags() calendarSettingsPatch {
	var patch calendarSettingsPatch
	if c.Summary != nil {
		patch.setSummary = true
		patch.summary = strings.TrimSpace(*c.Summary)
	}
	if c.Description != nil {
		patch.setDescription = true
		patch.description = strings.TrimSpace(*c.Description)
	}
	if c.Location != nil {
		patch.setLocation = true
		patch.location = strings.TrimSpace(*c.Location)
	}
	if c.TimeZone != nil {
		patch.setTimeZone = true
		patch.timeZone = strings.TrimSpace(*c.TimeZone)
	}
	return patch
}

func (c *CalendarUpdateCalendarCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	preparedID, err := prepareCalendarID(store, calendarID, false)
	if err != nil {
		return err
	}

	if c.Check {
		if c.settingsFromFlags().changed() {
			return usage("--check is read-only; drop --summary/--description/--location/--timezone")
		}

		account, accountErr := requireAccount(flags)
		if accountErr != nil {
			return accountErr
		}
		svc, serviceErr := calendarService(ctx, account)
		if serviceErr != nil {
			return serviceErr
		}
		resolvedID, resolveErr := resolveCalendarID(ctx, svc, preparedID)
		if resolveErr != nil {
			return resolveErr
		}
		return checkCalendarSettings(ctx, u, svc, resolvedID)
	}

	patch := c.settingsFromFlags()
	if !patch.changed() {
		return usage("provide at least one of --summary, --description, --location, --timezone (or --check to inspect current settings)")
	}
	if patch.setTimeZone && patch.timeZone == "" {
		// An empty timeZone is indistinguishable from an omitted one once the
		// request is marshalled, so this would send a PATCH that changes
		// nothing while reporting success.
		return usage("--timezone cannot be empty; omit it to keep the current timezone")
	}

	if patch.setSummary && patch.summary == "" {
		return usage("--summary cannot be empty; omit it to keep the current name")
	}
	if patch.setTimeZone {
		if _, tzErr := loadTimezoneLocation(patch.timeZone); tzErr != nil {
			return usagef("invalid timezone %q: %v", patch.timeZone, tzErr)
		}
	}

	calPatch := &calendar.Calendar{}
	patch.applyCalendar(calPatch)

	action := fmt.Sprintf("update settings for calendar %s (%s)", preparedID, summarizeCalendarSettingsPatch(patch))
	if patch.setTimeZone {
		action += "; this retroactively changes the UTC offset the API returns for its events"
	}
	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "calendar.update-calendar", map[string]any{
		"calendar_id": preparedID,
		"calendar":    calPatch,
	}, action); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := calendarService(ctx, account)
	if err != nil {
		return err
	}
	resolvedID, err := resolveCalendarID(ctx, svc, preparedID)
	if err != nil {
		return err
	}

	updated, err := svc.Calendars.Patch(resolvedID, calPatch).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("patch calendar %s: %w", resolvedID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), calendarUpdateCalendarResult{CalendarID: resolvedID, Calendar: updated})
	}
	return writeResult(ctx, u,
		kv("calendar_id", resolvedID),
		kv("summary", oneLine(updated.Summary)),
		kv("timezone", updated.TimeZone),
	)
}

func summarizeCalendarSettingsPatch(patch calendarSettingsPatch) string {
	fields := make([]string, 0, 4)
	if patch.setSummary {
		fields = append(fields, "summary")
	}
	if patch.setDescription {
		fields = append(fields, "description")
	}
	if patch.setLocation {
		fields = append(fields, "location")
	}
	if patch.setTimeZone {
		fields = append(fields, "timezone="+patch.timeZone)
	}
	return "changing " + strings.Join(fields, ", ")
}

// checkCalendarSettings is the read side: it reports the calendar's own
// default timezone next to the calendarList entry's timezone so a stale
// default is visible instead of silently shifting every event's serialised
// offset.
func checkCalendarSettings(ctx context.Context, u *ui.UI, svc *calendar.Service, calendarID string) error {
	cal, err := svc.Calendars.Get(calendarID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get calendar %s: %w", calendarID, err)
	}
	entry, err := svc.CalendarList.Get(calendarID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get calendarList entry %s: %w", calendarID, err)
	}

	calendarTimeZone := strings.TrimSpace(cal.TimeZone)
	listTimeZone := strings.TrimSpace(entry.TimeZone)
	result := calendarUpdateCalendarCheckResult{
		CalendarID:           calendarID,
		CalendarSummary:      cal.Summary,
		CalendarTimeZone:     calendarTimeZone,
		CalendarListSummary:  entry.Summary,
		CalendarListTimeZone: listTimeZone,
		TimeZonesMatch:       calendarTimeZone == listTimeZone,
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}

	return writeResult(ctx, u,
		kv("calendar_id", calendarID),
		kv("calendar_summary", oneLine(result.CalendarSummary)),
		kv("calendar_timezone", calendarTimeZone),
		kv("calendar_list_summary", oneLine(result.CalendarListSummary)),
		kv("calendar_list_timezone", listTimeZone),
		kv("timezones_match", result.TimeZonesMatch),
	)
}
