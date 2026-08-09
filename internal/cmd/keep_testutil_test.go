package cmd

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	keepapi "google.golang.org/api/keep/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func newKeepTestServiceFromServer(t *testing.T, srv *httptest.Server) *keepapi.Service {
	t.Helper()
	return newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", keepapi.NewService)
}

func fixedKeepTestService(svc *keepapi.Service) app.KeepServiceAccountFactory {
	return func(context.Context, string, string) (*keepapi.Service, error) {
		return svc, nil
	}
}

func unexpectedKeepTestService(t *testing.T, message string) app.KeepServiceAccountFactory {
	t.Helper()
	return func(context.Context, string, string) (*keepapi.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected Keep service call")
	}
}

func withKeepTestServiceFactory(ctx context.Context, factory app.KeepServiceAccountFactory) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.Keep = factory
	})
}

func executeWithKeepTestService(t *testing.T, args []string, svc *keepapi.Service) executeTestResult {
	t.Helper()
	return executeWithKeepTestServiceFactory(t, args, fixedKeepTestService(svc))
}

func executeWithKeepTestServiceFactory(
	t *testing.T,
	args []string,
	factory app.KeepServiceAccountFactory,
) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Keep: factory,
	}})
}
