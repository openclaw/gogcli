package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/storage/v1"
	"google.golang.org/api/vault/v1"
)

func TestVaultMattersListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/matters") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matters": []map[string]any{
				{"matterId": "matter-1", "name": "Case One", "state": "OPEN"},
			},
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &VaultMattersListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Case One") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultExportsDownloadCmd(t *testing.T) {
	vaultHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/exports/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "export-1",
			"name": "Export One",
			"cloudStorageSink": map[string]any{
				"files": []map[string]any{
					{"bucketName": "vault-bucket", "objectName": "exports/export1.zip"},
				},
			},
		})
	})
	stubVault(t, vaultHandler)

	storageHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/b/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("vault-export"))
	})
	stubStorage(t, storageHandler)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &VaultExportsDownloadCmd{MatterID: "matter-1", ExportID: "export-1", Output: t.TempDir()}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	path := filepath.Join(cmd.Output, "export1.zip")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if string(data) != "vault-export" {
		t.Fatalf("unexpected file contents: %s", string(data))
	}
	if !strings.Contains(out, "Downloaded") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubVault(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newVaultService
	svc, err := vault.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new vault service: %v", err)
	}
	newVaultService = func(context.Context, string) (*vault.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newVaultService = orig
		srv.Close()
	})
	return srv
}

func stubStorage(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newStorageService
	svc, err := storage.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new storage service: %v", err)
	}
	newStorageService = func(context.Context, string) (*storage.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newStorageService = orig
		srv.Close()
	})
	return srv
}

func TestVaultMattersGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/matters/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matterId":    "matter-1",
			"name":        "Test Matter",
			"state":       "OPEN",
			"description": "Test description",
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &VaultMattersGetCmd{MatterID: "matter-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Test Matter") || !strings.Contains(out, "OPEN") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultMattersCreateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/matters") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matterId": "new-matter",
			"name":     "New Matter",
			"state":    "OPEN",
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &VaultMattersCreateCmd{Name: "New Matter", Description: "Test"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Created matter") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultMattersUpdateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/matters/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matterId": "matter-1",
			"name":     "Updated Matter",
			"state":    "OPEN",
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	newName := "Updated Matter"
	cmd := &VaultMattersUpdateCmd{MatterID: "matter-1", Name: &newName}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Updated matter") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultMattersCloseCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, ":close") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matter": map[string]any{
				"matterId": "matter-1",
				"name":     "Closed Matter",
				"state":    "CLOSED",
			},
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &VaultMattersCloseCmd{MatterID: "matter-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Closed matter") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultMattersReopenCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, ":reopen") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matter": map[string]any{
				"matterId": "matter-1",
				"name":     "Reopened Matter",
				"state":    "OPEN",
			},
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &VaultMattersReopenCmd{MatterID: "matter-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Reopened matter") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultMattersDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/matters/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matterId": "matter-1",
			"state":    "DELETED",
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &VaultMattersDeleteCmd{MatterID: "matter-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted matter") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultExportsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/exports") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"exports": []map[string]any{
				{
					"id":         "export-1",
					"name":       "Test Export",
					"status":     "COMPLETED",
					"createTime": "2024-01-01T00:00:00Z",
				},
			},
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &VaultExportsListCmd{MatterID: "matter-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Test Export") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultExportsGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/exports/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         "export-1",
			"name":       "Export Details",
			"status":     "COMPLETED",
			"createTime": "2024-01-01T00:00:00Z",
			"query": map[string]any{
				"corpus": "MAIL",
			},
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &VaultExportsGetCmd{MatterID: "matter-1", ExportID: "export-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Export Details") || !strings.Contains(out, "COMPLETED") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultExportsCreateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/exports") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "new-export",
			"name":   "New Export",
			"status": "IN_PROGRESS",
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &VaultExportsCreateCmd{MatterID: "matter-1", Name: "New Export", Query: "test"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Created export") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultHoldsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/holds") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"holds": []map[string]any{
				{
					"holdId": "hold-1",
					"name":   "Legal Hold",
					"corpus": "MAIL",
					"accounts": []map[string]any{
						{"email": "user@example.com"},
					},
				},
			},
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &VaultHoldsListCmd{MatterID: "matter-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Legal Hold") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultHoldsGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/holds/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"holdId": "hold-1",
			"name":   "Legal Hold Details",
			"corpus": "MAIL",
			"orgUnit": map[string]any{
				"orgUnitId": "ou-123",
			},
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &VaultHoldsGetCmd{MatterID: "matter-1", HoldID: "hold-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Legal Hold Details") || !strings.Contains(out, "org-unit") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultHoldsCreateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/holds") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"holdId": "new-hold",
			"name":   "New Legal Hold",
			"corpus": "MAIL",
		})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &VaultHoldsCreateCmd{MatterID: "matter-1", Name: "New Legal Hold", Corpus: "MAIL", Accounts: "user@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Created hold") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestVaultHoldsDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/holds/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	stubVault(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &VaultHoldsDeleteCmd{MatterID: "matter-1", HoldID: "hold-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted hold") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestHoldScope(t *testing.T) {
	tests := []struct {
		name     string
		hold     *vault.Hold
		expected string
	}{
		{"nil hold", nil, ""},
		{"empty hold", &vault.Hold{}, ""},
		{"with org unit", &vault.Hold{OrgUnit: &vault.HeldOrgUnit{OrgUnitId: "ou-1"}}, "org-unit"},
		{"with accounts", &vault.Hold{Accounts: []*vault.HeldAccount{{Email: "a@b.c"}, {Email: "d@e.f"}}}, "accounts:2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := holdScope(tt.hold)
			if result != tt.expected {
				t.Errorf("holdScope() = %q, want %q", result, tt.expected)
			}
		})
	}
}
