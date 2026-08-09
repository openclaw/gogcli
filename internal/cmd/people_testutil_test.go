package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/people/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func newPeopleSearchTestService(
	t *testing.T,
	path string,
	resourceName string,
	displayName string,
	email string,
	queries *[]string,
) *people.Service {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, path) {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query().Get("query")
		*queries = append(*queries, query)
		w.Header().Set("Content-Type", "application/json")
		if query == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"person": map[string]any{
			"resourceName": resourceName,
			"names":        []map[string]any{{"displayName": displayName}},
			"emailAddresses": []map[string]any{
				{"value": email},
			},
		}}}})
	})
	svc, closeServer := newGoogleTestService(t, handler, people.NewService)
	t.Cleanup(closeServer)
	return svc
}

func newPeopleServiceFromServer(t *testing.T, srv *httptest.Server) *people.Service {
	t.Helper()
	return newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", people.NewService)
}

func withPeopleContactsTestService(ctx context.Context, svc *people.Service) context.Context {
	return withPeopleTestServices(ctx, peopleTestServices{
		Contacts: fixedPeopleTestService(svc),
	})
}

type peopleTestServices struct {
	Contacts  app.PeopleServiceFactory
	Directory app.PeopleServiceFactory
	Other     app.PeopleServiceFactory
}

func fixedPeopleTestService(svc *people.Service) app.PeopleServiceFactory {
	return func(context.Context, string) (*people.Service, error) {
		return svc, nil
	}
}

func withPeopleTestServices(ctx context.Context, services peopleTestServices) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	if services.Contacts != nil {
		runtime.Services.PeopleContacts = services.Contacts
	}
	if services.Directory != nil {
		runtime.Services.PeopleDirectory = services.Directory
	}
	if services.Other != nil {
		runtime.Services.PeopleOther = services.Other
	}
	return app.WithRuntime(ctx, runtime)
}

func executeWithPeopleTestServices(t *testing.T, args []string, services peopleTestServices) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		PeopleContacts:  services.Contacts,
		PeopleDirectory: services.Directory,
		PeopleOther:     services.Other,
	}})
}

func executeWithPeopleContactsTestService(t *testing.T, args []string, svc *people.Service) executeTestResult {
	t.Helper()
	return executeWithPeopleTestServices(t, args, peopleTestServices{
		Contacts: fixedPeopleTestService(svc),
	})
}

func executeWithPeopleOtherTestService(t *testing.T, args []string, svc *people.Service) executeTestResult {
	t.Helper()
	return executeWithPeopleTestServices(t, args, peopleTestServices{
		Other: fixedPeopleTestService(svc),
	})
}

func executeWithAllPeopleTestServices(t *testing.T, args []string, svc *people.Service) executeTestResult {
	t.Helper()
	factory := fixedPeopleTestService(svc)
	return executeWithPeopleTestServices(t, args, peopleTestServices{
		Contacts:  factory,
		Directory: factory,
		Other:     factory,
	})
}
