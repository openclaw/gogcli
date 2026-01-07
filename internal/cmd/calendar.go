package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newCalendarService = googleapi.NewCalendar

type CalendarCmd struct {
	Calendars CalendarCalendarsCmd `cmd:"" name:"calendars" help:"List calendars"`
	ACL       CalendarAclCmd       `cmd:"" name:"acl" help:"List calendar ACL"`
	Events    CalendarEventsCmd    `cmd:"" name:"events" help:"List events from a calendar or all calendars"`
	Event     CalendarEventCmd     `cmd:"" name:"event" help:"Get event"`
	Create    CalendarCreateCmd    `cmd:"" name:"create" help:"Create an event"`
	Update    CalendarUpdateCmd    `cmd:"" name:"update" help:"Update an event"`
	Delete    CalendarDeleteCmd    `cmd:"" name:"delete" help:"Delete an event"`
	FreeBusy  CalendarFreeBusyCmd  `cmd:"" name:"freebusy" help:"Get free/busy"`
	Respond   CalendarRespondCmd   `cmd:"" name:"respond" help:"Respond to an event invitation"`
	Colors    CalendarColorsCmd    `cmd:"" name:"colors" help:"Show calendar colors"`
	Conflicts CalendarConflictsCmd `cmd:"" name:"conflicts" help:"Find conflicts"`
	Search    CalendarSearchCmd    `cmd:"" name:"search" help:"Search events"`
	Time      CalendarTimeCmd      `cmd:"" name:"time" help:"Show server time"`
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
	CalendarID string `arg:"" name:"calendarId" optional:"" help:"Calendar ID"`
	From       string `name:"from" help:"Start time (RFC3339; default: now)"`
	To         string `name:"to" help:"End time (RFC3339; default: +7d)"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results" default:"10"`
	Page       string `name:"page" help:"Page token"`
	Query      string `name:"query" help:"Free text search"`
	All        bool   `name:"all" help:"Fetch events from all calendars"`
}

func (c *CalendarEventsCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if !c.All && strings.TrimSpace(c.CalendarID) == "" {
		return usage("calendarId required unless --all is specified")
	}
	if c.All && strings.TrimSpace(c.CalendarID) != "" {
		return usage("calendarId not allowed with --all flag")
	}

	now := time.Now().UTC()
	oneWeekLater := now.Add(7 * 24 * time.Hour)
	from := strings.TrimSpace(c.From)
	to := strings.TrimSpace(c.To)
	if from == "" {
		from = now.Format(time.RFC3339)
	}
	if to == "" {
		to = oneWeekLater.Format(time.RFC3339)
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	if c.All {
		return listAllCalendarsEvents(ctx, svc, from, to, c.Max, c.Page, c.Query)
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	return listCalendarEvents(ctx, svc, calendarID, from, to, c.Max, c.Page, c.Query)
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

type CalendarCreateCmd struct {
	CalendarID  string `arg:"" name:"calendarId" help:"Calendar ID"`
	Summary     string `name:"summary" help:"Event summary/title"`
	From        string `name:"from" help:"Start time (RFC3339)"`
	To          string `name:"to" help:"End time (RFC3339)"`
	Description string `name:"description" help:"Description"`
	Location    string `name:"location" help:"Location"`
	Attendees   string `name:"attendees" help:"Comma-separated attendee emails"`
	AllDay      bool   `name:"all-day" help:"All-day event (use date-only in --from/--to)"`
}

func (c *CalendarCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("empty calendarId")
	}

	if strings.TrimSpace(c.Summary) == "" || strings.TrimSpace(c.From) == "" || strings.TrimSpace(c.To) == "" {
		return usage("required: --summary, --from, --to")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	event := &calendar.Event{
		Summary:     strings.TrimSpace(c.Summary),
		Description: strings.TrimSpace(c.Description),
		Location:    strings.TrimSpace(c.Location),
		Start:       buildEventDateTime(c.From, c.AllDay),
		End:         buildEventDateTime(c.To, c.AllDay),
		Attendees:   buildAttendees(c.Attendees),
	}

	created, err := svc.Events.Insert(calendarID, event).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"event": created})
	}
	printCalendarEvent(u, created)
	return nil
}

type CalendarUpdateCmd struct {
	CalendarID  string `arg:"" name:"calendarId" help:"Calendar ID"`
	EventID     string `arg:"" name:"eventId" help:"Event ID"`
	Summary     string `name:"summary" help:"New summary/title (set empty to clear)"`
	From        string `name:"from" help:"New start time (RFC3339; set empty to clear)"`
	To          string `name:"to" help:"New end time (RFC3339; set empty to clear)"`
	Description string `name:"description" help:"New description (set empty to clear)"`
	Location    string `name:"location" help:"New location (set empty to clear)"`
	Attendees   string `name:"attendees" help:"Comma-separated attendee emails (replaces all; set empty to clear)"`
	AddAttendee string `name:"add-attendee" help:"Comma-separated attendee emails to add (preserves existing attendees)"`
	AllDay      bool   `name:"all-day" help:"All-day event (use date-only in --from/--to)"`
}

func (c *CalendarUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
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

	// If --all-day changed, require from/to to update both date/time fields.
	if flagProvided(kctx, "all-day") {
		if !flagProvided(kctx, "from") || !flagProvided(kctx, "to") {
			return usage("when changing --all-day, also provide --from and --to")
		}
	}

	// Cannot use both --attendees and --add-attendee at the same time.
	if flagProvided(kctx, "attendees") && flagProvided(kctx, "add-attendee") {
		return usage("cannot use both --attendees and --add-attendee; use --attendees to replace all, or --add-attendee to add")
	}

	patch := &calendar.Event{}
	changed := false
	if flagProvided(kctx, "summary") {
		patch.Summary = strings.TrimSpace(c.Summary)
		changed = true
	}
	if flagProvided(kctx, "description") {
		patch.Description = strings.TrimSpace(c.Description)
		changed = true
	}
	if flagProvided(kctx, "location") {
		patch.Location = strings.TrimSpace(c.Location)
		changed = true
	}
	if flagProvided(kctx, "from") {
		patch.Start = buildEventDateTime(c.From, c.AllDay)
		changed = true
	}
	if flagProvided(kctx, "to") {
		patch.End = buildEventDateTime(c.To, c.AllDay)
		changed = true
	}
	if flagProvided(kctx, "attendees") {
		patch.Attendees = buildAttendees(c.Attendees)
		changed = true
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	// For --add-attendee, fetch current event to preserve existing attendees with metadata.
	if flagProvided(kctx, "add-attendee") {
		var existing *calendar.Event
		existing, err = svc.Events.Get(calendarID, eventID).Do()
		if err != nil {
			return fmt.Errorf("failed to fetch current event: %w", err)
		}
		patch.Attendees = mergeAttendees(existing.Attendees, c.AddAttendee)
		changed = true
	}

	if !changed {
		return usage("no updates provided")
	}

	updated, err := svc.Events.Patch(calendarID, eventID, patch).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"event": updated})
	}
	printCalendarEvent(u, updated)
	return nil
}

type CalendarDeleteCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	EventID    string `arg:"" name:"eventId" help:"Event ID"`
}

func (c *CalendarDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete event %s from calendar %s", eventID, calendarID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Events.Delete(calendarID, eventID).Do(); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted":    true,
			"calendarId": calendarID,
			"eventId":    eventID,
		})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("calendarId\t%s", calendarID)
	u.Out().Printf("eventId\t%s", eventID)
	return nil
}

type CalendarFreeBusyCmd struct {
	CalendarIDs string `arg:"" name:"calendarIds" help:"Comma-separated calendar IDs"`
	From        string `name:"from" help:"Start time (RFC3339, required)"`
	To          string `name:"to" help:"End time (RFC3339, required)"`
}

func (c *CalendarFreeBusyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	calendarIDs := splitCSV(c.CalendarIDs)
	if len(calendarIDs) == 0 {
		return usage("no calendar IDs provided")
	}
	if strings.TrimSpace(c.From) == "" || strings.TrimSpace(c.To) == "" {
		return usage("required: --from and --to")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	req := &calendar.FreeBusyRequest{
		TimeMin: c.From,
		TimeMax: c.To,
		Items:   make([]*calendar.FreeBusyRequestItem, 0, len(calendarIDs)),
	}
	for _, id := range calendarIDs {
		req.Items = append(req.Items, &calendar.FreeBusyRequestItem{Id: id})
	}

	resp, err := svc.Freebusy.Query(req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"calendars": resp.Calendars})
	}

	if len(resp.Calendars) == 0 {
		u.Err().Println("No free/busy data")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "CALENDAR\tSTART\tEND")
	for id, data := range resp.Calendars {
		for _, b := range data.Busy {
			fmt.Fprintf(w, "%s\t%s\t%s\n", id, b.Start, b.End)
		}
	}
	return nil
}

func listCalendarEvents(ctx context.Context, svc *calendar.Service, calendarID, from, to string, maxResults int64, page, query string) error {
	u := ui.FromContext(ctx)

	call := svc.Events.List(calendarID).
		TimeMin(from).
		TimeMax(to).
		MaxResults(maxResults).
		PageToken(page).
		SingleEvents(true).
		OrderBy("startTime")
	if strings.TrimSpace(query) != "" {
		call = call.Q(query)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"events":        resp.Items,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Items) == 0 {
		u.Err().Println("No events")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintln(w, "ID\tSTART\tEND\tSUMMARY")
	for _, e := range resp.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Id, eventStart(e), eventEnd(e), e.Summary)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type eventWithCalendar struct {
	*calendar.Event
	CalendarID string
}

func listAllCalendarsEvents(ctx context.Context, svc *calendar.Service, from, to string, maxResults int64, page, query string) error {
	u := ui.FromContext(ctx)

	calResp, err := svc.CalendarList.List().Context(ctx).Do()
	if err != nil {
		return err
	}

	if len(calResp.Items) == 0 {
		u.Err().Println("No calendars")
		return nil
	}

	all := []*eventWithCalendar{}
	for _, c := range calResp.Items {
		events, err := svc.Events.List(c.Id).
			TimeMin(from).
			TimeMax(to).
			MaxResults(maxResults).
			PageToken(page).
			SingleEvents(true).
			OrderBy("startTime").
			Q(query).
			Context(ctx).
			Do()
		if err != nil {
			u.Err().Printf("calendar %s: %v", c.Id, err)
			continue
		}
		for _, e := range events.Items {
			all = append(all, &eventWithCalendar{Event: e, CalendarID: c.Id})
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"events": all})
	}
	if len(all) == 0 {
		u.Err().Println("No events")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "CALENDAR\tID\tSTART\tEND\tSUMMARY")
	for _, e := range all {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.CalendarID, e.Id, eventStart(e.Event), eventEnd(e.Event), e.Summary)
	}
	return nil
}

func printCalendarEvent(u *ui.UI, event *calendar.Event) {
	if u == nil || event == nil {
		return
	}
	u.Out().Printf("id\t%s", event.Id)
	u.Out().Printf("summary\t%s", orEmpty(event.Summary, "(no title)"))
	u.Out().Printf("start\t%s", eventStart(event))
	u.Out().Printf("end\t%s", eventEnd(event))
	if event.Description != "" {
		u.Out().Printf("description\t%s", event.Description)
	}
	if event.Location != "" {
		u.Out().Printf("location\t%s", event.Location)
	}
	if len(event.Attendees) > 0 {
		emails := []string{}
		for _, a := range event.Attendees {
			if a != nil && strings.TrimSpace(a.Email) != "" {
				emails = append(emails, strings.TrimSpace(a.Email))
			}
		}
		if len(emails) > 0 {
			u.Out().Printf("attendees\t%s", strings.Join(emails, ", "))
		}
	}
	if event.HtmlLink != "" {
		u.Out().Printf("link\t%s", event.HtmlLink)
	}
}

func buildEventDateTime(value string, allDay bool) *calendar.EventDateTime {
	value = strings.TrimSpace(value)
	if allDay {
		return &calendar.EventDateTime{Date: value}
	}
	return &calendar.EventDateTime{DateTime: value}
}

func buildAttendees(csv string) []*calendar.EventAttendee {
	addrs := splitCSV(csv)
	if len(addrs) == 0 {
		return nil
	}
	out := make([]*calendar.EventAttendee, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, &calendar.EventAttendee{Email: a})
	}
	return out
}

// mergeAttendees preserves existing attendees (with all their metadata like responseStatus)
// and adds new attendees from the CSV string. Duplicates (by email) are skipped.
func mergeAttendees(existing []*calendar.EventAttendee, addCSV string) []*calendar.EventAttendee {
	newEmails := splitCSV(addCSV)
	if len(newEmails) == 0 {
		return existing
	}

	// Build a set of existing emails for deduplication
	existingEmails := make(map[string]bool, len(existing))
	for _, a := range existing {
		if a != nil && a.Email != "" {
			existingEmails[strings.ToLower(a.Email)] = true
		}
	}

	// Start with existing attendees (preserving all metadata)
	out := make([]*calendar.EventAttendee, 0, len(existing)+len(newEmails))
	out = append(out, existing...)

	// Add new attendees that don't already exist
	for _, email := range newEmails {
		if !existingEmails[strings.ToLower(email)] {
			out = append(out, &calendar.EventAttendee{
				Email:          email,
				ResponseStatus: "needsAction",
			})
			existingEmails[strings.ToLower(email)] = true
		}
	}
	return out
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func eventStart(e *calendar.Event) string {
	if e == nil || e.Start == nil {
		return ""
	}
	if e.Start.DateTime != "" {
		return e.Start.DateTime
	}
	return e.Start.Date
}

func eventEnd(e *calendar.Event) string {
	if e == nil || e.End == nil {
		return ""
	}
	if e.End.DateTime != "" {
		return e.End.DateTime
	}
	return e.End.Date
}

func isAllDayEvent(e *calendar.Event) bool {
	return e != nil && e.Start != nil && e.Start.Date != ""
}

func orEmpty(s string, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
