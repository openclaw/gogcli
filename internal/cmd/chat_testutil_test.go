package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/chat/v1"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func newChatSearchTestClient(t *testing.T, handler http.HandlerFunc) *googleapi.ChatSearchClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return googleapi.NewChatSearchClient(srv.Client(), srv.URL+"/v1")
}

func executeWithChatSearchTestService(t *testing.T, args []string, svc *googleapi.ChatSearchClient) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		ChatSearch: fixedGoogleTestService(svc),
	}})
}

var chatTestServices = googleServiceTestSupport[chat.Service, app.ChatServiceFactory]{
	newService: chat.NewService,
	wrap: func(factory func(context.Context, string) (*chat.Service, error)) app.ChatServiceFactory {
		return factory
	},
	services: func(factory app.ChatServiceFactory) app.Services {
		return app.Services{Chat: factory}
	},
}

var (
	newChatTestService                = chatTestServices.new
	unexpectedChatTestService         = chatTestServices.unexpected
	executeWithChatTestService        = chatTestServices.executeWithService
	executeWithChatTestServiceFactory = chatTestServices.execute
)
