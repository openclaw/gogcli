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
