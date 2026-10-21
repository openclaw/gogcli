package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gapi "google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"
)

func TestExecute_PeopleMe_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/me") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceName": "people/me",
			"names":        []map[string]any{{"displayName": "Peter"}},
			"emailAddresses": []map[string]any{
				{"value": "a@b.com"},
			},
		})
	}))
	defer srv.Close()

	svc, err := people.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	result := executeWithPeopleContactsTestService(t, []string{"--json", "--account", "a@b.com", "people", "me"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Person struct {
			ResourceName string `json:"resourceName"`
		} `json:"person"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if parsed.Person.ResourceName != "people/me" {
		t.Fatalf("unexpected person: %#v", parsed.Person)
	}
}

func TestExecute_PeopleMe_Text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/me") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceName": "people/me",
			"names":        []map[string]any{{"displayName": "Peter"}},
			"emailAddresses": []map[string]any{
				{"value": "a@b.com"},
			},
			"photos": []map[string]any{
				{"url": "https://example.com/p.jpg"},
			},
		})
	}))
	defer srv.Close()

	svc, err := people.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	result := executeWithPeopleContactsTestService(t, []string{"--account", "a@b.com", "people", "me"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stdout, "name\tPeter") || !strings.Contains(result.stdout, "email\ta@b.com") || !strings.Contains(result.stdout, "photo\thttps://example.com/p.jpg") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_PeopleMe_FallsBackWhenPeopleAPIDisabled(t *testing.T) {
	origFallback := fallbackPeopleMeProfile
	t.Cleanup(func() { fallbackPeopleMeProfile = origFallback })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/me") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(&gapi.Error{
			Code:    http.StatusForbidden,
			Message: "People API has not been used in project before or it is disabled.",
			Errors: []gapi.ErrorItem{{
				Reason: "accessNotConfigured",
			}},
		})
	}))
	defer srv.Close()

	svc, err := people.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	fallbackPeopleMeProfile = func(context.Context, string) (*people.Person, error) {
		return &people.Person{
			ResourceName: peopleMeResource,
			EmailAddresses: []*people.EmailAddress{
				{Value: "fallback@example.com"},
			},
		}, nil
	}

	result := executeWithPeopleContactsTestService(t, []string{"--account", "a@b.com", "whoami"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stdout, "email\tfallback@example.com") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_PeopleMe_NoTokenFallbackWithAPIBaseURL(t *testing.T) {
	origFallback := fallbackPeopleMeProfile
	t.Cleanup(func() { fallbackPeopleMeProfile = origFallback })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/me") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(&gapi.Error{
			Code:    http.StatusForbidden,
			Message: "People API has not been used in project before or it is disabled.",
			Errors: []gapi.ErrorItem{{
				Reason: "accessNotConfigured",
			}},
		})
	}))
	defer srv.Close()

	svc, err := people.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Setenv("GOG_GOOGLE_API_BASE_URL", "http://127.0.0.1:18080")
	called := false
	fallbackPeopleMeProfile = func(context.Context, string) (*people.Person, error) {
		called = true
		return &people.Person{}, nil
	}

	result := executeWithPeopleContactsTestService(t, []string{"--account", "a@b.com", "whoami"}, svc)
	if result.err == nil || !strings.Contains(result.err.Error(), "accessNotConfigured") {
		t.Fatalf("error = %v, want the People API error", result.err)
	}
	if called {
		t.Fatal("token fallback ran with GOG_GOOGLE_API_BASE_URL set")
	}
}
