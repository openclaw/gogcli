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
	origNew := newPeopleContactsService
	t.Cleanup(func() { newPeopleContactsService = origNew })

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
	newPeopleContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "people", "me"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Person struct {
			ResourceName string `json:"resourceName"`
		} `json:"person"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Person.ResourceName != "people/me" {
		t.Fatalf("unexpected person: %#v", parsed.Person)
	}
}

func TestExecute_PeopleMe_Text(t *testing.T) {
	origNew := newPeopleContactsService
	t.Cleanup(func() { newPeopleContactsService = origNew })

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
	newPeopleContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "people", "me"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "name\tPeter") || !strings.Contains(out, "email\ta@b.com") || !strings.Contains(out, "photo\thttps://example.com/p.jpg") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_PeopleMe_FallbackWhenPeopleAPIDisabled(t *testing.T) {
	origNew := newPeopleContactsService
	origFallback := fallbackPeopleMeProfile
	t.Cleanup(func() {
		newPeopleContactsService = origNew
		fallbackPeopleMeProfile = origFallback
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/me") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(&gapi.Error{
			Code:    403,
			Message: "People API has not been used in project before or it is disabled.",
			Errors: []gapi.ErrorItem{
				{Reason: "accessNotConfigured"},
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
	newPeopleContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }
	fallbackPeopleMeProfile = func(context.Context, string) (*people.Person, error) {
		return &people.Person{
			ResourceName: "people/me",
			Names:        []*people.Name{{DisplayName: "Fallback User"}},
			EmailAddresses: []*people.EmailAddress{
				{Value: "fallback@example.com"},
			},
		}, nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "people", "me"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "name\tFallback User") || !strings.Contains(out, "email\tfallback@example.com") {
		t.Fatalf("unexpected out=%q", out)
	}
}
