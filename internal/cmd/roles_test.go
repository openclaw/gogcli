package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRolesListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"roleId":           "123",
						"roleName":         "Helpdesk",
						"isSystemRole":     false,
						"isSuperAdminRole": false,
						"rolePrivileges":   []map[string]any{{"privilegeName": "READ"}},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &RolesListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Helpdesk") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRolesCreateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "privileges"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"privilegeName": "READ", "serviceId": "svc"},
					{"privilegeName": "WRITE", "serviceId": "svc"},
				},
			})
			return
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"roleId":   "456",
				"roleName": "Helpdesk",
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &RolesCreateCmd{Name: "Helpdesk", Privileges: "READ,WRITE"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Created role") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAdminsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"roleId": "123", "roleName": "Helpdesk"},
				},
			})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roleassignments"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"roleAssignmentId": "1",
						"roleId":           "123",
						"assignedTo":       "user-1",
						"scopeType":        "CUSTOMER",
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AdminsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Helpdesk") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAdminsCreateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"roleId": "123", "roleName": "Helpdesk"},
				},
			})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-1"})
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/roleassignments"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"roleAssignmentId": "99"})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AdminsCreateCmd{User: "sam@example.com", Role: "Helpdesk"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Assigned role") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAdminsDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/roleassignments/"):
			w.WriteHeader(http.StatusOK)
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &AdminsDeleteCmd{AssignmentID: "99"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted admin assignment") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRolesGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"roleId":           "123",
				"roleName":         "Super Admin",
				"isSystemRole":     true,
				"isSuperAdminRole": true,
				"roleDescription":  "Full access",
				"rolePrivileges": []map[string]any{
					{"privilegeName": "ADMIN_READ"},
					{"privilegeName": "ADMIN_WRITE"},
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &RolesGetCmd{Role: "123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Super Admin") || !strings.Contains(out, "Full access") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRolesUpdateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"roleId":         "456",
				"roleName":       "Custom Role",
				"rolePrivileges": []map[string]any{{"privilegeName": "READ", "serviceId": "svc"}},
			})
			return
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/roles/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"roleId":   "456",
				"roleName": "Updated Role",
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newName := "Updated Role"
	cmd := &RolesUpdateCmd{Role: "456", Name: &newName}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Updated role") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRolesDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"roleId": "789", "roleName": "ToDelete"},
				},
			})
			return
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/roles/"):
			w.WriteHeader(http.StatusOK)
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &RolesDeleteCmd{Role: "ToDelete"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted role") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRolesPrivilegesCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/privileges"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"privilegeName": "ADMIN_READ", "serviceId": "admin.directory", "isOuScopable": true},
					{"privilegeName": "ADMIN_WRITE", "serviceId": "admin.directory", "isOuScopable": false},
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &RolesPrivilegesCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "ADMIN_READ") || !strings.Contains(out, "ADMIN_WRITE") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRolesUpdateCmd_AddRemovePrivileges(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/privileges"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"privilegeName": "READ", "serviceId": "svc"},
					{"privilegeName": "WRITE", "serviceId": "svc"},
					{"privilegeName": "DELETE", "serviceId": "svc"},
				},
			})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/roles/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"roleId":   "456",
				"roleName": "Custom Role",
				"rolePrivileges": []map[string]any{
					{"privilegeName": "READ", "serviceId": "svc"},
				},
			})
			return
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/roles/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"roleId":   "456",
				"roleName": "Custom Role",
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &RolesUpdateCmd{Role: "456", AddPrivileges: "WRITE", RemovePrivileges: "READ"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Updated role") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRolesGetCmd_ByName(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/roles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"roleId":           "999",
						"roleName":         "CustomRole",
						"isSystemRole":     false,
						"isSuperAdminRole": false,
						"roleDescription":  "A custom role",
						"rolePrivileges":   []map[string]any{{"privilegeName": "SOME_PRIVILEGE"}},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &RolesGetCmd{Role: "CustomRole"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "CustomRole") || !strings.Contains(out, "A custom role") {
		t.Fatalf("unexpected output: %s", out)
	}
}
