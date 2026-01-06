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

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestContactsListAndGet_NoResults_Text(t *testing.T) {
	origContacts := newPeopleContactsService
	t.Cleanup(func() { newPeopleContactsService = origContacts })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	flags := &RootFlags{Account: "a@b.com"}
	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: os.Stderr, Color: "never"})
			if uiErr != nil {
				t.Fatalf("ui.New: %v", uiErr)
			}
			ctx := ui.WithUI(context.Background(), u)

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

func TestContactsOtherDelete_InvalidResource(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}

	// Test with invalid prefix
	if err := runKong(t, &ContactsOtherDeleteCmd{}, []string{"people/123"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "resourceName must start with otherContacts/") {
		t.Fatalf("expected resourceName error, got %v", err)
	}

	// Test with no prefix
	if err := runKong(t, &ContactsOtherDeleteCmd{}, []string{"nope"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "resourceName must start with otherContacts/") {
		t.Fatalf("expected resourceName error, got %v", err)
	}
}

func TestContactsOtherDelete_Success_JSON(t *testing.T) {
	origOther := newPeopleOtherContactsService
	origContacts := newPeopleContactsService
	t.Cleanup(func() {
		newPeopleOtherContactsService = origOther
		newPeopleContactsService = origContacts
	})

	copiedResourceName := "people/c123456"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "otherContacts/") && strings.Contains(r.URL.Path, ":copyOtherContactToMyContactsGroup") && r.Method == http.MethodPost:
			// Mock CopyOtherContactToMyContactsGroup response
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceName": copiedResourceName,
				"names":        []map[string]any{{"displayName": "Test User"}},
			})
			return
		case strings.Contains(r.URL.Path, "people/c123456:deleteContact") && r.Method == http.MethodDelete:
			// Mock DeleteContact response
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
			return
		}
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
	newPeopleOtherContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }
	newPeopleContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &ContactsOtherDeleteCmd{}, []string{"otherContacts/abc123"}, ctx, flags); err != nil {
			t.Fatalf("delete: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v (output: %q)", err, out)
	}
	if result["deleted"] != true {
		t.Fatalf("expected deleted=true, got %v", result["deleted"])
	}
	if result["resource"] != "otherContacts/abc123" {
		t.Fatalf("expected resource=otherContacts/abc123, got %v", result["resource"])
	}
}

func TestContactsOtherDelete_Success_Text(t *testing.T) {
	origOther := newPeopleOtherContactsService
	origContacts := newPeopleContactsService
	t.Cleanup(func() {
		newPeopleOtherContactsService = origOther
		newPeopleContactsService = origContacts
	})

	copiedResourceName := "people/c789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "otherContacts/") && strings.Contains(r.URL.Path, ":copyOtherContactToMyContactsGroup") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceName": copiedResourceName,
			})
			return
		case strings.Contains(r.URL.Path, "people/c789:deleteContact") && r.Method == http.MethodDelete:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
			return
		}
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
	newPeopleOtherContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }
	newPeopleContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			// Create UI inside captureStdout so it uses the redirected os.Stdout
			u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
			if uiErr != nil {
				t.Fatalf("ui.New: %v", uiErr)
			}
			ctx := ui.WithUI(context.Background(), u)

			if err := runKong(t, &ContactsOtherDeleteCmd{}, []string{"otherContacts/xyz789"}, ctx, flags); err != nil {
				t.Fatalf("delete: %v", err)
			}
		})
	})

	if !strings.Contains(out, "deleted") || !strings.Contains(out, "true") {
		t.Fatalf("expected 'deleted' and 'true' in output, got: %q", out)
	}
	if !strings.Contains(out, "otherContacts/xyz789") {
		t.Fatalf("expected resource in output, got: %q", out)
	}
}

func TestContactsOtherDelete_CopyFailure(t *testing.T) {
	origOther := newPeopleOtherContactsService
	origContacts := newPeopleContactsService
	t.Cleanup(func() {
		newPeopleOtherContactsService = origOther
		newPeopleContactsService = origContacts
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "otherContacts/") && strings.Contains(r.URL.Path, ":copyOtherContactToMyContactsGroup") && r.Method == http.MethodPost:
			// Return error for copy operation
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    500,
					"message": "Copy failed",
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
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
	newPeopleOtherContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }
	newPeopleContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}

	err = runKong(t, &ContactsOtherDeleteCmd{}, []string{"otherContacts/abc123"}, context.Background(), flags)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "copy to my contacts:") {
		t.Fatalf("expected error to contain 'copy to my contacts:', got: %v", err)
	}
}

func TestContactsOtherDelete_DeleteFailure(t *testing.T) {
	origOther := newPeopleOtherContactsService
	origContacts := newPeopleContactsService
	t.Cleanup(func() {
		newPeopleOtherContactsService = origOther
		newPeopleContactsService = origContacts
	})

	copiedResourceName := "people/c999"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "otherContacts/") && strings.Contains(r.URL.Path, ":copyOtherContactToMyContactsGroup") && r.Method == http.MethodPost:
			// Copy succeeds
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceName": copiedResourceName,
			})
			return
		case strings.Contains(r.URL.Path, "people/c999:deleteContact") && r.Method == http.MethodDelete:
			// Delete fails
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    500,
					"message": "Delete failed",
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
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
	newPeopleOtherContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }
	newPeopleContactsService = func(context.Context, string) (*people.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com", Force: true}

	err = runKong(t, &ContactsOtherDeleteCmd{}, []string{"otherContacts/abc123"}, context.Background(), flags)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "delete copied contact") {
		t.Fatalf("expected error to contain 'delete copied contact', got: %v", err)
	}
}
