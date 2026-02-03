package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/option"
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
