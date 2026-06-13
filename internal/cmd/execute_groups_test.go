package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_GroupsList_JSON(t *testing.T) {
	svc := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithCloudIdentityTestService(t, []string{"--json", "--account", "a@b.com", "groups", "list"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		Groups []struct {
			GroupName   string `json:"groupName"`
			DisplayName string `json:"displayName"`
			Role        string `json:"role"`
		} `json:"groups"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
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
	svc := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithCloudIdentityTestService(t, []string{
		"--json", "--account", "a@b.com", "groups", "members", "engineering@example.com",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		Members []struct {
			Email string `json:"email"`
			Role  string `json:"role"`
			Type  string `json:"type"`
		} `json:"members"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
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

func TestExecute_GroupsMembers_PermissionErrors(t *testing.T) {
	tests := []struct {
		name      string
		failStage string
	}{
		{name: "lookup", failStage: "lookup"},
		{name: "list", failStage: "list"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "groups:lookup"):
					if tc.failStage == "lookup" {
						writeGroupsPermissionError(t, w)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"name": "groups/abc123"})
				case strings.Contains(r.URL.Path, "groups/abc123/memberships"):
					writeGroupsPermissionError(t, w)
				default:
					http.NotFound(w, r)
				}
			}))

			result := executeWithCloudIdentityTestService(t, []string{
				"--account", "admin@example.com", "groups", "members", "engineering@example.com",
			}, svc)
			if result.err == nil || ExitCode(result.err) != exitCodePermissionDenied {
				t.Fatalf("unexpected error: %v\nstderr=%q", result.err, result.stderr)
			}
			for _, want := range []string{
				"Insufficient permissions for Cloud Identity API",
				groupReadonlyScope,
				"gog auth service-account set admin@example.com",
			} {
				if !strings.Contains(result.stderr, want) {
					t.Fatalf("stderr = %q, want %q", result.stderr, want)
				}
			}
			if strings.Contains(result.stderr, "gog auth add") {
				t.Fatalf("stderr suggests unsupported OAuth recovery: %q", result.stderr)
			}
		})
	}
}

func writeGroupsPermissionError(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    http.StatusForbidden,
			"message": "Request had insufficient authentication scopes.",
			"errors": []map[string]any{{
				"reason": "insufficientPermissions",
			}},
		},
	}); err != nil {
		t.Fatalf("encode error response: %v", err)
	}
}

func TestExecute_GroupsList_Text(t *testing.T) {
	svc := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	result := executeWithCloudIdentityTestService(t, []string{"--account", "a@b.com", "groups", "list"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if !strings.Contains(result.stdout, "GROUP") || !strings.Contains(result.stdout, "NAME") || !strings.Contains(result.stdout, "RELATION") {
		t.Fatalf("missing headers in output: %q", result.stdout)
	}
	if !strings.Contains(result.stdout, "engineering@example.com") || !strings.Contains(result.stdout, "Engineering") {
		t.Fatalf("missing group data in output: %q", result.stdout)
	}
}
