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

	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func newCloudIdentityTestService(t *testing.T, handler http.HandlerFunc) (*cloudidentity.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(handler)
	svc, err := cloudidentity.NewService(context.Background(),
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

func stubCloudIdentityAdminService(t *testing.T, svc *cloudidentity.Service) {
	t.Helper()

	orig := newCloudIdentityAdminService
	t.Cleanup(func() { newCloudIdentityAdminService = orig })
	newCloudIdentityAdminService = func(context.Context, string) (*cloudidentity.Service, error) {
		return svc, nil
	}
}

func TestCloudIdentityGroupsListCmd(t *testing.T) {
	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/groups") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"groups": []map[string]any{{
				"name":        "groups/123",
				"groupKey":    map[string]any{"id": "group@example.com"},
				"displayName": "Example Group",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CloudIdentityGroupsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "group@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCloudIdentityMembersAddCmd(t *testing.T) {
	var gotEmail string
	var gotRole string

	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/groups:lookup":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/123"})
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/groups/123/memberships"):
			var payload cloudidentity.Membership
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload.PreferredMemberKey != nil {
				gotEmail = payload.PreferredMemberKey.Id
			}
			if len(payload.Roles) > 0 && payload.Roles[0] != nil {
				gotRole = payload.Roles[0].Name
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/op1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CloudIdentityMembersAddCmd{Group: "group@example.com", Email: "user@example.com", Role: "MANAGER"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotEmail != "user@example.com" || gotRole != "MANAGER" {
		t.Fatalf("unexpected membership payload: %s %s", gotEmail, gotRole)
	}
	if !strings.Contains(out, "Added") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCloudIdentityPoliciesListCmd(t *testing.T) {
	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/policies") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"policies": []map[string]any{{
				"name":    "policies/1",
				"etag":    "abc",
				"setting": map[string]any{"type": "settings/gmail.service_status"},
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CloudIdentityPoliciesListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "policies/1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCloudIdentityGroupsGetCmd(t *testing.T) {
	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/groups:lookup":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/123"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/groups/123":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "groups/123",
				"groupKey":    map[string]any{"id": "group@example.com"},
				"displayName": "Example Group",
				"parent":      "customers/my_customer",
				"labels":      map[string]any{"cloudidentity.googleapis.com/groups.discussion_forum": ""},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CloudIdentityGroupsGetCmd{Group: "group@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "groups/123") {
		t.Fatalf("expected name in output: %s", out)
	}
	if !strings.Contains(out, "group@example.com") {
		t.Fatalf("expected email in output: %s", out)
	}
	if !strings.Contains(out, "Example Group") {
		t.Fatalf("expected display name in output: %s", out)
	}
}

func TestCloudIdentityGroupsGetCmd_JSON(t *testing.T) {
	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/groups:lookup":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/123"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/groups/123":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "groups/123",
				"groupKey":    map[string]any{"id": "group@example.com"},
				"displayName": "Example Group",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CloudIdentityGroupsGetCmd{Group: "group@example.com"}

	u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["name"] != "groups/123" {
		t.Fatalf("unexpected name in JSON: %v", result["name"])
	}
}

func TestCloudIdentityGroupsCreateCmd(t *testing.T) {
	var gotEmail string
	var gotDisplayName string

	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/v1/groups") {
			http.NotFound(w, r)
			return
		}
		var payload cloudidentity.Group
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.GroupKey != nil {
			gotEmail = payload.GroupKey.Id
		}
		gotDisplayName = payload.DisplayName
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/op1"})
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CloudIdentityGroupsCreateCmd{Email: "newgroup@example.com", DisplayName: "New Group"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotEmail != "newgroup@example.com" {
		t.Fatalf("unexpected email: %s", gotEmail)
	}
	if gotDisplayName != "New Group" {
		t.Fatalf("unexpected display name: %s", gotDisplayName)
	}
	if !strings.Contains(out, "Created group") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCloudIdentityGroupsCreateCmd_WithDynamicQuery(t *testing.T) {
	var gotDynamicQuery string

	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/v1/groups") {
			http.NotFound(w, r)
			return
		}
		var payload cloudidentity.Group
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.DynamicGroupMetadata != nil && len(payload.DynamicGroupMetadata.Queries) > 0 {
			gotDynamicQuery = payload.DynamicGroupMetadata.Queries[0].Query
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/op1"})
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CloudIdentityGroupsCreateCmd{
		Email:        "dynamic@example.com",
		DynamicQuery: "user.is_enrolled_in_2sv == true",
	}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotDynamicQuery != "user.is_enrolled_in_2sv == true" {
		t.Fatalf("unexpected dynamic query: %s", gotDynamicQuery)
	}
}

func TestCloudIdentityGroupsUpdateCmd(t *testing.T) {
	var gotDisplayName string

	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/groups:lookup":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/123"})
			return
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/groups/123":
			var payload cloudidentity.Group
			_ = json.NewDecoder(r.Body).Decode(&payload)
			gotDisplayName = payload.DisplayName
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/op1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CloudIdentityGroupsUpdateCmd{Group: "group@example.com", DisplayName: "Updated Name"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotDisplayName != "Updated Name" {
		t.Fatalf("unexpected display name: %s", gotDisplayName)
	}
	if !strings.Contains(out, "Updated group") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCloudIdentityGroupsDeleteCmd(t *testing.T) {
	var deleteCalled bool

	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/groups:lookup":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/123"})
			return
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/groups/123":
			deleteCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/op1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &CloudIdentityGroupsDeleteCmd{Group: "group@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleteCalled {
		t.Fatal("delete was not called")
	}
	if !strings.Contains(out, "Deleted group") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCloudIdentityMembersListCmd(t *testing.T) {
	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/groups:lookup":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/123"})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/groups/123/memberships"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{{
					"name":               "groups/123/memberships/456",
					"preferredMemberKey": map[string]any{"id": "member@example.com"},
					"roles":              []map[string]any{{"name": "MEMBER"}, {"name": "MANAGER"}},
				}},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CloudIdentityMembersListCmd{Group: "group@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "member@example.com") {
		t.Fatalf("expected member email in output: %s", out)
	}
	if !strings.Contains(out, "MANAGER") {
		t.Fatalf("expected MANAGER role in output: %s", out)
	}
}

func TestCloudIdentityMembersRemoveCmd(t *testing.T) {
	var deleteCalled bool

	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/groups:lookup":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/123"})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/groups/123/memberships:lookup":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/123/memberships/456"})
			return
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/groups/123/memberships/456":
			deleteCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/op1"})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &CloudIdentityMembersRemoveCmd{Group: "group@example.com", Email: "member@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleteCalled {
		t.Fatal("delete was not called")
	}
	if !strings.Contains(out, "Removed") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCloudIdentityGroupsListCmd_JSON(t *testing.T) {
	svc, closeSrv := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"groups": []map[string]any{{
				"name":        "groups/123",
				"groupKey":    map[string]any{"id": "group@example.com"},
				"displayName": "Example Group",
			}},
			"nextPageToken": "npt123",
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudIdentityAdminService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CloudIdentityGroupsListCmd{}

	u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["nextPageToken"] != "npt123" {
		t.Fatalf("unexpected nextPageToken in JSON: %v", result["nextPageToken"])
	}
}

func TestMembershipRoleNames(t *testing.T) {
	tests := []struct {
		name  string
		roles []*cloudidentity.MembershipRole
		want  []string
	}{
		{"nil", nil, nil},
		{"empty", []*cloudidentity.MembershipRole{}, nil},
		{"nil role", []*cloudidentity.MembershipRole{nil}, nil},
		{"single", []*cloudidentity.MembershipRole{{Name: "MEMBER"}}, []string{"MEMBER"}},
		{"multiple sorted", []*cloudidentity.MembershipRole{{Name: "OWNER"}, {Name: "MEMBER"}}, []string{"MEMBER", "OWNER"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := membershipRoleNames(tt.roles)
			if len(got) != len(tt.want) {
				t.Fatalf("membershipRoleNames() = %v, want %v", got, tt.want)
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Fatalf("membershipRoleNames() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
