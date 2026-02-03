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

	"google.golang.org/api/groupssettings/v1"
	"google.golang.org/api/option"
)

func TestGroupsCreateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/groups") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email": "engineering@example.com",
			"name":  "Engineering",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &GroupsCreateCmd{Email: "engineering@example.com", Name: "Engineering"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Created group") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestGroupsSettingsCmd_Get(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/groups/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email":                "engineering@example.com",
			"whoCanJoin":           "INVITED_CAN_JOIN",
			"whoCanPostMessage":    "ALL_IN_DOMAIN_CAN_POST",
			"whoCanViewGroup":      "ALL_IN_DOMAIN_CAN_VIEW",
			"whoCanViewMembership": "ALL_IN_DOMAIN_CAN_VIEW",
		})
	})
	stubGroupsSettings(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &GroupsSettingsCmd{Group: "engineering@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "WhoCanJoin") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubGroupsSettings(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newGroupsSettings
	svc, err := groupssettings.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/groups/v1/groups/"),
	)
	if err != nil {
		t.Fatalf("new groupssettings service: %v", err)
	}
	newGroupsSettings = func(context.Context, string) (*groupssettings.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newGroupsSettings = orig
		srv.Close()
	})
	return srv
}

func TestReadCSVEmails(t *testing.T) {
	content := "email\nalpha@example.com\nALPHA@example.com\nbeta@example.com\n"
	path := writeTempFile(t, content)
	got, err := readCSVEmails(path)
	if err != nil {
		t.Fatalf("readCSVEmails: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 emails, got %d", len(got))
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "csv-*.csv")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := io.WriteString(f, content); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return f.Name()
}

func TestGroupsUpdateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/groups/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email": "engineering@example.com",
			"name":  "Engineering Team",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newName := "Engineering Team"
	cmd := &GroupsUpdateCmd{Group: "engineering@example.com", Name: &newName}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Updated group") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestGroupsDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/groups/") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &GroupsDeleteCmd{Group: "old-group@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted group") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestGroupsSettingsCmd_Update(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/groups/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email":      "engineering@example.com",
			"whoCanJoin": "CAN_REQUEST_TO_JOIN",
		})
	})
	stubGroupsSettings(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	whoCanJoin := "CAN_REQUEST_TO_JOIN"
	cmd := &GroupsSettingsCmd{Group: "engineering@example.com", WhoCanJoin: &whoCanJoin}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Updated settings") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestGroupsMembersAddCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/members") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email": "newmember@example.com",
			"role":  "MEMBER",
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &GroupsMembersAddCmd{Group: "team@example.com", Email: "newmember@example.com", Role: "MEMBER"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Added") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestGroupsMembersRemoveCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/members/") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &GroupsMembersRemoveCmd{Group: "team@example.com", Email: "oldmember@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Removed") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestGroupsMembersSyncCmd_AlreadyInSync(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/members") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"members": []map[string]any{
				{"email": "alpha@example.com", "role": "MEMBER"},
				{"email": "beta@example.com", "role": "MEMBER"},
			},
		})
	})
	stubAdminDirectory(t, h)

	csvContent := "email\nalpha@example.com\nbeta@example.com\n"
	csvPath := writeTempFile(t, csvContent)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &GroupsMembersSyncCmd{Group: "team@example.com", File: csvPath}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "already in sync") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestNormalizeGroupRole(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"MEMBER", "MEMBER", false},
		{"member", "MEMBER", false},
		{"Manager", "MANAGER", false},
		{"owner", "OWNER", false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := normalizeGroupRole(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("normalizeGroupRole(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFindEmailColumn(t *testing.T) {
	tests := []struct {
		header   []string
		expected int
	}{
		{[]string{"email", "name"}, 0},
		{[]string{"name", "EmailAddress"}, 1},
		{[]string{"member", "role"}, 0},
		{[]string{"MEMBER_EMAIL", "status"}, 0},
		{[]string{"name", "role"}, -1},
	}

	for _, tt := range tests {
		got := findEmailColumn(tt.header)
		if got != tt.expected {
			t.Errorf("findEmailColumn(%v) = %d, want %d", tt.header, got, tt.expected)
		}
	}
}

func TestListGroupMembers(t *testing.T) {
	calls := 0
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/members") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		calls++
		if calls == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"members": []map[string]any{
					{"email": "a@example.com"},
					{"email": "b@example.com"},
				},
				"nextPageToken": "page2",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"members": []map[string]any{
				{"email": "c@example.com"},
			},
		})
	})
	srv := stubAdminDirectory(t, h)
	_ = srv

	svc, _ := newAdminDirectoryForServer(httptest.NewServer(h))
	members, err := listGroupMembers(context.Background(), svc, "team@example.com")
	if err != nil {
		t.Fatalf("listGroupMembers: %v", err)
	}
	if len(members) != 3 {
		t.Errorf("expected 3 members, got %d", len(members))
	}
}
