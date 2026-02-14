package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarCmd struct {
	Calendars       CalendarCalendarsCmd       `cmd:"" name:"calendars" help:"List calendars"`
	ACL             CalendarAclCmd             `cmd:"" name:"acl" aliases:"permissions,perms" help:"List calendar ACL"`
	Events          CalendarEventsCmd          `cmd:"" name:"events" aliases:"list,ls" help:"List events from a calendar or all calendars"`
	Event           CalendarEventCmd           `cmd:"" name:"event" aliases:"get,info,show" help:"Get event"`
	Create          CalendarCreateCmd          `cmd:"" name:"create" aliases:"add,new" help:"Create an event"`
	Update          CalendarUpdateCmd          `cmd:"" name:"update" aliases:"edit,set" help:"Update an event"`
	Delete          CalendarDeleteCmd          `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete an event"`
	FreeBusy        CalendarFreeBusyCmd        `cmd:"" name:"freebusy" help:"Get free/busy"`
	Respond         CalendarRespondCmd         `cmd:"" name:"respond" aliases:"rsvp,reply" help:"Respond to an event invitation"`
	ProposeTime     CalendarProposeTimeCmd     `cmd:"" name:"propose-time" help:"Generate URL to propose a new meeting time (browser-only feature)"`
	Colors          CalendarColorsCmd          `cmd:"" name:"colors" help:"Show calendar colors"`
	Conflicts       CalendarConflictsCmd       `cmd:"" name:"conflicts" help:"Find conflicts"`
	Search          CalendarSearchCmd          `cmd:"" name:"search" aliases:"find,query" help:"Search events"`
	Time            CalendarTimeCmd            `cmd:"" name:"time" help:"Show server time"`
	Users           CalendarUsersCmd           `cmd:"" name:"users" help:"List workspace users (use their email as calendar ID)"`
	Team            CalendarTeamCmd            `cmd:"" name:"team" help:"Show events for all members of a Google Group"`
	FocusTime       CalendarFocusTimeCmd       `cmd:"" name:"focus-time" aliases:"focus" help:"Create a Focus Time block"`
	OOO             CalendarOOOCmd             `cmd:"" name:"out-of-office" aliases:"ooo" help:"Create an Out of Office event"`
	WorkingLocation CalendarWorkingLocationCmd `cmd:"" name:"working-location" aliases:"wl" help:"Set working location (home/office/custom)"`
}

type CalendarCalendarsCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *CalendarCalendarsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*calendar.CalendarListEntry, string, error) {
		call := svc.CalendarList.List().MaxResults(c.Max)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		r, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return r.Items, r.NextPageToken, nil
	}

	var items []*calendar.CalendarListEntry
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		items = all
	} else {
		var err error
		items, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"calendars":     items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}
	if len(items) == 0 {
		u.Err().Println("No calendars")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tROLE")
	for _, cal := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\n", cal.Id, cal.Summary, cal.AccessRole)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type CalendarAclCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page       string `name:"page" aliases:"cursor" help:"Page token"`
	All        bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty  bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *CalendarAclCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}
	calendarID, err = resolveCalendarID(ctx, svc, calendarID)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*calendar.AclRule, string, error) {
		call := svc.Acl.List(calendarID).MaxResults(c.Max)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		r, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return r.Items, r.NextPageToken, nil
	}

	var items []*calendar.AclRule
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		items = all
	} else {
		var err error
		items, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"rules":         items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}
	if len(items) == 0 {
		u.Err().Println("No ACL rules")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "SCOPE_TYPE\tSCOPE_VALUE\tROLE")
	for _, rule := range items {
		scopeType := ""
		scopeValue := ""
		if rule.Scope != nil {
			scopeType = rule.Scope.Type
			scopeValue = rule.Scope.Value
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", scopeType, scopeValue, rule.Role)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type CalendarEventsCmd struct {
	CalendarID        string `arg:"" name:"calendarId" optional:"" help:"Calendar ID (default: primary)"`
	From              string `name:"from" help:"Start time (RFC3339, date, or relative: today, tomorrow, monday)"`
	To                string `name:"to" help:"End time (RFC3339, date, or relative)"`
	Today             bool   `name:"today" help:"Today only (timezone-aware)"`
	Tomorrow          bool   `name:"tomorrow" help:"Tomorrow only (timezone-aware)"`
	Week              bool   `name:"week" help:"This week (uses --week-start, default Mon)"`
	Days              int    `name:"days" help:"Next N days (timezone-aware)" default:"0"`
	WeekStart         string `name:"week-start" help:"Week start day for --week (sun, mon, ...)" default:""`
	Max               int64  `name:"max" aliases:"limit" help:"Max results" default:"10"`
	Page              string `name:"page" aliases:"cursor" help:"Page token"`
	AllPages          bool   `name:"all-pages" aliases:"allpages" help:"Fetch all pages"`
	FailEmpty         bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	Query             string `name:"query" help:"Free text search"`
	All               bool   `name:"all" help:"Fetch events from all calendars"`
	PrivatePropFilter string `name:"private-prop-filter" help:"Filter by private extended property (key=value)"`
	SharedPropFilter  string `name:"shared-prop-filter" help:"Filter by shared extended property (key=value)"`
	Fields            string `name:"fields" help:"Comma-separated fields to return"`
	Weekday           bool   `name:"weekday" help:"Include start/end day-of-week columns" default:"${calendar_weekday}"`
}

func (c *CalendarEventsCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	calendarID := strings.TrimSpace(c.CalendarID)
	if c.All && calendarID != "" {
		return usage("calendarId not allowed with --all flag")
	}
	if !c.All && calendarID == "" {
		calendarID = primaryCalendarID
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}
	if !c.All {
		calendarID, err = resolveCalendarID(ctx, svc, calendarID)
		if err != nil {
			return err
		}
	}

	// Use timezone-aware time resolution
	timeRange, err := ResolveTimeRange(ctx, svc, TimeRangeFlags{
		From:      c.From,
		To:        c.To,
		Today:     c.Today,
		Tomorrow:  c.Tomorrow,
		Week:      c.Week,
		Days:      c.Days,
		WeekStart: c.WeekStart,
	})
	if err != nil {
		return err
	}

	from, to := timeRange.FormatRFC3339()

	if c.All {
		return listAllCalendarsEvents(ctx, svc, from, to, c.Max, c.Page, c.AllPages, c.FailEmpty, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields, c.Weekday)
	}
	return listCalendarEvents(ctx, svc, calendarID, from, to, c.Max, c.Page, c.AllPages, c.FailEmpty, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields, c.Weekday)
}

type CalendarEventCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	EventID    string `arg:"" name:"eventId" help:"Event ID"`
}

func (c *CalendarEventCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	eventID := normalizeCalendarEventID(c.EventID)
	if calendarID == "" {
		return usage("empty calendarId")
	}
	if eventID == "" {
		return usage("empty eventId")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}
	calendarID, err = resolveCalendarID(ctx, svc, calendarID)
	if err != nil {
		return err
	}

	event, err := svc.Events.Get(calendarID, eventID).Do()
	if err != nil {
		return err
	}
	tz, loc, _ := getCalendarLocation(ctx, svc, calendarID)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"event": wrapEventWithDaysWithTimezone(event, tz, loc)})
	}
	printCalendarEventWithTimezone(u, event, tz, loc)
	return nil
}
