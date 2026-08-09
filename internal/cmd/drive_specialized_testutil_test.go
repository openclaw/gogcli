package cmd

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/drivelabels/v2"

	"github.com/openclaw/gogcli/internal/app"
)

func fixedDriveActivityTestService(svc *driveactivity.Service) app.DriveActivityServiceFactory {
	return func(context.Context, string) (*driveactivity.Service, error) {
		return svc, nil
	}
}

func unexpectedDriveActivityTestService(t *testing.T, message string) app.DriveActivityServiceFactory {
	t.Helper()
	return func(context.Context, string) (*driveactivity.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected Drive Activity service call")
	}
}

func withDriveActivityTestService(ctx context.Context, svc *driveactivity.Service) context.Context {
	return withDriveActivityTestServiceFactory(ctx, fixedDriveActivityTestService(svc))
}

func withDriveActivityTestServiceFactory(
	ctx context.Context,
	factory app.DriveActivityServiceFactory,
) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.DriveActivity = factory
	})
}

func fixedDriveLabelsTestService(svc *drivelabels.Service) app.DriveLabelsServiceFactory {
	return func(context.Context, string) (*drivelabels.Service, error) {
		return svc, nil
	}
}

func unexpectedDriveLabelsTestService(t *testing.T, message string) app.DriveLabelsServiceFactory {
	t.Helper()
	return func(context.Context, string) (*drivelabels.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected Drive Labels service call")
	}
}

func withDriveLabelsTestService(ctx context.Context, svc *drivelabels.Service) context.Context {
	return withDriveLabelsTestServiceFactory(ctx, fixedDriveLabelsTestService(svc))
}

func withDriveLabelsTestServiceFactory(
	ctx context.Context,
	factory app.DriveLabelsServiceFactory,
) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.DriveLabels = factory
	})
}
