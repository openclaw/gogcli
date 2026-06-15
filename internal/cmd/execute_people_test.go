package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"
)

var errUnexpectedPeopleServiceCall = errors.New("unexpected people directory service call")

func TestExecute_PeopleGet_Text(t *testing.T) {
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
	result := executeWithPeopleDirectoryTestService(t, []string{"--account", "a@b.com", "people", "get", "123"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stdout, "resource\tpeople/123") || !strings.Contains(result.stdout, "email\tada@example.com") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_PeopleGet_JSON(t *testing.T) {
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
	result := executeWithPeopleDirectoryTestService(t, []string{"--json", "--account", "a@b.com", "people", "get", "people/123"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Person struct {
			ResourceName string `json:"resourceName"`
		} `json:"person"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Person.ResourceName != "people/123" {
		t.Fatalf("unexpected person: %#v", parsed.Person)
	}
}

func TestExecute_PeopleGet_Me_UsesContacts(t *testing.T) {
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
	result := executeWithPeopleTestServices(t, []string{"--account", "a@b.com", "people", "get", "me"}, peopleTestServices{
		Contacts: fixedPeopleTestService(svc),
		Directory: func(context.Context, string) (*people.Service, error) {
			t.Fatalf("unexpected directory service call")
			return nil, errUnexpectedPeopleServiceCall
		},
	})
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stdout, "resource\tpeople/me") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_PeopleSearch_JSON(t *testing.T) {
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
	result := executeWithPeopleDirectoryTestService(t, []string{"--json", "--account", "a@b.com", "people", "search", "Ada"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	var parsed struct {
		People []struct {
			Resource string `json:"resource"`
			Name     string `json:"name"`
			Email    string `json:"email"`
		} `json:"people"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.NextPageToken != "npt" || len(parsed.People) != 1 {
		t.Fatalf("unexpected response: %#v", parsed)
	}
	if parsed.People[0].Resource != "people/abc" || parsed.People[0].Email != "ada@example.com" {
		t.Fatalf("unexpected person: %#v", parsed.People[0])
	}
}

func TestExecute_PeopleSearchInvalidMaxFailsBeforeService(t *testing.T) {
	directory := func(context.Context, string) (*people.Service, error) {
		t.Fatalf("expected max validation to fail before creating people service")
		return nil, errUnexpectedPeopleServiceCall
	}

	testCases := [][]string{
		{"--account", "a@b.com", "people", "search", "alice", "--max", "0"},
		{"--account", "a@b.com", "people", "search", "alice", "--max=-1"},
	}
	for _, args := range testCases {
		t.Run(strings.Join(args[2:], "_"), func(t *testing.T) {
			result := executeWithPeopleTestServices(t, args, peopleTestServices{Directory: directory})
			if result.err == nil || ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), "max must be > 0") {
				t.Fatalf("unexpected err: %v", result.err)
			}
		})
	}
}

func TestExecute_PeopleSearch_Text(t *testing.T) {
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
	result := executeWithPeopleDirectoryTestService(t, []string{"--account", "a@b.com", "people", "search", "Ada"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stderr, "# More results: use --all/--all-pages to fetch every page, or --page npt for the next page") {
		t.Fatalf("unexpected stderr=%q", result.stderr)
	}
	if !strings.Contains(result.stdout, "RESOURCE") || !strings.Contains(result.stdout, "people/abc") || !strings.Contains(result.stdout, "Ada Lovelace") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_PeopleRelations_JSON(t *testing.T) {
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
	result := executeWithPeopleDirectoryTestService(t, []string{"--json", "--account", "a@b.com", "people", "relations", "123", "--type", "manager"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	var parsed struct {
		Resource     string `json:"resource"`
		RelationType string `json:"relationType"`
		Relations    []struct {
			Type   string `json:"type"`
			Person string `json:"person"`
		} `json:"relations"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Resource != "people/123" || parsed.RelationType != "manager" {
		t.Fatalf("unexpected response: %#v", parsed)
	}
	if len(parsed.Relations) != 1 || parsed.Relations[0].Person != "people/456" {
		t.Fatalf("unexpected relations: %#v", parsed.Relations)
	}
}

func TestExecute_PeopleRelations_JSON_EmptyArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/people/123") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceName": "people/123",
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
	result := executeWithPeopleDirectoryTestService(t, []string{"--json", "--account", "a@b.com", "people", "relations", "123"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	var parsed struct {
		Resource  string             `json:"resource"`
		Relations []*people.Relation `json:"relations"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Resource != "people/123" {
		t.Fatalf("resource = %q", parsed.Resource)
	}
	if parsed.Relations == nil || len(parsed.Relations) != 0 {
		t.Fatalf("relations = %#v, want empty non-nil slice", parsed.Relations)
	}
}

func TestExecute_PeopleRelations_Text(t *testing.T) {
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
	result := executeWithPeopleDirectoryTestService(t, []string{"--account", "a@b.com", "people", "relations", "people/123"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stdout, "TYPE") || !strings.Contains(result.stdout, "manager") || !strings.Contains(result.stdout, "people/456") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}
