package cmd

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	scriptapi "google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func newAppScriptTestService(t *testing.T, srv *httptest.Server) *scriptapi.Service {
	t.Helper()
	return newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", scriptapi.NewService)
}

func fixedAppScriptTestService(svc *scriptapi.Service) app.AppScriptServiceFactory {
	return func(context.Context, string) (*scriptapi.Service, error) {
		return svc, nil
	}
}

func unexpectedAppScriptTestService(t *testing.T, message string) app.AppScriptServiceFactory {
	t.Helper()
	return func(context.Context, string) (*scriptapi.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected Apps Script service call")
	}
}

func executeWithAppScriptTestService(t *testing.T, args []string, svc *scriptapi.Service) executeTestResult {
	t.Helper()
	return executeWithAppScriptTestServiceFactory(t, args, fixedAppScriptTestService(svc))
}

func executeWithAppScriptTestServiceFactory(
	t *testing.T,
	args []string,
	factory app.AppScriptServiceFactory,
) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		AppScript: factory,
	}})
}
