package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/people/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func stubPeopleDirectoryService(t *testing.T, svc *people.Service) {
	t.Helper()
	orig := newPeopleDirectoryService
	t.Cleanup(func() { newPeopleDirectoryService = orig })
	newPeopleDirectoryService = func(context.Context, string) (*people.Service, error) { return svc, nil }
}

// testContextWithStderr creates a UI context that writes to os.Stderr for capturing stderr.
func testContextWithStderr(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
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

// --- ContactsDelegatesAddCmd Tests ---

func TestContactsDelegatesAddCmd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/delegates") {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"delegateEmail":      body["delegateEmail"],
			"verificationStatus": "pending",
		})
	}))
	t.Cleanup(srv.Close)
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDelegatesAddCmd{Delegate: "new-delegate@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Added delegate") || !strings.Contains(out, "new-delegate@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestContactsDelegatesAddCmd_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/delegates") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"delegateEmail":      "new-delegate@example.com",
			"verificationStatus": "pending",
		})
	}))
	t.Cleanup(srv.Close)
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDelegatesAddCmd{Delegate: "new-delegate@example.com"}

	out := captureStdout(t, func() {
		u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v (output: %q)", err, out)
	}
	if result["delegateEmail"] != "new-delegate@example.com" {
		t.Fatalf("unexpected delegateEmail: %v", result["delegateEmail"])
	}
}

func TestContactsDelegatesAddCmd_EmptyDelegate(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDelegatesAddCmd{Delegate: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil || !strings.Contains(err.Error(), "--delegate is required") {
		t.Fatalf("expected delegate required error, got: %v", err)
	}
}

// --- ContactsDelegatesRemoveCmd Tests ---

func TestContactsDelegatesRemoveCmd(t *testing.T) {
	var deletedDelegate string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/delegates/") {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(r.URL.Path, "/delegates/")
		if len(parts) > 1 {
			deletedDelegate = parts[1]
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &ContactsDelegatesRemoveCmd{Delegate: "remove-me@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if deletedDelegate != "remove-me@example.com" {
		t.Fatalf("unexpected deleted delegate: %s", deletedDelegate)
	}
	if !strings.Contains(out, "Removed delegate") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestContactsDelegatesRemoveCmd_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/delegates/") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &ContactsDelegatesRemoveCmd{Delegate: "remove-me@example.com"}

	out := captureStdout(t, func() {
		u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v (output: %q)", err, out)
	}
	if result["removed"] != true {
		t.Fatalf("expected removed=true, got %v", result["removed"])
	}
	if result["delegate"] != "remove-me@example.com" {
		t.Fatalf("unexpected delegate: %v", result["delegate"])
	}
}

func TestContactsDelegatesRemoveCmd_EmptyDelegate(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &ContactsDelegatesRemoveCmd{Delegate: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil || !strings.Contains(err.Error(), "--delegate is required") {
		t.Fatalf("expected delegate required error, got: %v", err)
	}
}

// --- ContactsDelegatesListCmd JSON Test ---

func TestContactsDelegatesListCmd_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/delegates") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"delegates": []map[string]any{
				{"delegateEmail": "delegate1@example.com", "verificationStatus": "accepted"},
				{"delegateEmail": "delegate2@example.com", "verificationStatus": "pending"},
			},
		})
	}))
	t.Cleanup(srv.Close)
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDelegatesListCmd{}

	out := captureStdout(t, func() {
		u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v (output: %q)", err, out)
	}
	delegates, ok := result["delegates"].([]any)
	if !ok || len(delegates) != 2 {
		t.Fatalf("unexpected delegates: %v", result["delegates"])
	}
}

func TestContactsDelegatesListCmd_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/delegates") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"delegates": []map[string]any{}})
	}))
	t.Cleanup(srv.Close)
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDelegatesListCmd{}

	errOut := captureStderr(t, func() {
		if err := cmd.Run(testContextWithStderr(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(errOut, "No delegates") {
		t.Fatalf("expected 'No delegates' message, got: %s", errOut)
	}
}

// --- ContactsDomainCreateCmd Tests ---

func TestContactsDomainCreateCmd(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "people:createContact") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceName":   "people/c123",
			"names":          []map[string]any{{"displayName": "New Contact"}},
			"emailAddresses": []map[string]any{{"value": "new@example.com"}},
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDomainCreateCmd{Email: "new@example.com", Name: "New Contact"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Created contact") || !strings.Contains(out, "people/c123") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestContactsDomainCreateCmd_JSON(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "people:createContact") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceName":   "people/c456",
			"names":          []map[string]any{{"displayName": "JSON Contact"}},
			"emailAddresses": []map[string]any{{"value": "json@example.com"}},
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDomainCreateCmd{Email: "json@example.com", Name: "JSON Contact"}

	out := captureStdout(t, func() {
		u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v (output: %q)", err, out)
	}
	if result["resourceName"] != "people/c456" {
		t.Fatalf("unexpected resourceName: %v", result["resourceName"])
	}
}

func TestContactsDomainCreateCmd_MissingFields(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}

	cmd1 := &ContactsDomainCreateCmd{Email: "", Name: "Test"}
	err1 := cmd1.Run(testContext(t), flags)
	if err1 == nil || !strings.Contains(err1.Error(), "--email and --name are required") {
		t.Fatalf("expected required fields error, got: %v", err1)
	}

	cmd2 := &ContactsDomainCreateCmd{Email: "test@example.com", Name: ""}
	err2 := cmd2.Run(testContext(t), flags)
	if err2 == nil || !strings.Contains(err2.Error(), "--email and --name are required") {
		t.Fatalf("expected required fields error, got: %v", err2)
	}
}

// --- ContactsDomainDeleteCmd Tests ---

func TestContactsDomainDeleteCmd_ByResourceName(t *testing.T) {
	var deletedResource string
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, ":deleteContact") {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		for i, p := range parts {
			if strings.HasPrefix(p, "people") && i+1 < len(parts) {
				deletedResource = "people/" + strings.Split(parts[i+1], ":")[0]
				break
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &ContactsDomainDeleteCmd{Email: "people/c789"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if deletedResource != "people/c789" {
		t.Fatalf("unexpected deleted resource: %s", deletedResource)
	}
	if !strings.Contains(out, "Deleted contact") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestContactsDomainDeleteCmd_ByEmail(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "people:searchContacts"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"person": map[string]any{"resourceName": "people/found123"}},
				},
			})
			return
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, ":deleteContact"):
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &ContactsDomainDeleteCmd{Email: "search@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted contact") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestContactsDomainDeleteCmd_JSON(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, ":deleteContact") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &ContactsDomainDeleteCmd{Email: "people/c999"}

	out := captureStdout(t, func() {
		u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v (output: %q)", err, out)
	}
	if result["deleted"] != true {
		t.Fatalf("expected deleted=true, got %v", result["deleted"])
	}
}

func TestContactsDomainDeleteCmd_NotFound(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "people:searchContacts") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &ContactsDomainDeleteCmd{Email: "notfound@example.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil || !strings.Contains(err.Error(), "no contact found") {
		t.Fatalf("expected not found error, got: %v", err)
	}
}

func TestContactsDomainDeleteCmd_EmptyIdentifier(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com", Force: true}
	cmd := &ContactsDomainDeleteCmd{Email: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil || !strings.Contains(err.Error(), "email is required") {
		t.Fatalf("expected email required error, got: %v", err)
	}
}

// --- ContactsDomainListCmd JSON and Empty Tests ---

func TestContactsDomainListCmd_JSON(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "people:listDirectoryPeople") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"people": []map[string]any{
				{"resourceName": "people/d1", "names": []map[string]any{{"displayName": "Dir1"}}},
				{"resourceName": "people/d2", "names": []map[string]any{{"displayName": "Dir2"}}},
			},
			"nextPageToken": "token123",
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleDirectoryService(t, svc)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDomainListCmd{}

	out := captureStdout(t, func() {
		u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v (output: %q)", err, out)
	}
	if result["nextPageToken"] != "token123" {
		t.Fatalf("unexpected nextPageToken: %v", result["nextPageToken"])
	}
}

func TestContactsDomainListCmd_Empty(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "people:listDirectoryPeople") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"people": []map[string]any{}})
	}))
	t.Cleanup(closeSrv)
	stubPeopleDirectoryService(t, svc)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDomainListCmd{}

	errOut := captureStderr(t, func() {
		if err := cmd.Run(testContextWithStderr(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(errOut, "No domain contacts") {
		t.Fatalf("expected 'No domain contacts' message, got: %s", errOut)
	}
}

// --- ContactsImportCmd Additional Tests ---

func TestContactsImportCmd_JSON(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "people:batchCreateContacts") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"createdPeople": []map[string]any{
				{"person": map[string]any{"resourceName": "people/c1"}},
				{"person": map[string]any{"resourceName": "people/c2"}},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	csvPath := filepath.Join(t.TempDir(), "contacts.csv")
	if err := os.WriteFile(csvPath, []byte("name,email,phone\nAlice,alice@example.com,123\nBob,bob@example.com,456\n"), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsImportCmd{File: csvPath}

	out := captureStdout(t, func() {
		u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v (output: %q)", err, out)
	}
	if result["createdPeople"] == nil {
		t.Fatalf("expected createdPeople in output")
	}
}

func TestContactsImportCmd_EmptyCSV(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "empty.csv")
	if err := os.WriteFile(csvPath, []byte(""), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsImportCmd{File: csvPath}

	err := cmd.Run(testContext(t), flags)
	if err == nil || !strings.Contains(err.Error(), "empty csv") {
		t.Fatalf("expected empty csv error, got: %v", err)
	}
}

func TestContactsImportCmd_HeaderOnlyCSV(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "header_only.csv")
	if err := os.WriteFile(csvPath, []byte("name,email,phone\n"), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsImportCmd{File: csvPath}

	err := cmd.Run(testContext(t), flags)
	if err == nil || !strings.Contains(err.Error(), "no contacts found") {
		t.Fatalf("expected no contacts error, got: %v", err)
	}
}

func TestContactsImportCmd_FileNotFound(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsImportCmd{File: "/nonexistent/path/contacts.csv"}

	err := cmd.Run(testContext(t), flags)
	if err == nil || !strings.Contains(err.Error(), "open csv") {
		t.Fatalf("expected file not found error, got: %v", err)
	}
}

// --- ContactsExportCmd Tests ---

func TestContactsExportCmd(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "people/me/connections") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"connections": []map[string]any{
				{
					"resourceName":   "people/c1",
					"names":          []map[string]any{{"displayName": "Alice"}},
					"emailAddresses": []map[string]any{{"value": "alice@example.com"}},
					"phoneNumbers":   []map[string]any{{"value": "+1234567890"}},
				},
				{
					"resourceName":   "people/c2",
					"names":          []map[string]any{{"displayName": "Bob"}},
					"emailAddresses": []map[string]any{{"value": "bob@example.com"}},
				},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	csvPath := filepath.Join(t.TempDir(), "exported.csv")
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsExportCmd{File: csvPath}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Exported 2 contacts") {
		t.Fatalf("unexpected output: %s", out)
	}

	content, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}

	if !strings.Contains(string(content), "Alice") || !strings.Contains(string(content), "alice@example.com") {
		t.Fatalf("exported CSV missing expected data: %s", content)
	}
}

func TestContactsExportCmd_JSON(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "people/me/connections") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"connections": []map[string]any{
				{"resourceName": "people/c1", "names": []map[string]any{{"displayName": "Alice"}}},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	csvPath := filepath.Join(t.TempDir(), "exported_json.csv")
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsExportCmd{File: csvPath}

	out := captureStdout(t, func() {
		u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v (output: %q)", err, out)
	}
	if result["exported"].(float64) != 1 {
		t.Fatalf("expected exported=1, got %v", result["exported"])
	}
}

func TestContactsExportCmd_ToStdout(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "people/me/connections") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"connections": []map[string]any{
				{"resourceName": "people/c1", "names": []map[string]any{{"displayName": "Alice"}}},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsExportCmd{File: "-"}

	out := captureStdout(t, func() {
		u, _ := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		ctx := ui.WithUI(context.Background(), u)
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "name,email,phone") {
		t.Fatalf("expected CSV header in stdout output: %s", out)
	}
}

// --- ContactsDedupCmd Additional Tests ---

func TestContactsDedupCmd_NoDuplicates(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "people/me/connections") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"connections": []map[string]any{
				{"resourceName": "people/c1", "emailAddresses": []map[string]any{{"value": "unique1@example.com"}}},
				{"resourceName": "people/c2", "emailAddresses": []map[string]any{{"value": "unique2@example.com"}}},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDedupCmd{}

	errOut := captureStderr(t, func() {
		if err := cmd.Run(testContextWithStderr(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(errOut, "No duplicates") {
		t.Fatalf("expected 'No duplicates' message, got: %s", errOut)
	}
}

func TestContactsDedupCmd_JSON(t *testing.T) {
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
		u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json unmarshal: %v (output: %q)", err, out)
	}
	if result["deleted"].(float64) != 1 {
		t.Fatalf("expected deleted=1, got %v", result["deleted"])
	}
}

func TestContactsDedupCmd_DryRun(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "people/me/connections") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"connections": []map[string]any{
				{"resourceName": "people/c1", "emailAddresses": []map[string]any{{"value": "dup@example.com"}}},
				{"resourceName": "people/c2", "emailAddresses": []map[string]any{{"value": "dup@example.com"}}},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDedupCmd{Apply: false}

	// The "Run with --apply" hint is written to stdout via ui.Printf in dry-run mode
	// We verify the duplicates are shown
	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	// The dry-run output should contain duplicate info and the --apply hint
	if !strings.Contains(out, "dup@example.com") && !strings.Contains(out, "duplicates") {
		t.Fatalf("expected duplicate info in output: %s", out)
	}
}

// --- CSV Helper Tests ---

func TestNormalizeCSVHeader(t *testing.T) {
	input := []string{"  Name ", "EMAIL", " Phone "}
	expected := []string{"name", "email", "phone"}

	result := normalizeCSVHeader(input)
	for i, v := range result {
		if v != expected[i] {
			t.Fatalf("expected %q at index %d, got %q", expected[i], i, v)
		}
	}
}

func TestParseCSVRow(t *testing.T) {
	header := []string{"name", "email", "phone"}
	row := []string{"  Alice ", "alice@example.com", " 123 "}

	result := parseCSVRow(header, row)
	if result["name"] != "Alice" {
		t.Fatalf("expected name='Alice', got %q", result["name"])
	}
	if result["email"] != "alice@example.com" {
		t.Fatalf("expected email='alice@example.com', got %q", result["email"])
	}
	if result["phone"] != "123" {
		t.Fatalf("expected phone='123', got %q", result["phone"])
	}
}

func TestParseCSVRow_ShortRow(t *testing.T) {
	header := []string{"name", "email", "phone"}
	row := []string{"Alice"}

	result := parseCSVRow(header, row)
	if result["name"] != "Alice" {
		t.Fatalf("expected name='Alice', got %q", result["name"])
	}
	if result["email"] != "" {
		t.Fatalf("expected email='', got %q", result["email"])
	}
}

func TestCsvPerson(t *testing.T) {
	tests := []struct {
		name   string
		entry  map[string]string
		expect bool
	}{
		{"with name", map[string]string{"name": "Alice"}, true},
		{"with given", map[string]string{"given": "Alice"}, true},
		{"with family", map[string]string{"family": "Smith"}, true},
		{"with email", map[string]string{"email": "a@b.com"}, true},
		{"with phone", map[string]string{"phone": "123"}, true},
		{"empty", map[string]string{}, false},
		{"only whitespace", map[string]string{"name": "", "email": "", "phone": ""}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := csvPerson(tc.entry)
			if tc.expect && result == nil {
				t.Fatalf("expected non-nil person")
			}
			if !tc.expect && result != nil {
				t.Fatalf("expected nil person")
			}
		})
	}
}

func TestOpenCSVReader_EmptyPath(t *testing.T) {
	_, _, err := openCSVReader("  ")
	if err == nil || !strings.Contains(err.Error(), "file is required") {
		t.Fatalf("expected file required error, got: %v", err)
	}
}

func TestOpenCSVWriter_EmptyPath(t *testing.T) {
	_, _, err := openCSVWriter("  ")
	if err == nil || !strings.Contains(err.Error(), "file is required") {
		t.Fatalf("expected file required error, got: %v", err)
	}
}

// --- Delegates with User Override Tests ---

func TestContactsDelegatesListCmd_WithUserOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/delegates") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"delegates": []map[string]any{}})
	}))
	t.Cleanup(srv.Close)
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &ContactsDelegatesListCmd{User: "other@example.com"}

	_ = captureStderr(t, func() {
		_ = cmd.Run(testContextWithStderr(t), flags)
	})
}
