package cmd

type CalendarCmd struct {
	Calendars       CalendarCalendarsCmd       `cmd:"" name:"calendars" help:"List calendars"`
	Subscribe       CalendarSubscribeCmd       `cmd:"" name:"subscribe" aliases:"sub,add-calendar" help:"Add a calendar to your calendar list"`
	CreateCalendar  CalendarCreateCalendarCmd  `cmd:"" name:"create-calendar" aliases:"new-calendar" help:"Create a new secondary calendar"`
	ACL             CalendarAclCmd             `cmd:"" name:"acl" aliases:"permissions,perms" help:"List calendar ACL"`
	Alias           CalendarAliasCmd           `cmd:"" name:"alias" help:"Manage calendar aliases"`
	Events          CalendarEventsCmd          `cmd:"" name:"events" aliases:"list,ls" help:"List events from a calendar or all calendars"`
	Event           CalendarEventCmd           `cmd:"" name:"event" aliases:"get,info,show" help:"Get event"`
	Raw             CalendarRawCmd             `cmd:"" name:"raw" help:"Dump raw Google Calendar API response as JSON (Events.Get; lossless; for scripting and LLM consumption)"`
	Create          CalendarCreateCmd          `cmd:"" name:"create" aliases:"add,new" help:"Create an event"`
	Update          CalendarUpdateCmd          `cmd:"" name:"update" aliases:"edit,set" help:"Update an event"`
	Move            CalendarMoveCmd            `cmd:"" name:"move" aliases:"transfer" help:"Move an event to another calendar"`
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
