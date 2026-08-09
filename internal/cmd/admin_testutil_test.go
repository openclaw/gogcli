package cmd

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func newAdminTestService(t *testing.T, handler http.Handler) *admin.Service {
	t.Helper()
	svc, closeServer := newGoogleTestService(t, handler, admin.NewService)
	t.Cleanup(closeServer)
	return svc
}

func fixedAdminTestService(svc *admin.Service) app.AdminDirectoryServiceFactory {
	return func(context.Context, string) (*admin.Service, error) {
		return svc, nil
	}
}

func unexpectedAdminTestService(t *testing.T, message string) app.AdminDirectoryServiceFactory {
	t.Helper()
	return func(context.Context, string) (*admin.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected admin service call")
	}
}

func withAdminDirectoryTestServiceFactory(ctx context.Context, factory app.AdminDirectoryServiceFactory) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.AdminDirectory = factory
	})
}

func withAdminOrgUnitTestServiceFactory(ctx context.Context, factory app.AdminDirectoryServiceFactory) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.AdminOrgUnit = factory
	})
}

func runWithAdminDirectoryTestService(
	t *testing.T,
	svc *admin.Service,
	run func(context.Context) error,
) executeTestResult {
	t.Helper()
	return runWithAdminDirectoryTestServiceFactory(t, fixedAdminTestService(svc), run)
}

func runWithAdminDirectoryTestServiceFactory(
	t *testing.T,
	factory app.AdminDirectoryServiceFactory,
	run func(context.Context) error,
) executeTestResult {
	t.Helper()
	return runWithAdminTestService(t, factory, false, run)
}

func runWithAdminOrgUnitTestService(
	t *testing.T,
	svc *admin.Service,
	run func(context.Context) error,
) executeTestResult {
	t.Helper()
	return runWithAdminTestService(t, fixedAdminTestService(svc), true, run)
}

func runWithAdminTestService(
	t *testing.T,
	factory app.AdminDirectoryServiceFactory,
	orgUnit bool,
	run func(context.Context) error,
) executeTestResult {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &stdout, &stderr)
	if orgUnit {
		ctx = withAdminOrgUnitTestServiceFactory(ctx, factory)
	} else {
		ctx = withAdminDirectoryTestServiceFactory(ctx, factory)
	}
	err := run(ctx)
	return executeTestResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
}

func executeWithAdminDirectoryTestServiceFactory(
	t *testing.T,
	args []string,
	factory app.AdminDirectoryServiceFactory,
) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		AdminDirectory: factory,
	}})
}
