package cmd

import (
	"context"
)

const calendarAppointmentScheduleEventType = "appointmentSchedule"

type CalendarAppointmentsCmd struct {
	List CalendarAppointmentSchedulesListCmd `cmd:"" name:"list" aliases:"ls" help:"List appointment schedules"`
}

type CalendarAppointmentSchedulesListCmd struct {
	CalendarEventsCmd `embed:""`
}

func (c *CalendarAppointmentSchedulesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return c.run(ctx, flags, []string{calendarAppointmentScheduleEventType})
}
