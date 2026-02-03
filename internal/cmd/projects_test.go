package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/cloudresourcemanager/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func newCloudResourceServiceStub(t *testing.T, handler http.HandlerFunc) (*cloudresourcemanager.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(handler)
	svc, err := cloudresourcemanager.NewService(context.Background(),
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

func stubCloudResourceService(t *testing.T, svc *cloudresourcemanager.Service) {
	t.Helper()
	orig := newCloudResourceService
	t.Cleanup(func() { newCloudResourceService = orig })
	newCloudResourceService = func(context.Context, string) (*cloudresourcemanager.Service, error) { return svc, nil }
}

func stubCloudResourceServiceError(t *testing.T, err error) {
	t.Helper()
	orig := newCloudResourceService
	t.Cleanup(func() { newCloudResourceService = orig })
	newCloudResourceService = func(context.Context, string) (*cloudresourcemanager.Service, error) { return nil, err }
}

func projectsTestContext(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func projectsTestContextWithStdout(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func projectsTestContextJSON(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	return outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
}

// -----------------------------------------------------------------------------
// normalizeProjectName helper tests
// -----------------------------------------------------------------------------

func TestNormalizeProjectName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bare project ID",
			input: "my-project",
			want:  "projects/my-project",
		},
		{
			name:  "already prefixed",
			input: "projects/my-project",
			want:  "projects/my-project",
		},
		{
			name:  "empty string",
			input: "",
			want:  "projects/",
		},
		{
			name:  "project ID with numbers",
			input: "test-project-123",
			want:  "projects/test-project-123",
		},
		{
			name:  "project ID with underscores",
			input: "my_project_name",
			want:  "projects/my_project_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeProjectName(tt.input)
			if got != tt.want {
				t.Errorf("normalizeProjectName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// ProjectsListCmd tests
// -----------------------------------------------------------------------------

func TestProjectsListCmd(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v3/projects") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("parent") == "" {
			http.Error(w, "missing parent", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"projects": []map[string]any{{
				"projectId":   "p1",
				"displayName": "Project One",
				"state":       "ACTIVE",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsListCmd{Parent: "organizations/123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Project One") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestProjectsListCmd_JSON(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v3/projects") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"projects": []map[string]any{{
				"projectId":   "p1",
				"displayName": "Project One",
				"state":       "ACTIVE",
			}},
			"nextPageToken": "npt123",
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsListCmd{Parent: "organizations/123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextJSON(t), flags); err != nil {
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

func TestProjectsListCmd_EmptyParent(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsListCmd{Parent: ""}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty parent")
	}
	if !strings.Contains(err.Error(), "--parent is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsListCmd_WhitespaceParent(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsListCmd{Parent: "   "}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for whitespace-only parent")
	}
	if !strings.Contains(err.Error(), "--parent is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsListCmd_NoProjects(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"projects": []map[string]any{},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsListCmd{Parent: "organizations/123"}

	// No projects should not return an error, just a message to stderr
	err := cmd.Run(projectsTestContext(t), flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsListCmd_WithPagination(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") != "page2" {
			http.Error(w, "expected page token", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"projects": []map[string]any{{
				"projectId":   "p2",
				"displayName": "Project Two",
				"state":       "ACTIVE",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsListCmd{Parent: "organizations/123", Page: "page2"}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Project Two") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestProjectsListCmd_WithShowDeleted(t *testing.T) {
	var gotShowDeleted bool
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotShowDeleted = r.URL.Query().Get("showDeleted") == "true"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"projects": []map[string]any{{
				"projectId":   "p1",
				"displayName": "Project One",
				"state":       "DELETE_REQUESTED",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsListCmd{Parent: "organizations/123", ShowDeleted: true}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !gotShowDeleted {
		t.Fatal("showDeleted was not passed to API")
	}
	if !strings.Contains(out, "DELETE_REQUESTED") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestProjectsListCmd_APIError(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "permission denied", http.StatusForbidden)
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsListCmd{Parent: "organizations/123"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	if !strings.Contains(err.Error(), "list projects") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsListCmd_ServiceError(t *testing.T) {
	stubCloudResourceServiceError(t, errors.New("service unavailable"))

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsListCmd{Parent: "organizations/123"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for service creation failure")
	}
	if !strings.Contains(err.Error(), "service unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsListCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{Account: ""}
	cmd := &ProjectsListCmd{Parent: "organizations/123"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestProjectsListCmd_FolderParent(t *testing.T) {
	var gotParent string
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotParent = r.URL.Query().Get("parent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"projects": []map[string]any{{
				"projectId":   "p1",
				"displayName": "Project One",
				"state":       "ACTIVE",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsListCmd{Parent: "folders/456"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotParent != "folders/456" {
		t.Fatalf("unexpected parent: %s", gotParent)
	}
}

// -----------------------------------------------------------------------------
// ProjectsGetCmd tests
// -----------------------------------------------------------------------------

func TestProjectsGetCmd(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v3/projects/my-project" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "projects/my-project",
			"projectId":   "my-project",
			"displayName": "My Project",
			"state":       "ACTIVE",
			"parent":      "organizations/123",
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsGetCmd{Project: "my-project"}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "my-project") {
		t.Fatalf("expected project ID in output: %s", out)
	}
	if !strings.Contains(out, "My Project") {
		t.Fatalf("expected display name in output: %s", out)
	}
	if !strings.Contains(out, "ACTIVE") {
		t.Fatalf("expected state in output: %s", out)
	}
	if !strings.Contains(out, "organizations/123") {
		t.Fatalf("expected parent in output: %s", out)
	}
}

func TestProjectsGetCmd_WithPrefix(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/projects/my-project" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"projectId":   "my-project",
			"displayName": "My Project",
			"state":       "ACTIVE",
			"parent":      "organizations/123",
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsGetCmd{Project: "projects/my-project"}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "my-project") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestProjectsGetCmd_JSON(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "projects/my-project",
			"projectId":   "my-project",
			"displayName": "My Project",
			"state":       "ACTIVE",
			"parent":      "organizations/123",
		})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsGetCmd{Project: "my-project"}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextJSON(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["projectId"] != "my-project" {
		t.Fatalf("unexpected projectId in JSON: %v", result["projectId"])
	}
	if result["state"] != "ACTIVE" {
		t.Fatalf("unexpected state in JSON: %v", result["state"])
	}
}

func TestProjectsGetCmd_EmptyProject(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsGetCmd{Project: ""}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty project")
	}
	if !strings.Contains(err.Error(), "project is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsGetCmd_WhitespaceProject(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsGetCmd{Project: "   "}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for whitespace-only project")
	}
	if !strings.Contains(err.Error(), "project is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsGetCmd_NotFound(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "project not found", http.StatusNotFound)
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsGetCmd{Project: "nonexistent"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for not found project")
	}
	if !strings.Contains(err.Error(), "get project") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsGetCmd_ServiceError(t *testing.T) {
	stubCloudResourceServiceError(t, errors.New("service unavailable"))

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsGetCmd{Project: "my-project"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for service creation failure")
	}
	if !strings.Contains(err.Error(), "service unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsGetCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{Account: ""}
	cmd := &ProjectsGetCmd{Project: "my-project"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

// -----------------------------------------------------------------------------
// ProjectsCreateCmd tests
// -----------------------------------------------------------------------------

func TestProjectsCreateCmd(t *testing.T) {
	var gotID string
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/v3/projects") {
			http.NotFound(w, r)
			return
		}
		var payload cloudresourcemanager.Project
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotID = payload.ProjectId
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/op1"})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsCreateCmd{ID: "p1", Name: "Project One", Parent: "organizations/123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotID != "p1" {
		t.Fatalf("unexpected project id: %s", gotID)
	}
	if !strings.Contains(out, "Requested creation") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestProjectsCreateCmd_JSON(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/op1", "done": false})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsCreateCmd{ID: "p1", Name: "Project One", Parent: "organizations/123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextJSON(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["name"] != "operations/op1" {
		t.Fatalf("unexpected operation name in JSON: %v", result["name"])
	}
}

func TestProjectsCreateCmd_Payload(t *testing.T) {
	var gotPayload cloudresourcemanager.Project
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/op1"})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsCreateCmd{ID: "test-project-id", Name: "Test Project Name", Parent: "folders/789"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPayload.ProjectId != "test-project-id" {
		t.Fatalf("unexpected project ID in payload: %s", gotPayload.ProjectId)
	}
	if gotPayload.DisplayName != "Test Project Name" {
		t.Fatalf("unexpected display name in payload: %s", gotPayload.DisplayName)
	}
	if gotPayload.Parent != "folders/789" {
		t.Fatalf("unexpected parent in payload: %s", gotPayload.Parent)
	}
}

func TestProjectsCreateCmd_MissingID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsCreateCmd{ID: "", Name: "Project One", Parent: "organizations/123"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing ID")
	}
	if !strings.Contains(err.Error(), "--id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsCreateCmd_MissingName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsCreateCmd{ID: "p1", Name: "", Parent: "organizations/123"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsCreateCmd_MissingParent(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsCreateCmd{ID: "p1", Name: "Project One", Parent: ""}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing parent")
	}
	if !strings.Contains(err.Error(), "--parent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsCreateCmd_APIError(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "quota exceeded", http.StatusTooManyRequests)
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsCreateCmd{ID: "p1", Name: "Project One", Parent: "organizations/123"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	if !strings.Contains(err.Error(), "create project") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsCreateCmd_ServiceError(t *testing.T) {
	stubCloudResourceServiceError(t, errors.New("service unavailable"))

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ProjectsCreateCmd{ID: "p1", Name: "Project One", Parent: "organizations/123"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for service creation failure")
	}
}

// -----------------------------------------------------------------------------
// ProjectsDeleteCmd tests
// -----------------------------------------------------------------------------

func TestProjectsDeleteCmd(t *testing.T) {
	var deleteCalled bool
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v3/projects/my-project" {
			http.NotFound(w, r)
			return
		}
		deleteCalled = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/delete-op"})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ProjectsDeleteCmd{Project: "my-project"}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleteCalled {
		t.Fatal("delete was not called")
	}
	if !strings.Contains(out, "Requested deletion") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestProjectsDeleteCmd_WithPrefix(t *testing.T) {
	var gotPath string
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/delete-op"})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ProjectsDeleteCmd{Project: "projects/my-project"}

	_ = captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPath != "/v3/projects/my-project" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestProjectsDeleteCmd_JSON(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/delete-op", "done": false})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ProjectsDeleteCmd{Project: "my-project"}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextJSON(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["name"] != "operations/delete-op" {
		t.Fatalf("unexpected operation name in JSON: %v", result["name"])
	}
}

func TestProjectsDeleteCmd_EmptyProject(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ProjectsDeleteCmd{Project: ""}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty project")
	}
	if !strings.Contains(err.Error(), "project is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsDeleteCmd_APIError(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "permission denied", http.StatusForbidden)
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ProjectsDeleteCmd{Project: "my-project"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	if !strings.Contains(err.Error(), "delete project") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectsDeleteCmd_ServiceError(t *testing.T) {
	stubCloudResourceServiceError(t, errors.New("service unavailable"))

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ProjectsDeleteCmd{Project: "my-project"}

	err := cmd.Run(projectsTestContext(t), flags)
	if err == nil {
		t.Fatal("expected error for service creation failure")
	}
}

func TestProjectsDeleteCmd_WithOperationName(t *testing.T) {
	svc, closeSrv := newCloudResourceServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "operations/long-running-delete"})
	}))
	t.Cleanup(closeSrv)
	stubCloudResourceService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ProjectsDeleteCmd{Project: "my-project"}

	out := captureStdout(t, func() {
		if err := cmd.Run(projectsTestContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Operation:") {
		t.Fatalf("expected operation name in output: %s", out)
	}
	if !strings.Contains(out, "operations/long-running-delete") {
		t.Fatalf("expected full operation name in output: %s", out)
	}
}
