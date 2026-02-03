package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
)

func newIAMServiceStub(t *testing.T, handler http.HandlerFunc) (*iam.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(handler)
	svc, err := iam.NewService(context.Background(),
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

func stubIAMService(t *testing.T, svc *iam.Service) {
	t.Helper()
	orig := newIAMService
	t.Cleanup(func() { newIAMService = orig })
	newIAMService = func(context.Context, string) (*iam.Service, error) { return svc, nil }
}

// ============================================================================
// ServiceAccountsListCmd Tests
// ============================================================================

func TestServiceAccountsListCmd_Success(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/serviceAccounts") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": []map[string]any{
				{
					"email":       "sa1@test-project.iam.gserviceaccount.com",
					"displayName": "Service Account One",
					"name":        "projects/test-project/serviceAccounts/sa1@test-project.iam.gserviceaccount.com",
				},
				{
					"email":       "sa2@test-project.iam.gserviceaccount.com",
					"displayName": "Service Account Two",
					"name":        "projects/test-project/serviceAccounts/sa2@test-project.iam.gserviceaccount.com",
				},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsListCmd{Project: "test-project", Max: 100}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "sa1@test-project.iam.gserviceaccount.com") {
		t.Fatalf("expected email in output, got: %s", out)
	}
	if !strings.Contains(out, "Service Account One") {
		t.Fatalf("expected display name in output, got: %s", out)
	}
}

func TestServiceAccountsListCmd_JSON(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/serviceAccounts") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": []map[string]any{
				{
					"email":       "sa1@test-project.iam.gserviceaccount.com",
					"displayName": "Service Account One",
				},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsListCmd{Project: "test-project"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "accounts") {
		t.Fatalf("expected JSON accounts output, got: %s", out)
	}
}

func TestServiceAccountsListCmd_EmptyResults(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": []map[string]any{},
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsListCmd{Project: "test-project"}

	// Should not error even with no accounts
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestServiceAccountsListCmd_EmptyProject(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsListCmd{Project: "   "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty project")
	}
	if !strings.Contains(err.Error(), "--project is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsListCmd_NoAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &ServiceAccountsListCmd{Project: "test-project"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestServiceAccountsListCmd_APIError(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "permission denied", http.StatusForbidden)
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsListCmd{Project: "test-project"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error from API")
	}
	if !strings.Contains(err.Error(), "list service accounts") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsListCmd_Pagination(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageToken := r.URL.Query().Get("pageToken")
		w.Header().Set("Content-Type", "application/json")
		if pageToken == "page2" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accounts": []map[string]any{
					{"email": "sa2@test-project.iam.gserviceaccount.com"},
				},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accounts": []map[string]any{
					{"email": "sa1@test-project.iam.gserviceaccount.com"},
				},
				"nextPageToken": "page2",
			})
		}
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsListCmd{Project: "test-project", Page: "page2"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "sa2@test-project.iam.gserviceaccount.com") {
		t.Fatalf("expected page 2 results, got: %s", out)
	}
}

// ============================================================================
// ServiceAccountsCreateCmd Tests
// ============================================================================

func TestServiceAccountsCreateCmd_Success(t *testing.T) {
	var gotAccountID, gotDisplayName string
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/serviceAccounts") {
			http.NotFound(w, r)
			return
		}
		var payload iam.CreateServiceAccountRequest
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotAccountID = payload.AccountId
		if payload.ServiceAccount != nil {
			gotDisplayName = payload.ServiceAccount.DisplayName
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email":       gotAccountID + "@test-project.iam.gserviceaccount.com",
			"displayName": gotDisplayName,
			"name":        "projects/test-project/serviceAccounts/" + gotAccountID + "@test-project.iam.gserviceaccount.com",
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsCreateCmd{
		Project:     "test-project",
		Name:        "my-service-account",
		DisplayName: "My Service Account",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotAccountID != "my-service-account" {
		t.Fatalf("expected accountId 'my-service-account', got: %s", gotAccountID)
	}
	if gotDisplayName != "My Service Account" {
		t.Fatalf("expected displayName 'My Service Account', got: %s", gotDisplayName)
	}
	if !strings.Contains(out, "Created service account") {
		t.Fatalf("expected success message, got: %s", out)
	}
}

func TestServiceAccountsCreateCmd_DefaultDisplayName(t *testing.T) {
	var gotDisplayName string
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload iam.CreateServiceAccountRequest
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.ServiceAccount != nil {
			gotDisplayName = payload.ServiceAccount.DisplayName
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email": "my-sa@test-project.iam.gserviceaccount.com",
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsCreateCmd{
		Project: "test-project",
		Name:    "my-sa",
		// No DisplayName - should default to Name
	}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotDisplayName != "my-sa" {
		t.Fatalf("expected displayName to default to accountId 'my-sa', got: %s", gotDisplayName)
	}
}

func TestServiceAccountsCreateCmd_JSON(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email":       "my-sa@test-project.iam.gserviceaccount.com",
			"displayName": "My SA",
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsCreateCmd{
		Project: "test-project",
		Name:    "my-sa",
	}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "email") {
		t.Fatalf("expected JSON output with email, got: %s", out)
	}
}

func TestServiceAccountsCreateCmd_EmptyProject(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsCreateCmd{Project: "", Name: "my-sa"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty project")
	}
	if !strings.Contains(err.Error(), "--project and --name are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsCreateCmd_EmptyName(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsCreateCmd{Project: "test-project", Name: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "--project and --name are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsCreateCmd_InvalidName(t *testing.T) {
	tests := []struct {
		name        string
		accountName string
	}{
		{"uppercase", "MyServiceAccount"},
		{"starts with digit", "1my-service-account"},
		{"starts with hyphen", "-my-service-account"},
		{"contains underscore", "my_service_account"},
		{"contains special chars", "my-sa@test"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flags := &RootFlags{Account: "admin@example.com"}
			cmd := &ServiceAccountsCreateCmd{Project: "test-project", Name: tc.accountName}

			err := cmd.Run(testContext(t), flags)
			if err == nil {
				t.Fatalf("expected error for invalid name: %s", tc.accountName)
			}
			if !strings.Contains(err.Error(), "valid service account ID") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestServiceAccountsCreateCmd_ValidNames(t *testing.T) {
	tests := []struct {
		name        string
		accountName string
	}{
		{"simple", "my-sa"},
		{"with digits", "my-sa-123"},
		{"all lowercase", "myserviceaccount"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"email": tc.accountName + "@test-project.iam.gserviceaccount.com",
				})
			}))
			t.Cleanup(closeSrv)
			stubIAMService(t, svc)

			flags := &RootFlags{Account: "admin@example.com"}
			cmd := &ServiceAccountsCreateCmd{Project: "test-project", Name: tc.accountName}

			if err := cmd.Run(testContext(t), flags); err != nil {
				t.Fatalf("expected no error for valid name %s, got: %v", tc.accountName, err)
			}
		})
	}
}

func TestServiceAccountsCreateCmd_APIError(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "already exists", http.StatusConflict)
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsCreateCmd{
		Project: "test-project",
		Name:    "my-sa",
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error from API")
	}
	if !strings.Contains(err.Error(), "create service account") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsCreateCmd_NoAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &ServiceAccountsCreateCmd{Project: "test-project", Name: "my-sa"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

// ============================================================================
// ServiceAccountsDeleteCmd Tests
// ============================================================================

func TestServiceAccountsDeleteCmd_Success(t *testing.T) {
	var deletedName string
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/serviceAccounts/") {
			http.NotFound(w, r)
			return
		}
		deletedName = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ServiceAccountsDeleteCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(deletedName, "my-sa@test-project.iam.gserviceaccount.com") {
		t.Fatalf("expected delete to include email, got: %s", deletedName)
	}
	if !strings.Contains(out, "Deleted service account") {
		t.Fatalf("expected success message, got: %s", out)
	}
}

func TestServiceAccountsDeleteCmd_FullResourceName(t *testing.T) {
	var deletedPath string
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		deletedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ServiceAccountsDeleteCmd{
		ServiceAccount: "projects/test-project/serviceAccounts/my-sa@test-project.iam.gserviceaccount.com",
	}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// When already a full resource name, it should use it as-is
	if !strings.Contains(deletedPath, "projects/test-project/serviceAccounts/my-sa") {
		t.Fatalf("expected full resource name in path, got: %s", deletedPath)
	}
}

func TestServiceAccountsDeleteCmd_JSON(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ServiceAccountsDeleteCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
	}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"deleted":true`) && !strings.Contains(out, `"deleted": true`) {
		t.Fatalf("expected JSON output with deleted:true, got: %s", out)
	}
}

func TestServiceAccountsDeleteCmd_EmptyServiceAccount(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ServiceAccountsDeleteCmd{ServiceAccount: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty service account")
	}
	if !strings.Contains(err.Error(), "service account is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsDeleteCmd_NoForce(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", NoInput: true}
	cmd := &ServiceAccountsDeleteCmd{ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error without force")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got: %v", err)
	}
}

func TestServiceAccountsDeleteCmd_APIError(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ServiceAccountsDeleteCmd{
		ServiceAccount: "nonexistent@test-project.iam.gserviceaccount.com",
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error from API")
	}
	if !strings.Contains(err.Error(), "delete service account") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsDeleteCmd_NoAccount(t *testing.T) {
	flags := &RootFlags{Force: true}
	cmd := &ServiceAccountsDeleteCmd{ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

// ============================================================================
// ServiceAccountsKeysListCmd Tests
// ============================================================================

func TestServiceAccountsKeysListCmd_Success(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/keys") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"name":            "projects/test-project/serviceAccounts/my-sa@test-project.iam.gserviceaccount.com/keys/key1",
					"keyType":         "USER_MANAGED",
					"validAfterTime":  "2024-01-01T00:00:00Z",
					"validBeforeTime": "2025-01-01T00:00:00Z",
				},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsKeysListCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "USER_MANAGED") {
		t.Fatalf("expected key type in output, got: %s", out)
	}
}

func TestServiceAccountsKeysListCmd_JSON(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{"name": "key1", "keyType": "USER_MANAGED"},
			},
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsKeysListCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
	}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "keys") {
		t.Fatalf("expected JSON keys output, got: %s", out)
	}
}

func TestServiceAccountsKeysListCmd_EmptyResults(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{},
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsKeysListCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
	}

	// Should not error even with no keys
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestServiceAccountsKeysListCmd_EmptyServiceAccount(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsKeysListCmd{ServiceAccount: ""}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty service account")
	}
	if !strings.Contains(err.Error(), "service account is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsKeysListCmd_APIError(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "permission denied", http.StatusForbidden)
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsKeysListCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error from API")
	}
	if !strings.Contains(err.Error(), "list keys") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsKeysListCmd_NoAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &ServiceAccountsKeysListCmd{ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

// ============================================================================
// ServiceAccountsKeysCreateCmd Tests
// ============================================================================

func TestServiceAccountsKeysCreateCmd_Success(t *testing.T) {
	keyData := `{"type":"service_account","project_id":"test-project"}`
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/keys") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":           "projects/test-project/serviceAccounts/my-sa@test-project.iam.gserviceaccount.com/keys/newkey",
			"privateKeyData": base64.StdEncoding.EncodeToString([]byte(keyData)),
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "subdir", "key.json")

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsKeysCreateCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		Output:         outputPath,
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Created key") {
		t.Fatalf("expected success message, got: %s", out)
	}

	// Verify key file was written
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read key file: %v", err)
	}
	if string(data) != keyData {
		t.Fatalf("expected key data %q, got %q", keyData, string(data))
	}

	// Verify file permissions (0600)
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestServiceAccountsKeysCreateCmd_JSON(t *testing.T) {
	keyData := `{"type":"service_account"}`
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":           "projects/test-project/serviceAccounts/my-sa@test-project.iam.gserviceaccount.com/keys/newkey",
			"privateKeyData": base64.StdEncoding.EncodeToString([]byte(keyData)),
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "key.json")

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsKeysCreateCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		Output:         outputPath,
	}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "key") || !strings.Contains(out, "output") {
		t.Fatalf("expected JSON output with key and output, got: %s", out)
	}
}

func TestServiceAccountsKeysCreateCmd_EmptyServiceAccount(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsKeysCreateCmd{
		ServiceAccount: "",
		Output:         "/tmp/key.json",
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty service account")
	}
	if !strings.Contains(err.Error(), "service account is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsKeysCreateCmd_EmptyOutput(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsKeysCreateCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		Output:         "  ",
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty output")
	}
	if !strings.Contains(err.Error(), "--output is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsKeysCreateCmd_APIError(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "quota exceeded", http.StatusTooManyRequests)
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "key.json")

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsKeysCreateCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		Output:         outputPath,
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error from API")
	}
	if !strings.Contains(err.Error(), "create key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsKeysCreateCmd_NoAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &ServiceAccountsKeysCreateCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		Output:         "/tmp/key.json",
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

// ============================================================================
// ServiceAccountsKeysDeleteCmd Tests
// ============================================================================

func TestServiceAccountsKeysDeleteCmd_Success(t *testing.T) {
	var deletedPath string
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/keys/") {
			http.NotFound(w, r)
			return
		}
		deletedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ServiceAccountsKeysDeleteCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		KeyID:          "key123",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(deletedPath, "key123") {
		t.Fatalf("expected key id in deleted path, got: %s", deletedPath)
	}
	if !strings.Contains(out, "Deleted key") {
		t.Fatalf("expected success message, got: %s", out)
	}
}

func TestServiceAccountsKeysDeleteCmd_FullResourceName(t *testing.T) {
	var deletedPath string
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		deletedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ServiceAccountsKeysDeleteCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		KeyID:          "projects/test-project/serviceAccounts/my-sa@test-project.iam.gserviceaccount.com/keys/key456",
	}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(deletedPath, "key456") {
		t.Fatalf("expected full key path, got: %s", deletedPath)
	}
}

func TestServiceAccountsKeysDeleteCmd_JSON(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ServiceAccountsKeysDeleteCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		KeyID:          "key123",
	}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, `"deleted":true`) && !strings.Contains(out, `"deleted": true`) {
		t.Fatalf("expected JSON output with deleted:true, got: %s", out)
	}
}

func TestServiceAccountsKeysDeleteCmd_EmptyServiceAccount(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ServiceAccountsKeysDeleteCmd{
		ServiceAccount: "",
		KeyID:          "key123",
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty service account")
	}
	if !strings.Contains(err.Error(), "service account and key are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsKeysDeleteCmd_EmptyKey(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ServiceAccountsKeysDeleteCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		KeyID:          "",
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "service account and key are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsKeysDeleteCmd_NoForce(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", NoInput: true}
	cmd := &ServiceAccountsKeysDeleteCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		KeyID:          "key123",
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error without force")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got: %v", err)
	}
}

func TestServiceAccountsKeysDeleteCmd_APIError(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &ServiceAccountsKeysDeleteCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		KeyID:          "nonexistent",
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error from API")
	}
	if !strings.Contains(err.Error(), "delete key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceAccountsKeysDeleteCmd_NoAccount(t *testing.T) {
	flags := &RootFlags{Force: true}
	cmd := &ServiceAccountsKeysDeleteCmd{
		ServiceAccount: "my-sa@test-project.iam.gserviceaccount.com",
		KeyID:          "key123",
	}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

// ============================================================================
// Helper function tests
// ============================================================================

func TestNormalizeServiceAccountName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "my-sa@project.iam.gserviceaccount.com",
			expected: "projects/-/serviceAccounts/my-sa@project.iam.gserviceaccount.com",
		},
		{
			input:    "projects/test-project/serviceAccounts/my-sa@test-project.iam.gserviceaccount.com",
			expected: "projects/test-project/serviceAccounts/my-sa@test-project.iam.gserviceaccount.com",
		},
		{
			input:    "  my-sa@project.iam.gserviceaccount.com  ",
			expected: "projects/-/serviceAccounts/my-sa@project.iam.gserviceaccount.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizeServiceAccountName(tc.input)
			if result != tc.expected {
				t.Fatalf("normalizeServiceAccountName(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestNormalizeServiceAccountKeyName(t *testing.T) {
	tests := []struct {
		sa       string
		key      string
		expected string
	}{
		{
			sa:       "my-sa@project.iam.gserviceaccount.com",
			key:      "key123",
			expected: "projects/-/serviceAccounts/my-sa@project.iam.gserviceaccount.com/keys/key123",
		},
		{
			sa:       "my-sa@project.iam.gserviceaccount.com",
			key:      "projects/test/serviceAccounts/sa/keys/key456",
			expected: "projects/test/serviceAccounts/sa/keys/key456",
		},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			result := normalizeServiceAccountKeyName(tc.sa, tc.key)
			if result != tc.expected {
				t.Fatalf("normalizeServiceAccountKeyName(%q, %q) = %q, want %q", tc.sa, tc.key, result, tc.expected)
			}
		})
	}
}

func TestIsValidServiceAccountID(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"my-service-account", true},
		{"mysa", true},
		{"my-sa-123", true},
		{"a", true},
		{"a1", true},
		{"MyServiceAccount", false}, // uppercase
		{"1my-sa", false},           // starts with digit
		{"-my-sa", false},           // starts with hyphen
		{"my_sa", false},            // underscore
		{"my.sa", false},            // dot
		{"my@sa", false},            // @ symbol
		{"", false},                 // empty
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			result := isValidServiceAccountID(tc.id)
			if result != tc.valid {
				t.Fatalf("isValidServiceAccountID(%q) = %v, want %v", tc.id, result, tc.valid)
			}
		})
	}
}
