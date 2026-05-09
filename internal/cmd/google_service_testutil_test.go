package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/option"
)

type googleTestServiceFactory[T any] func(context.Context, ...option.ClientOption) (*T, error)

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

func stubGoogleTestService[T any](t *testing.T, target *func(context.Context, string) (*T, error), svc *T) {
	t.Helper()

	orig := *target
	t.Cleanup(func() { *target = orig })
	//nolint:unparam // test stub must match production service constructor signature.
	*target = func(context.Context, string) (*T, error) { return svc, nil }
}
