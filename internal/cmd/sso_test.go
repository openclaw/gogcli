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

func TestSSOSettingsGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/inboundSamlSsoProfiles") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"inboundSamlSsoProfiles": []map[string]any{
				{
					"name":        "inboundSamlSsoProfiles/profile-1",
					"displayName": "Workspace",
					"idpConfig": map[string]any{
						"entityId":               "https://idp.example.com",
						"singleSignOnServiceUri": "https://sso.example.com",
						"logoutRedirectUri":      "https://logout.example.com",
					},
				},
			},
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsGetCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "https://sso.example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSSOAssignmentsCreateCmd(t *testing.T) {
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orgunits/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"orgUnitId":   "ou-123",
			"orgUnitPath": "/Sales",
		})
	})
	stubAdminDirectory(t, adminHandler)

	var gotTarget, gotMode string
	cloudHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/inboundSsoAssignments"):
			var payload struct {
				TargetOrgUnit string `json:"targetOrgUnit"`
				SsoMode       string `json:"ssoMode"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			gotTarget = payload.TargetOrgUnit
			gotMode = payload.SsoMode
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-1",
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubInboundSSO(t, cloudHandler)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsCreateCmd{OrgUnit: "/Sales", Mode: "SSO_ON"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotTarget != "orgUnits/ou-123" {
		t.Fatalf("expected target orgUnits/ou-123, got %q", gotTarget)
	}
	if gotMode != "DOMAIN_WIDE_SAML_IF_ENABLED" {
		t.Fatalf("expected mode DOMAIN_WIDE_SAML_IF_ENABLED, got %q", gotMode)
	}
	if !strings.Contains(out, "Created inbound SSO assignment") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubInboundSSO(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newInboundSSOService
	svc, err := cloudidentity.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new cloudidentity service: %v", err)
	}
	newInboundSSOService = func(context.Context, string) (*cloudidentity.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newInboundSSOService = orig
		srv.Close()
	})
	return srv
}
