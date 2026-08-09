package cmd

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/app"
)

func fixedSitesDriveTestService(svc *drive.Service) app.DriveServiceFactory {
	return func(context.Context, string) (*drive.Service, error) {
		return svc, nil
	}
}

func unexpectedSitesDriveTestService(t *testing.T, message string) app.DriveServiceFactory {
	t.Helper()
	return func(context.Context, string) (*drive.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected Sites Drive service call")
	}
}

func withSitesDriveTestService(ctx context.Context, svc *drive.Service) context.Context {
	return withSitesDriveTestServiceFactory(ctx, fixedSitesDriveTestService(svc))
}

func withSitesDriveTestServiceFactory(ctx context.Context, factory app.DriveServiceFactory) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.SitesDrive = factory
	})
}
