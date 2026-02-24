package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func newGmailServiceForTest(t *testing.T, h http.HandlerFunc) (*gmail.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(h)
	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		srv.Close()
		t.Fatalf("NewService: %v", err)
	}
	return svc, srv.Close
}
