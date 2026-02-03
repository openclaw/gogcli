package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/accesscontextmanager/v1"
	"google.golang.org/api/option"
)

func TestCAALevelsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/accessPolicies/123/accessLevels") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessLevels": []map[string]any{
				{
					"name":        "accessPolicies/123/accessLevels/level1",
					"title":       "level1",
					"description": "Corp",
					"basic": map[string]any{
						"conditions": []map[string]any{{"ipSubnetworks": []string{"10.0.0.0/24"}}},
					},
				},
			},
		})
	})
	stubAccessContextManager(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CAALevelsListCmd{Policy: "123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "level1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCAALevelsCreateCmd(t *testing.T) {
	var gotName string
	var gotSubnet string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/accessPolicies/123/accessLevels") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Name  string `json:"name"`
			Basic struct {
				Conditions []struct {
					IpSubnetworks []string `json:"ipSubnetworks"`
				} `json:"conditions"`
			} `json:"basic"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotName = payload.Name
		if len(payload.Basic.Conditions) > 0 && len(payload.Basic.Conditions[0].IpSubnetworks) > 0 {
			gotSubnet = payload.Basic.Conditions[0].IpSubnetworks[0]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "operations/op-1",
		})
	})
	stubAccessContextManager(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CAALevelsCreateCmd{
		Name:       "level1",
		Policy:     "123",
		Basic:      true,
		Conditions: []string{"{\"ipSubnetworks\":[\"10.0.0.0/24\"]}"},
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotName != "accessPolicies/123/accessLevels/level1" {
		t.Fatalf("unexpected name: %q", gotName)
	}
	if gotSubnet != "10.0.0.0/24" {
		t.Fatalf("unexpected subnet: %q", gotSubnet)
	}
	if !strings.Contains(out, "Created access level") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubAccessContextManager(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newAccessContextManagerService
	svc, err := accesscontextmanager.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new access context manager service: %v", err)
	}
	newAccessContextManagerService = func(context.Context, string) (*accesscontextmanager.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newAccessContextManagerService = orig
		srv.Close()
	})
	return srv
}
