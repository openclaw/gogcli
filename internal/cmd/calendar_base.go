package cmd

import "github.com/steipete/gogcli/internal/googleapi"

var newCalendarService = googleapi.NewCalendar

const (
	scopeAll    = literalAll
	scopeSingle = "single"
	scopeFuture = "future"
)
