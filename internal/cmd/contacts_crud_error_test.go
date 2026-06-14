package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/ui"
)

func newPeopleService(t *testing.T, handler http.HandlerFunc) (*people.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(handler)
	svc, err := people.NewService(context.Background(),
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

func withStubPeopleServices(ctx context.Context, svc *people.Service) context.Context {
	return withPeopleTestServices(ctx, peopleTestServices{
		Contacts: fixedPeopleTestService(svc),
		Other:    fixedPeopleTestService(svc),
	})
}

func TestContactsListAndGet_NoResults_Text(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "people/me/connections") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"connections": []map[string]any{}})
			return
		case strings.Contains(r.URL.Path, "people:searchContacts") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(closeSrv)

	flags := &RootFlags{Account: "a@b.com"}
	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: os.Stderr, Color: "never"})
			if uiErr != nil {
				t.Fatalf("ui.New: %v", uiErr)
			}
			ctx := withStubPeopleServices(ui.WithUI(context.Background(), u), svc)

			if err := runKong(t, &ContactsListCmd{}, []string{}, ctx, flags); err != nil {
				t.Fatalf("list: %v", err)
			}

			if err := runKong(t, &ContactsGetCmd{}, []string{"missing@example.com"}, ctx, flags); err != nil {
				t.Fatalf("get: %v", err)
			}
		})
	})
	if !strings.Contains(errOut, "No contacts") && !strings.Contains(errOut, "Not found") {
		t.Fatalf("unexpected stderr: %q", errOut)
	}
}

func TestContactsUpdateDelete_InvalidResource(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}

	if err := runKong(t, &ContactsUpdateCmd{}, []string{"nope"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "resourceName must start") {
		t.Fatalf("expected resourceName error, got %v", err)
	}

	if err := runKong(t, &ContactsDeleteCmd{}, []string{"nope"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "resourceName must start") {
		t.Fatalf("expected resourceName error, got %v", err)
	}
}
