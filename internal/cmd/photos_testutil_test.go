package cmd

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleapi"
)

type photosTestServices struct {
	Photos       app.PhotosServiceFactory
	PhotosPicker app.PhotosPickerServiceFactory
	OpenURL      app.OpenURLFunc
}

func fixedPhotosTestService(client *googleapi.PhotosClient) app.PhotosServiceFactory {
	return func(context.Context, string) (*googleapi.PhotosClient, error) {
		return client, nil
	}
}

func unexpectedPhotosTestService(t *testing.T, message string) app.PhotosServiceFactory {
	t.Helper()
	return func(context.Context, string) (*googleapi.PhotosClient, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected Photos client call")
	}
}

func fixedPhotosPickerTestService(client *googleapi.PhotosPickerClient) app.PhotosPickerServiceFactory {
	return func(context.Context, string) (*googleapi.PhotosPickerClient, error) {
		return client, nil
	}
}

func unexpectedPhotosPickerTestService(t *testing.T, message string) app.PhotosPickerServiceFactory {
	t.Helper()
	return func(context.Context, string) (*googleapi.PhotosPickerClient, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected Photos Picker client call")
	}
}

func runWithPhotosTestServices(
	t *testing.T,
	services photosTestServices,
	run func(context.Context) error,
) executeTestResult {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &stdout, &stderr)
	ctx = withPhotosTestServices(ctx, services)
	err := run(ctx)
	return executeTestResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
}

func executeWithPhotosTestServices(t *testing.T, args []string, services photosTestServices) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Photos:       services.Photos,
		PhotosPicker: services.PhotosPicker,
		OpenURL:      services.OpenURL,
	}})
}

func withPhotosTestServices(ctx context.Context, services photosTestServices) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.Photos = services.Photos
		runtime.Services.PhotosPicker = services.PhotosPicker
		runtime.Services.OpenURL = services.OpenURL
	})
}
