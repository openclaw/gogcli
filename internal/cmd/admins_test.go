package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
)

// -----------------------------------------------------------------------------
// resolveOrgUnitID tests
// -----------------------------------------------------------------------------

func TestResolveOrgUnitID_Success(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"orgUnitPath": "/Sales",
			"orgUnitId":   "ou-123456",
			"name":        "Sales",
		})
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new admin service: %v", err)
	}

	id, err := resolveOrgUnitID(context.Background(), svc, "/Sales")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "ou-123456" {
		t.Fatalf("expected ou-123456, got %q", id)
	}
}

func TestResolveOrgUnitID_NotFound(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": {"message": "not found"}}`, http.StatusNotFound)
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new admin service: %v", err)
	}

	_, err = resolveOrgUnitID(context.Background(), svc, "/NonExistent")
	if err == nil {
		t.Fatalf("expected error for not found org unit")
	}
	if !strings.Contains(err.Error(), "resolve org unit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveOrgUnitID_EmptyID(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"orgUnitPath": "/BadOU",
			"orgUnitId":   "",
			"name":        "BadOU",
		})
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new admin service: %v", err)
	}

	_, err = resolveOrgUnitID(context.Background(), svc, "/BadOU")
	if err == nil {
		t.Fatalf("expected error for empty org unit ID")
	}
	if !strings.Contains(err.Error(), "has no ID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// resolveUserID tests
// -----------------------------------------------------------------------------

func TestResolveUserID_Success(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "user-abc123",
			"primaryEmail": "user@example.com",
		})
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new admin service: %v", err)
	}

	id, err := resolveUserID(context.Background(), svc, "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "user-abc123" {
		t.Fatalf("expected user-abc123, got %q", id)
	}
}

func TestResolveUserID_EmptyUser(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new admin service: %v", err)
	}

	_, err = resolveUserID(context.Background(), svc, "   ")
	if err == nil {
		t.Fatalf("expected error for empty user")
	}
	if !strings.Contains(err.Error(), "user required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveUserID_EmptyID(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "",
			"primaryEmail": "nouser@example.com",
		})
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new admin service: %v", err)
	}

	_, err = resolveUserID(context.Background(), svc, "nouser@example.com")
	if err == nil {
		t.Fatalf("expected error for empty user ID")
	}
	if !strings.Contains(err.Error(), "has no ID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// AdminsListCmd tests
// -----------------------------------------------------------------------------

func TestAdminsListCmd_NoAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &AdminsListCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatalf("expected error for missing account")
	}
}

func TestAdminsListCmd_EmptyResults(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roleassignments"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AdminsListCmd{}

	// Empty results should not error, just print message to stderr
	err := cmd.Run(testContext(t), flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdminsListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"roleId": "100", "roleName": "Admin"},
				},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roleassignments"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"roleAssignmentId": "1",
						"roleId":           "100",
						"assignedTo":       "user-1",
						"scopeType":        "CUSTOMER",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AdminsListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "items") {
		t.Fatalf("expected JSON items output: %s", out)
	}
}

func TestAdminsListCmd_WithPagination(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"roleId": "100", "roleName": "Admin"}},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roleassignments"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"roleAssignmentId": "1", "roleId": "100", "assignedTo": "user-1", "scopeType": "CUSTOMER"},
				},
				"nextPageToken": "next-page",
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AdminsListCmd{Max: 1}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Admin") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAdminsListCmd_NilAssignment(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roleassignments"):
			w.Header().Set("Content-Type", "application/json")
			// Simulate response with null items (testing nil check in loop)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					nil,
					{"roleAssignmentId": "2", "roleId": "100", "assignedTo": "user-2", "scopeType": "CUSTOMER"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AdminsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "user-2") {
		t.Fatalf("unexpected output: %s", out)
	}
}

// -----------------------------------------------------------------------------
// AdminsCreateCmd tests
// -----------------------------------------------------------------------------

func TestAdminsCreateCmd_NoAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &AdminsCreateCmd{User: "sam@example.com", Role: "Admin"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatalf("expected error for missing account")
	}
}

func TestAdminsCreateCmd_WithOrgUnit(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"roleId": "123", "roleName": "Helpdesk"}},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-1"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/orgunits/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"orgUnitPath": "/Sales",
				"orgUnitId":   "ou-sales-123",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/roleassignments"):
			w.Header().Set("Content-Type", "application/json")
			// Verify request body contains ORG_UNIT scope
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["scopeType"] != "ORG_UNIT" {
				t.Errorf("expected ORG_UNIT scope, got %v", payload["scopeType"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"roleAssignmentId": "99"})
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AdminsCreateCmd{User: "sam@example.com", Role: "Helpdesk", OrgUnit: "/Sales"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Assigned role") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAdminsCreateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"roleId": "123", "roleName": "Helpdesk"}},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-1"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/roleassignments"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"roleAssignmentId": "99",
				"roleId":           "123",
				"assignedTo":       "user-1",
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AdminsCreateCmd{User: "sam@example.com", Role: "Helpdesk"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "roleAssignmentId") {
		t.Fatalf("expected JSON roleAssignmentId: %s", out)
	}
}

func TestAdminsCreateCmd_RoleNotFound(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			// Return roles that don't match the requested one
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"roleId": 123, "roleName": "DifferentRole"}},
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AdminsCreateCmd{User: "sam@example.com", Role: "NonExistentRole"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatalf("expected error for role not found")
	}
	if !strings.Contains(err.Error(), "role") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// AdminsDeleteCmd tests
// -----------------------------------------------------------------------------

func TestAdminsDeleteCmd_NoAccount(t *testing.T) {
	flags := &RootFlags{Force: true}
	cmd := &AdminsDeleteCmd{AssignmentID: "99"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatalf("expected error for missing account")
	}
}

func TestAdminsDeleteCmd_RequiresConfirmation(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", NoInput: true}
	cmd := &AdminsDeleteCmd{AssignmentID: "99"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatalf("expected error without --force in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "refusing to delete") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdminsDeleteCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": {"message": "forbidden"}}`, http.StatusForbidden)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &AdminsDeleteCmd{AssignmentID: "99"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "delete admin assignment") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// roleIDNameMap tests
// -----------------------------------------------------------------------------

func TestRoleIDNameMap_Success(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/roles") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"roleId": "100", "roleName": "Super Admin"},
				{"roleId": "200", "roleName": "Helpdesk"},
				nil, // Test nil handling
			},
		})
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new admin service: %v", err)
	}

	m, err := roleIDNameMap(context.Background(), svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["100"] != "Super Admin" {
		t.Fatalf("expected Super Admin, got %q", m["100"])
	}
	if m["200"] != "Helpdesk" {
		t.Fatalf("expected Helpdesk, got %q", m["200"])
	}
}

func TestRoleIDNameMap_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": {"message": "forbidden"}}`, http.StatusForbidden)
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	svc, err := admin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new admin service: %v", err)
	}

	_, err = roleIDNameMap(context.Background(), svc)
	if err == nil {
		t.Fatalf("expected error from API")
	}
}
