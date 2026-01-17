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

func TestExecute_PeopleGet_Text(t *testing.T) {
	origNew := newPeopleDirectoryService
	t.Cleanup(func() { newPeopleDirectoryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/123") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceName": "people/123",
			"names":        []map[string]any{{"displayName": "Ada"}},
			"emailAddresses": []map[string]any{{
				"value": "ada@example.com",
			}},
			"photos": []map[string]any{{"url": "https://example.com/ada.jpg"}},
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
	newPeopleDirectoryService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "people", "get", "123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "resource\tpeople/123") || !strings.Contains(out, "email\tada@example.com") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_PeopleGet_JSON(t *testing.T) {
	origNew := newPeopleDirectoryService
	t.Cleanup(func() { newPeopleDirectoryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/123") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceName": "people/123",
			"names":        []map[string]any{{"displayName": "Ada"}},
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
	newPeopleDirectoryService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "people", "get", "people/123"}); err != nil {
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
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Person.ResourceName != "people/123" {
		t.Fatalf("unexpected person: %#v", parsed.Person)
	}
}

func TestExecute_PeopleGet_Me_UsesContacts(t *testing.T) {
	origContacts := newPeopleContactsService
	origDir := newPeopleDirectoryService
	t.Cleanup(func() {
		newPeopleContactsService = origContacts
		newPeopleDirectoryService = origDir
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/me") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceName": "people/me",
			"names":        []map[string]any{{"displayName": "Ada"}},
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
	newPeopleDirectoryService = func(context.Context, string) (*people.Service, error) {
		t.Fatalf("unexpected directory service call")
		return nil, nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "people", "get", "me"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "resource\tpeople/me") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_PeopleSearch_JSON(t *testing.T) {
	origNew := newPeopleDirectoryService
	t.Cleanup(func() { newPeopleDirectoryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "people:searchDirectoryPeople") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"people": []map[string]any{{
				"resourceName": "people/abc",
				"names":        []map[string]any{{"displayName": "Ada Lovelace"}},
				"emailAddresses": []map[string]any{{
					"value": "ada@example.com",
				}},
			}},
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
	newPeopleDirectoryService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "people", "search", "Ada"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	var parsed struct {
		People []struct {
			Resource string `json:"resource"`
			Name     string `json:"name"`
			Email    string `json:"email"`
		} `json:"people"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.NextPageToken != "npt" || len(parsed.People) != 1 {
		t.Fatalf("unexpected response: %#v", parsed)
	}
	if parsed.People[0].Resource != "people/abc" || parsed.People[0].Email != "ada@example.com" {
		t.Fatalf("unexpected person: %#v", parsed.People[0])
	}
}

func TestExecute_PeopleSearch_Text(t *testing.T) {
	origNew := newPeopleDirectoryService
	t.Cleanup(func() { newPeopleDirectoryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "people:searchDirectoryPeople") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"people": []map[string]any{{
				"resourceName": "people/abc",
				"names":        []map[string]any{{"displayName": "Ada Lovelace"}},
				"emailAddresses": []map[string]any{{
					"value": "ada@example.com",
				}},
			}},
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
	newPeopleDirectoryService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		errOut := captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "people", "search", "Ada"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(errOut, "# Next page: --page npt") {
			t.Fatalf("unexpected stderr=%q", errOut)
		}
	})
	if !strings.Contains(out, "RESOURCE") || !strings.Contains(out, "people/abc") || !strings.Contains(out, "Ada Lovelace") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_PeopleRelations_JSON(t *testing.T) {
	origNew := newPeopleDirectoryService
	t.Cleanup(func() { newPeopleDirectoryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/123") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceName": "people/123",
			"relations": []map[string]any{{
				"type":   "manager",
				"person": "people/456",
			}, {
				"type":   "friend",
				"person": "people/789",
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
	newPeopleDirectoryService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "people", "relations", "123", "--type", "manager"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	var parsed struct {
		Resource     string `json:"resource"`
		RelationType string `json:"relationType"`
		Relations    []struct {
			Type   string `json:"type"`
			Person string `json:"person"`
		} `json:"relations"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Resource != "people/123" || parsed.RelationType != "manager" {
		t.Fatalf("unexpected response: %#v", parsed)
	}
	if len(parsed.Relations) != 1 || parsed.Relations[0].Person != "people/456" {
		t.Fatalf("unexpected relations: %#v", parsed.Relations)
	}
}

func TestExecute_PeopleRelations_Text(t *testing.T) {
	origNew := newPeopleDirectoryService
	t.Cleanup(func() { newPeopleDirectoryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/123") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceName": "people/123",
			"relations": []map[string]any{{
				"type":   "manager",
				"person": "people/456",
			}, {
				"type":   "friend",
				"person": "people/789",
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
	newPeopleDirectoryService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "people", "relations", "people/123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "TYPE") || !strings.Contains(out, "manager") || !strings.Contains(out, "people/456") {
		t.Fatalf("unexpected out=%q", out)
	}
}
