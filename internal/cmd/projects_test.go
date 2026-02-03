package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/cloudresourcemanager/v3"
	"google.golang.org/api/option"
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
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Project One") {
		t.Fatalf("unexpected output: %s", out)
	}
}

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
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
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
