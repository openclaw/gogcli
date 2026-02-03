package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/people/v1"
)

func stubPeopleDirectoryService(t *testing.T, svc *people.Service) {
	t.Helper()
	orig := newPeopleDirectoryService
	t.Cleanup(func() { newPeopleDirectoryService = orig })
	newPeopleDirectoryService = func(context.Context, string) (*people.Service, error) { return svc, nil }
}

func TestContactsDelegatesListCmd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/delegates") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"delegates": []map[string]any{{"delegateEmail": "delegate@example.com", "verificationStatus": "accepted"}},
		})
	}))
	t.Cleanup(srv.Close)
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDelegatesListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "delegate@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestContactsDomainListCmd(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "people:listDirectoryPeople") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"people": []map[string]any{{
				"resourceName":   "people/abc",
				"names":          []map[string]any{{"displayName": "Dir Contact"}},
				"emailAddresses": []map[string]any{{"value": "dir@example.com"}},
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleDirectoryService(t, svc)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDomainListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "dir@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestContactsImportCmd(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "people:batchCreateContacts") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"createdPeople": []map[string]any{{"person": map[string]any{"resourceName": "people/c1"}}}})
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	csvPath := filepath.Join(t.TempDir(), "contacts.csv")
	if err := os.WriteFile(csvPath, []byte("name,email,phone\nAlice,alice@example.com,123\n"), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsImportCmd{File: csvPath}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Imported") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestContactsDedupCmd(t *testing.T) {
	deleted := false

	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "people/me/connections"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"connections": []map[string]any{
					{"resourceName": "people/c1", "emailAddresses": []map[string]any{{"value": "dup@example.com"}}},
					{"resourceName": "people/c2", "emailAddresses": []map[string]any{{"value": "dup@example.com"}}},
				},
			})
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "people:batchDeleteContacts"):
			deleted = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &ContactsDedupCmd{Apply: true}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleted {
		t.Fatalf("expected delete call")
	}
	if !strings.Contains(out, "Deleted") {
		t.Fatalf("unexpected output: %s", out)
	}
}
