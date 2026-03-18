package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/option"
)

func TestRequireAdminAccount_ConsumerBlocked(t *testing.T) {
	account, err := requireAdminAccount(&RootFlags{Account: "user@gmail.com"})
	if err == nil {
		t.Fatal("expected error")
	}
	if account != "" {
		t.Fatalf("expected empty account, got %q", account)
	}
	if !strings.Contains(err.Error(), "Google Workspace account") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrapAdminDirectoryError_MapsPermissions(t *testing.T) {
	err := wrapAdminDirectoryError(errors.New("insufficient authentication scopes"), "svc@example.com")
	if err == nil || !strings.Contains(err.Error(), "admin.directory.group.member") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdminUsersCreate_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	flags := &RootFlags{Account: "svc@example.com"}

	tests := []struct {
		name string
		cmd  AdminUsersCreateCmd
		want string
	}{
		{name: "missing email", cmd: AdminUsersCreateCmd{GivenName: "Ada", FamilyName: "Lovelace", Password: "pw"}, want: "email required"},
		{name: "missing given", cmd: AdminUsersCreateCmd{Email: "ada@example.com", FamilyName: "Lovelace", Password: "pw"}, want: "--given required"},
		{name: "missing family", cmd: AdminUsersCreateCmd{Email: "ada@example.com", GivenName: "Ada", Password: "pw"}, want: "--family required"},
		{name: "missing password", cmd: AdminUsersCreateCmd{Email: "ada@example.com", GivenName: "Ada", FamilyName: "Lovelace"}, want: "--password required"},
		{name: "admin unsupported", cmd: AdminUsersCreateCmd{Email: "ada@example.com", GivenName: "Ada", FamilyName: "Lovelace", Password: "pw", Admin: true}, want: "--admin is not supported"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cmd.Run(ctx, flags); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestAdminUsersList_JSON_AllowsNilName(t *testing.T) {
	origNew := newAdminDirectoryService
	t.Cleanup(func() { newAdminDirectoryService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"users": []map[string]any{
				{
					"primaryEmail": "ada@example.com",
					"suspended":    false,
					"isAdmin":      true,
				},
			},
		})
	}))
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAdminDirectoryService = func(context.Context, string) (*admin.Service, error) { return svc, nil }

	ctx := newCmdJSONContext(t)

	out := captureStdout(t, func() {
		if err := (&AdminUsersListCmd{Domain: "example.com"}).Run(ctx, &RootFlags{Account: "svc@example.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed struct {
		Users []struct {
			Email string `json:"email"`
			Name  string `json:"name"`
			Admin bool   `json:"admin"`
		} `json:"users"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Users) != 1 || parsed.Users[0].Email != "ada@example.com" || parsed.Users[0].Name != "" || !parsed.Users[0].Admin {
		t.Fatalf("unexpected users: %#v", parsed.Users)
	}
}

func TestAdminGroupsMembersAdd_JSON(t *testing.T) {
	origNew := newAdminDirectoryService
	t.Cleanup(func() { newAdminDirectoryService = origNew })

	var gotRole string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/members")) {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotRole, _ = body["role"].(string)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email": "dev@example.com",
			"role":  gotRole,
		})
	}))
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAdminDirectoryService = func(context.Context, string) (*admin.Service, error) { return svc, nil }

	ctx := newCmdJSONContext(t)

	out := captureStdout(t, func() {
		if err := (&AdminGroupsMembersAddCmd{
			GroupEmail:  "eng@example.com",
			MemberEmail: "dev@example.com",
			Role:        "owner",
		}).Run(ctx, &RootFlags{Account: "svc@example.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotRole != adminRoleOwner {
		t.Fatalf("unexpected role sent: %q", gotRole)
	}
	var parsed struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Email != "dev@example.com" || parsed.Role != adminRoleOwner {
		t.Fatalf("unexpected response: %#v", parsed)
	}
}
