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

	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestUsersListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/users") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"users": []map[string]any{
				{
					"primaryEmail":  "alex@example.com",
					"name":          map[string]any{"givenName": "Alex", "familyName": "Admin"},
					"orgUnitPath":   "/",
					"suspended":     false,
					"isAdmin":       true,
					"lastLoginTime": "2026-01-01T00:00:00Z",
				},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &UsersListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "users") {
		t.Fatalf("expected JSON users output, got: %s", out)
	}
}

func TestUsersGetCmd_Plain(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"primaryEmail": "sam@example.com",
			"name":         map[string]any{"givenName": "Sam", "familyName": "User"},
			"id":           "user-1",
			"isAdmin":      false,
			"suspended":    false,
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &UsersGetCmd{User: "sam@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Email:") || !strings.Contains(out, "sam@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestUsersUpdateCmd_AdminOnly(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/makeAdmin") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": true})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &UsersUpdateCmd{User: "sam@example.com", Admin: boolPtr(true)}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestUsersCountCmd(t *testing.T) {
	calls := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/users") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		calls++
		if calls == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"users": []map[string]any{
					{"orgUnitPath": "/"},
					{"orgUnitPath": "/Sales"},
				},
				"nextPageToken": "next",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"users": []map[string]any{
				{"orgUnitPath": "/Sales"},
			},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &UsersCountCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "/Sales") || !strings.Contains(out, "2") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubAdminDirectory(t *testing.T, handler http.Handler) {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newAdminDirectory
	svc, err := newAdminDirectoryForServer(srv)
	if err != nil {
		t.Fatalf("new admin service: %v", err)
	}
	newAdminDirectory = func(context.Context, string) (*admin.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newAdminDirectory = orig
		srv.Close()
	})
}

func newAdminDirectoryForServer(srv *httptest.Server) (*admin.Service, error) {
	return admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func testContextWithStdout(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func boolPtr(v bool) *bool { return &v }
