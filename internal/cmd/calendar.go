package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarCmd struct {
	Calendars       CalendarCalendarsCmd       `cmd:"" name:"calendars" help:"List calendars"`
	ACL             CalendarAclCmd             `cmd:"" name:"acl" help:"List calendar ACL"`
	Events          CalendarEventsCmd          `cmd:"" name:"events" aliases:"list" help:"List events from a calendar or all calendars"`
	Event           CalendarEventCmd           `cmd:"" name:"event" help:"Get event"`
	Create          CalendarCreateCmd          `cmd:"" name:"create" help:"Create an event"`
	Update          CalendarUpdateCmd          `cmd:"" name:"update" help:"Update an event"`
	Delete          CalendarDeleteCmd          `cmd:"" name:"delete" help:"Delete an event"`
	FreeBusy        CalendarFreeBusyCmd        `cmd:"" name:"freebusy" help:"Get free/busy"`
	Respond         CalendarRespondCmd         `cmd:"" name:"respond" help:"Respond to an event invitation"`
	Colors          CalendarColorsCmd          `cmd:"" name:"colors" help:"Show calendar colors"`
	Conflicts       CalendarConflictsCmd       `cmd:"" name:"conflicts" help:"Find conflicts"`
	Search          CalendarSearchCmd          `cmd:"" name:"search" help:"Search events"`
	Time            CalendarTimeCmd            `cmd:"" name:"time" help:"Show server time"`
	Users           CalendarUsersCmd           `cmd:"" name:"users" help:"List workspace users (use their email as calendar ID)"`
	Team            CalendarTeamCmd            `cmd:"" name:"team" help:"Show events for all members of a Google Group"`
	FocusTime       CalendarFocusTimeCmd       `cmd:"" name:"focus-time" help:"Create a Focus Time block"`
	OOO             CalendarOOOCmd             `cmd:"" name:"out-of-office" aliases:"ooo" help:"Create an Out of Office event"`
	WorkingLocation CalendarWorkingLocationCmd `cmd:"" name:"working-location" aliases:"wl" help:"Set working location (home/office/custom)"`
}

type CalendarCalendarsCmd struct {
	Max  int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page string `name:"page" help:"Page token"`
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

	resp, err := svc.CalendarList.List().MaxResults(c.Max).PageToken(c.Page).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"calendars":     resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No calendars")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tROLE")
	for _, cal := range resp.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\n", cal.Id, cal.Summary, cal.AccessRole)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type CalendarAclCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page       string `name:"page" help:"Page token"`
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

	resp, err := svc.Acl.List(calendarID).MaxResults(c.Max).PageToken(c.Page).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"rules":         resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Items) == 0 {
		u.Err().Println("No ACL rules")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "SCOPE_TYPE\tSCOPE_VALUE\tROLE")
	for _, rule := range resp.Items {
		scopeType := ""
		scopeValue := ""
		if rule.Scope != nil {
			scopeType = rule.Scope.Type
			scopeValue = rule.Scope.Value
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", scopeType, scopeValue, rule.Role)
	}
	printNextPageHint(u, resp.NextPageToken)
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
	Page              string `name:"page" help:"Page token"`
	Query             string `name:"query" help:"Free text search"`
	All               bool   `name:"all" help:"Fetch events from all calendars"`
	PrivatePropFilter string `name:"private-prop-filter" help:"Filter by private extended property (key=value)"`
	SharedPropFilter  string `name:"shared-prop-filter" help:"Filter by shared extended property (key=value)"`
	Fields            string `name:"fields" help:"Comma-separated fields to return"`
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
		calendarID = "primary"
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
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
		return listAllCalendarsEvents(ctx, svc, from, to, c.Max, c.Page, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields)
	}
	return listCalendarEvents(ctx, svc, calendarID, from, to, c.Max, c.Page, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields)
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
	eventID := strings.TrimSpace(c.EventID)
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

	event, err := svc.Events.Get(calendarID, eventID).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"event": event})
	}
	printCalendarEvent(u, event)
	return nil
}
