package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"google.golang.org/api/calendar/v3"

	"github.com/openclaw/gogcli/internal/app"
)

func newCalendarServiceForTest(t *testing.T, h http.Handler) (*calendar.Service, func()) {
	t.Helper()

	return newGoogleTestService(t, h, calendar.NewService)
}

func newTestCalendarService(t *testing.T, h http.Handler) (*calendar.Service, func()) {
	t.Helper()
	return newCalendarServiceForTest(t, h)
}

func withCalendarTestService(ctx context.Context, svc *calendar.Service) context.Context {
	return withCalendarTestServiceFactory(ctx, func(context.Context, string) (*calendar.Service, error) {
		return svc, nil
	})
}

func withCalendarTestServiceFactory(ctx context.Context, factory app.CalendarServiceFactory) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.Calendar = factory
	})
}

func executeWithCalendarTestService(t *testing.T, args []string, svc *calendar.Service) executeTestResult {
	t.Helper()
	return executeWithCalendarTestServiceFactory(t, args, func(context.Context, string) (*calendar.Service, error) {
		return svc, nil
	})
}

func executeWithCalendarTestServiceFactory(t *testing.T, args []string, factory app.CalendarServiceFactory) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Calendar: factory,
	}})
}

func newCalendarTestJSONContext(t *testing.T, svc *calendar.Service) (context.Context, *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	return withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &output, io.Discard), svc), &output
}
