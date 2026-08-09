package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	formsapi "google.golang.org/api/forms/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/ui"
)

func newFormsTestService(t *testing.T, ctx context.Context, srv *httptest.Server) *formsapi.Service {
	t.Helper()

	svc, err := formsapi.NewService(ctx,
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func fixedFormsTestService(svc *formsapi.Service) app.FormsServiceFactory {
	return func(context.Context, string) (*formsapi.Service, error) {
		return svc, nil
	}
}

func unexpectedFormsTestService(t *testing.T, message string) app.FormsServiceFactory {
	t.Helper()
	return func(context.Context, string) (*formsapi.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected forms service call")
	}
}

func withFormsTestService(ctx context.Context, svc *formsapi.Service) context.Context {
	return withFormsTestServiceFactory(ctx, fixedFormsTestService(svc))
}

func withFormsTestServiceFactory(ctx context.Context, factory app.FormsServiceFactory) context.Context {
	return withTestRuntime(ctx, func(runtime *app.Runtime) {
		runtime.Services.Forms = factory
	})
}

func executeWithFormsTestService(t *testing.T, args []string, svc *formsapi.Service) executeTestResult {
	t.Helper()
	return executeWithFormsTestServiceFactory(t, args, fixedFormsTestService(svc))
}

func executeWithFormsTestServiceFactory(t *testing.T, args []string, factory app.FormsServiceFactory) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Forms: factory,
	}})
}

func formsRawTestContext(t *testing.T, svc *formsapi.Service) (context.Context, *bytes.Buffer) {
	t.Helper()
	output := &bytes.Buffer{}
	return withFormsTestService(newCmdRuntimeOutputContext(t, output, io.Discard), svc), output
}

func newQuietUIContext(t *testing.T) context.Context {
	t.Helper()

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}
