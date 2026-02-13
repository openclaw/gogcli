package cmd

import "github.com/degree-analytics/ratatosk/internal/googleapi"

var newCalendarService = googleapi.NewCalendar

const (
	scopeAll    = "all"
	scopeSingle = "single"
	scopeFuture = "future"
)
