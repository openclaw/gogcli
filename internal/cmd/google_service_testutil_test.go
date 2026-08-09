package cmd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/app"
)

type googleTestServiceFactory[T any] func(context.Context, ...option.ClientOption) (*T, error)

type googleServiceTestSupport[T any, F any] struct {
	newService googleTestServiceFactory[T]
	wrap       func(func(context.Context, string) (*T, error)) F
	services   func(F) app.Services
}

func (s googleServiceTestSupport[T, F]) new(t *testing.T, handler http.Handler) *T {
	t.Helper()
	svc, closeServer := newGoogleTestService(t, handler, s.newService)
	t.Cleanup(closeServer)
	return svc
}

func (s googleServiceTestSupport[T, F]) fixed(svc *T) F {
	return s.wrap(fixedGoogleTestService(svc))
}

func (s googleServiceTestSupport[T, F]) unexpected(t *testing.T, message string) F {
	t.Helper()
	return s.wrap(unexpectedGoogleTestService[T](t, message))
}

func (s googleServiceTestSupport[T, F]) execute(t *testing.T, args []string, factory F) executeTestResult {
	t.Helper()
	return executeWithGoogleTestServiceFactory(t, args, factory, s.services)
}

func (s googleServiceTestSupport[T, F]) executeWithService(t *testing.T, args []string, svc *T) executeTestResult {
	t.Helper()
	return s.execute(t, args, s.fixed(svc))
}

func fixedGoogleTestService[T any](svc *T) func(context.Context, string) (*T, error) {
	return func(context.Context, string) (*T, error) { return svc, nil }
}

func unexpectedGoogleTestService[T any](t *testing.T, message string) func(context.Context, string) (*T, error) {
	t.Helper()
	return func(context.Context, string) (*T, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected Google service call")
	}
}

func executeWithGoogleTestServiceFactory[F any](
	t *testing.T,
	args []string,
	factory F,
	services func(F) app.Services,
) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: services(factory)})
}

func newGoogleTestService[T any](t *testing.T, h http.Handler, factory googleTestServiceFactory[T]) (*T, func()) {
	t.Helper()

	srv := httptest.NewServer(h)
	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", factory)
	return svc, srv.Close
}

func newGoogleTestServiceWithEndpoint[T any](
	t *testing.T,
	client *http.Client,
	endpoint string,
	factory googleTestServiceFactory[T],
) *T {
	t.Helper()

	svc, err := factory(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(client),
		option.WithEndpoint(endpoint),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}
