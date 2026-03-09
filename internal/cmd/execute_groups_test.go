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

func TestExecute_GroupsList_JSON(t *testing.T) {
	origNew := newCloudIdentityService
	t.Cleanup(func() { newCloudIdentityService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "groups/-/memberships:searchTransitiveGroups") && r.Method == http.MethodGet {
			query := r.URL.Query().Get("query")
			if !strings.Contains(query, "'"+groupLabelDiscussionForum+"' in labels") {
				t.Fatalf("missing discussion label filter in query: %q", query)
			}
			if !strings.Contains(query, "member_key_id == 'a@b.com'") {
				t.Fatalf("missing member_key_id filter in query: %q", query)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{
					{
						"groupKey":     map[string]any{"id": "engineering@example.com"},
						"displayName":  "Engineering",
						"relationType": "DIRECT",
					},
					{
						"groupKey":     map[string]any{"id": "all@example.com"},
						"displayName":  "All Employees",
						"relationType": "INDIRECT",
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := cloudidentity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCloudIdentityService = func(context.Context, string) (*cloudidentity.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "groups", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Groups []struct {
			GroupName   string `json:"groupName"`
			DisplayName string `json:"displayName"`
			Role        string `json:"role"`
		} `json:"groups"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(parsed.Groups))
	}
	if parsed.Groups[0].GroupName != "engineering@example.com" {
		t.Fatalf("unexpected group name: %s", parsed.Groups[0].GroupName)
	}
	if parsed.Groups[0].DisplayName != "Engineering" {
		t.Fatalf("unexpected display name: %s", parsed.Groups[0].DisplayName)
	}
}

func TestExecute_GroupsMembers_JSON(t *testing.T) {
	origNew := newCloudIdentityService
	t.Cleanup(func() { newCloudIdentityService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "groups:lookup"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "groups/abc123",
			})
			return
		case strings.Contains(r.URL.Path, "groups/abc123/memberships") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{
					{
						"preferredMemberKey": map[string]any{"id": "alice@example.com"},
						"roles":              []map[string]any{{"name": "OWNER"}},
						"type":               "USER",
					},
					{
						"preferredMemberKey": map[string]any{"id": "bob@example.com"},
						"roles":              []map[string]any{{"name": "MEMBER"}},
						"type":               "USER",
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := cloudidentity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCloudIdentityService = func(context.Context, string) (*cloudidentity.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "groups", "members", "engineering@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Members []struct {
			Email string `json:"email"`
			Role  string `json:"role"`
			Type  string `json:"type"`
		} `json:"members"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(parsed.Members))
	}
	if parsed.Members[0].Email != "alice@example.com" {
		t.Fatalf("unexpected email: %s", parsed.Members[0].Email)
	}
	if parsed.Members[0].Role != "OWNER" {
		t.Fatalf("unexpected role: %s", parsed.Members[0].Role)
	}
}

func TestExecute_GroupsList_Text(t *testing.T) {
	origNew := newCloudIdentityService
	t.Cleanup(func() { newCloudIdentityService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "groups/-/memberships:searchTransitiveGroups") && r.Method == http.MethodGet {
			query := r.URL.Query().Get("query")
			if !strings.Contains(query, "'"+groupLabelDiscussionForum+"' in labels") {
				t.Fatalf("missing discussion label filter in query: %q", query)
			}
			if !strings.Contains(query, "member_key_id == 'a@b.com'") {
				t.Fatalf("missing member_key_id filter in query: %q", query)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{
					{
						"groupKey":     map[string]any{"id": "engineering@example.com"},
						"displayName":  "Engineering",
						"relationType": "DIRECT",
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := cloudidentity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newCloudIdentityService = func(context.Context, string) (*cloudidentity.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "groups", "list"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "GROUP") || !strings.Contains(out, "NAME") || !strings.Contains(out, "RELATION") {
		t.Fatalf("missing headers in output: %q", out)
	}
	if !strings.Contains(out, "engineering@example.com") || !strings.Contains(out, "Engineering") {
		t.Fatalf("missing group data in output: %q", out)
	}
}
