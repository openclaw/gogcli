package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
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

func TestServiceAccountsKeysCreateCmd(t *testing.T) {
	keyPayload := []byte("{}")
	encoded := base64.StdEncoding.EncodeToString(keyPayload)

	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v1/projects/-/serviceAccounts/sa@example.com/keys") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":           "projects/-/serviceAccounts/sa@example.com/keys/key1",
			"privateKeyData": encoded,
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	output := filepath.Join(t.TempDir(), "sa.json")
	cmd := &ServiceAccountsKeysCreateCmd{ServiceAccount: "sa@example.com", Output: output}

	_ = captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(keyPayload) {
		t.Fatalf("unexpected key data: %s", string(data))
	}
}

func TestServiceAccountsListCmd(t *testing.T) {
	svc, closeSrv := newIAMServiceStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/projects/p1/serviceAccounts") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": []map[string]any{{
				"name":        "projects/p1/serviceAccounts/sa@example.com",
				"email":       "sa@example.com",
				"displayName": "SA",
			}},
		})
	}))
	t.Cleanup(closeSrv)
	stubIAMService(t, svc)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ServiceAccountsListCmd{Project: "p1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "sa@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}
