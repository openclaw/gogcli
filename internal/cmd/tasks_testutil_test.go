package cmd

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/tasks/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func newTasksServiceFromServer(t *testing.T, srv *httptest.Server) *tasks.Service {
	t.Helper()
	return newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", tasks.NewService)
}

func fixedTasksTestService(svc *tasks.Service) app.TasksServiceFactory {
	return func(context.Context, string) (*tasks.Service, error) {
		return svc, nil
	}
}

func unexpectedTasksTestService(t *testing.T, message string) app.TasksServiceFactory {
	t.Helper()
	return func(context.Context, string) (*tasks.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected tasks service call")
	}
}

func withTasksTestService(ctx context.Context, svc *tasks.Service) context.Context {
	return withTasksTestServiceFactory(ctx, fixedTasksTestService(svc))
}

func withTasksTestServiceFactory(ctx context.Context, factory app.TasksServiceFactory) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.Tasks = factory
	})
}

func executeWithTasksTestService(t *testing.T, args []string, svc *tasks.Service) executeTestResult {
	t.Helper()
	return executeWithTasksTestServiceFactory(t, args, fixedTasksTestService(svc))
}

func executeWithTasksTestServiceFactory(t *testing.T, args []string, factory app.TasksServiceFactory) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Tasks: factory,
	}})
}
