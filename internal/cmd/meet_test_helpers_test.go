package cmd

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"google.golang.org/api/meet/v2"

	"github.com/openclaw/gogcli/internal/app"
)

func newTestMeetService(t *testing.T, handler http.Handler) *meet.Service {
	t.Helper()
	svc, closeServer := newGoogleTestService(t, handler, meet.NewService)
	t.Cleanup(closeServer)
	return svc
}

func fixedMeetTestService(svc *meet.Service) app.MeetServiceFactory {
	return func(context.Context, string) (*meet.Service, error) {
		return svc, nil
	}
}

func unexpectedMeetTestService(t *testing.T, message string) app.MeetServiceFactory {
	t.Helper()
	return func(context.Context, string) (*meet.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected Meet service call")
	}
}

func executeWithMeetTestService(t *testing.T, args []string, svc *meet.Service) executeTestResult {
	t.Helper()
	return executeWithMeetTestOperations(t, args, fixedMeetTestService(svc), nil)
}

func executeWithMeetTestOperations(
	t *testing.T,
	args []string,
	factory app.MeetServiceFactory,
	openURL app.OpenURLFunc,
) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Meet:    factory,
		OpenURL: openURL,
	}})
}
