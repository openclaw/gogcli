package cmd

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"google.golang.org/api/cloudidentity/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func newCloudIdentityTestService(t *testing.T, handler http.Handler) *cloudidentity.Service {
	t.Helper()
	svc, closeServer := newGoogleTestService(t, handler, cloudidentity.NewService)
	t.Cleanup(closeServer)
	return svc
}

func fixedCloudIdentityTestService(svc *cloudidentity.Service) app.CloudIdentityServiceFactory {
	return func(context.Context, string) (*cloudidentity.Service, error) {
		return svc, nil
	}
}

func unexpectedCloudIdentityTestService(t *testing.T, message string) app.CloudIdentityServiceFactory {
	t.Helper()
	return func(context.Context, string) (*cloudidentity.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected Cloud Identity service call")
	}
}

func withCloudIdentityTestService(ctx context.Context, svc *cloudidentity.Service) context.Context {
	return withCloudIdentityTestServiceFactory(ctx, fixedCloudIdentityTestService(svc))
}

func withCloudIdentityTestServiceFactory(ctx context.Context, factory app.CloudIdentityServiceFactory) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.CloudIdentity = factory
	})
}

func executeWithCloudIdentityTestService(t *testing.T, args []string, svc *cloudidentity.Service) executeTestResult {
	t.Helper()
	return executeWithCloudIdentityTestServiceFactory(t, args, fixedCloudIdentityTestService(svc))
}

func executeWithCloudIdentityTestServiceFactory(
	t *testing.T,
	args []string,
	factory app.CloudIdentityServiceFactory,
) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		CloudIdentity: factory,
	}})
}
