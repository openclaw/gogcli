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

func TestExecute_ContactsDirectoryList_Text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "people:listDirectoryPeople") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"people": []map[string]any{
				{
					"resourceName": "people/d1",
					"names":        []map[string]any{{"displayName": "Dir"}},
					"emailAddresses": []map[string]any{
						{"value": "dir@example.com"},
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

	result := executeWithPeopleDirectoryTestService(t, []string{"--account", "a@b.com", "contacts", "directory", "list", "--max", "1"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stderr, "# More results: use --all/--all-pages to fetch every page, or --page npt for the next page") {
		t.Fatalf("unexpected stderr=%q", result.stderr)
	}
	out := result.stdout
	if !strings.Contains(out, "RESOURCE") || !strings.Contains(out, "people/d1") || !strings.Contains(out, "dir@example.com") {
		t.Fatalf("unexpected out=%q", out)
	}
}
