package cmd

import (
	"net/http"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func newGmailServiceForTest(t *testing.T, h http.HandlerFunc) (*gmail.Service, func()) {
	t.Helper()

	return newGoogleTestService(t, h, gmail.NewService)
}

func stubGmailServiceForTest(t *testing.T, svc *gmail.Service) {
	t.Helper()
	stubGoogleTestService(t, &newGmailService, svc)
}
