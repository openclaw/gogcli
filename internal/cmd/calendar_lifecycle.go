package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/openclaw/gogcli/internal/ui"
)

type calendarLifecycleDelete struct {
	op          string
	action      func(string) string
	resultKey   string
	delete      func(*calendar.Service, string) error
	errorPrefix string
}

type CalendarUnsubscribeCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID or alias to remove from your calendar list"`
}

func (c *CalendarUnsubscribeCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCalendarLifecycleDelete(ctx, flags, c.CalendarID, calendarLifecycleDelete{
		op:        "calendar.unsubscribe",
		action:    func(id string) string { return fmt.Sprintf("unsubscribe from calendar %s", id) },
		resultKey: "unsubscribed",
		delete: func(svc *calendar.Service, id string) error {
			return svc.CalendarList.Delete(id).Context(ctx).Do()
		},
		errorPrefix: "unsubscribe from calendar",
	})
}

type CalendarDeleteCalendarCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Owned secondary calendar ID or alias"`
}

func (c *CalendarDeleteCalendarCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runCalendarLifecycleDelete(ctx, flags, c.CalendarID, calendarLifecycleDelete{
		op:        "calendar.delete-calendar",
		action:    func(id string) string { return fmt.Sprintf("permanently delete secondary calendar %s", id) },
		resultKey: "deleted",
		delete: func(svc *calendar.Service, id string) error {
			return svc.Calendars.Delete(id).Context(ctx).Do()
		},
		errorPrefix: "delete secondary calendar",
	})
}

func runCalendarLifecycleDelete(ctx context.Context, flags *RootFlags, rawCalendarID string, operation calendarLifecycleDelete) error {
	u := ui.FromContext(ctx)
	calendarID := strings.TrimSpace(rawCalendarID)
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

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, operation.op, map[string]any{
		"calendar_id": preparedID,
	}, operation.action(preparedID)); confirmErr != nil {
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

	if err := operation.delete(svc, resolvedID); err != nil {
		return fmt.Errorf("%s %s: %w", operation.errorPrefix, resolvedID, err)
	}

	return writeResult(ctx, u,
		kv(operation.resultKey, true),
		kv("calendarId", resolvedID),
	)
}
