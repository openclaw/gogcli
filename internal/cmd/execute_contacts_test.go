package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"
)

func TestExecute_ContactsList_JSON(t *testing.T) {
	origNew := newPeopleContactsService
	t.Cleanup(func() { newPeopleContactsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/people/me/connections") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"connections": []map[string]any{
				{
					"resourceName": "people/c1",
					"names":        []map[string]any{{"displayName": "Ada Lovelace"}},
					"emailAddresses": []map[string]any{
						{"value": "ada@example.com"},
					},
					"birthdays": []map[string]any{
						{"date": map[string]any{"year": 1815, "month": 12, "day": 10}},
					},
				},
			},
			"nextPageToken": "npt",
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "contacts", "list", "--max", "1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Contacts []struct {
			Resource string `json:"resource"`
			Name     string `json:"name"`
			Email    string `json:"email"`
			Birthday string `json:"birthday"`
		} `json:"contacts"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.NextPageToken != "npt" || len(parsed.Contacts) != 1 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if parsed.Contacts[0].Resource != "people/c1" || parsed.Contacts[0].Name != "Ada Lovelace" || parsed.Contacts[0].Email != "ada@example.com" {
		t.Fatalf("unexpected contact: %#v", parsed.Contacts[0])
	}
	if parsed.Contacts[0].Birthday != "1815-12-10" {
		t.Fatalf("unexpected birthday: %#v", parsed.Contacts[0])
	}
}

func TestExecute_ContactsGet_ByEmail_JSON(t *testing.T) {
	origNew := newPeopleContactsService
	t.Cleanup(func() { newPeopleContactsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "people:searchContacts") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"person": map[string]any{
						"resourceName": "people/c1",
						"names":        []map[string]any{{"displayName": "Ada Lovelace"}},
						"emailAddresses": []map[string]any{
							{"value": "ada@example.com"},
						},
					},
				},
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
			if err := Execute([]string{"--json", "--account", "a@b.com", "contacts", "get", "ada@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Contact struct {
			ResourceName string `json:"resourceName"`
		} `json:"contact"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Contact.ResourceName != "people/c1" {
		t.Fatalf("unexpected contact: %#v", parsed.Contact)
	}
}
