package cmd

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"

	formsapi "google.golang.org/api/forms/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/ui"
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

func newQuietUIContext(t *testing.T) context.Context {
	t.Helper()

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}
