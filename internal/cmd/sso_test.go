package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func testContextJSONSSO(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
}

// -----------------------------------------------------------------------------
// SSOSettingsGetCmd Tests
// -----------------------------------------------------------------------------

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
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "https://sso.example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSSOSettingsGetCmd_JSON(t *testing.T) {
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
					},
				},
			},
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsGetCmd{}

	ctx := testContextJSONSSO(t)

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["name"] != "inboundSamlSsoProfiles/profile-1" {
		t.Fatalf("unexpected name in JSON: %v", result["name"])
	}
}

func TestSSOSettingsGetCmd_AllFields(t *testing.T) {
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
					"displayName": "My SSO",
					"idpConfig": map[string]any{
						"entityId":               "https://idp.example.com",
						"singleSignOnServiceUri": "https://sso.example.com",
						"logoutRedirectUri":      "https://logout.example.com",
						"changePasswordUri":      "https://password.example.com",
					},
					"spConfig": map[string]any{
						"entityId":                    "https://sp.example.com",
						"assertionConsumerServiceUri": "https://acs.example.com",
					},
				},
			},
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsGetCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	expected := []string{
		"Profile:",
		"Display Name:",
		"Entity ID:",
		"SSO URL:",
		"Logout URL:",
		"Change Password:",
		"SP Entity ID:",
		"SP ACS URL:",
	}
	for _, exp := range expected {
		if !strings.Contains(out, exp) {
			t.Fatalf("expected %q in output: %s", exp, out)
		}
	}
}

func TestSSOSettingsGetCmd_NoProfiles(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"inboundSamlSsoProfiles": []map[string]any{},
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsGetCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for no profiles")
	}
	if !strings.Contains(err.Error(), "no inbound SAML SSO profiles found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSSOSettingsGetCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &SSOSettingsGetCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestSSOSettingsGetCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    403,
				"message": "access denied",
			},
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsGetCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// -----------------------------------------------------------------------------
// SSOSettingsUpdateCmd Tests
// -----------------------------------------------------------------------------

func TestSSOSettingsUpdateCmd_SSOURL(t *testing.T) {
	var gotURL string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/inboundSamlSsoProfiles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"inboundSamlSsoProfiles": []map[string]any{{
					"name": "inboundSamlSsoProfiles/profile-1",
				}},
			})
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/inboundSamlSsoProfiles/profile-1"):
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if cfg, ok := payload["idpConfig"].(map[string]any); ok {
				gotURL, _ = cfg["singleSignOnServiceUri"].(string)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-update-1",
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsUpdateCmd{SSOURL: "https://new-sso.example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotURL != "https://new-sso.example.com" {
		t.Fatalf("expected sso url to be set, got: %s", gotURL)
	}
	if !strings.Contains(out, "Updated inbound SSO profile") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSSOSettingsUpdateCmd_MultipleURLs(t *testing.T) {
	var gotSSOURL, gotLogoutURL, gotPwdURL string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/inboundSamlSsoProfiles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"inboundSamlSsoProfiles": []map[string]any{{
					"name": "inboundSamlSsoProfiles/profile-1",
				}},
			})
		case r.Method == http.MethodPatch:
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if cfg, ok := payload["idpConfig"].(map[string]any); ok {
				gotSSOURL, _ = cfg["singleSignOnServiceUri"].(string)
				gotLogoutURL, _ = cfg["logoutRedirectUri"].(string)
				gotPwdURL, _ = cfg["changePasswordUri"].(string)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-update-1",
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsUpdateCmd{
		SSOURL:            "https://sso.example.com",
		LogoutURL:         "https://logout.example.com",
		ChangePasswordURL: "https://password.example.com",
	}

	if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if gotSSOURL != "https://sso.example.com" {
		t.Fatalf("expected sso url, got: %s", gotSSOURL)
	}
	if gotLogoutURL != "https://logout.example.com" {
		t.Fatalf("expected logout url, got: %s", gotLogoutURL)
	}
	if gotPwdURL != "https://password.example.com" {
		t.Fatalf("expected password url, got: %s", gotPwdURL)
	}
}

func TestSSOSettingsUpdateCmd_Certificate(t *testing.T) {
	var gotPemData string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/inboundSamlSsoProfiles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"inboundSamlSsoProfiles": []map[string]any{{
					"name": "inboundSamlSsoProfiles/profile-1",
				}},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/idpCredentials:add"):
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			gotPemData, _ = payload["pemData"].(string)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-cert-1",
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsUpdateCmd{Certificate: "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPemData == "" {
		t.Fatal("expected certificate to be set")
	}
	if !strings.Contains(out, "Updated inbound SSO profile") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSSOSettingsUpdateCmd_CertificateFromFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "cert.pem")
	certContent := "-----BEGIN CERTIFICATE-----\nfilecontent\n-----END CERTIFICATE-----"
	if err := os.WriteFile(tmpFile, []byte(certContent), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var gotPemData string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/inboundSamlSsoProfiles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"inboundSamlSsoProfiles": []map[string]any{{
					"name": "inboundSamlSsoProfiles/profile-1",
				}},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/idpCredentials:add"):
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			gotPemData, _ = payload["pemData"].(string)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-cert-1",
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsUpdateCmd{Certificate: "@" + tmpFile}

	if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if gotPemData != certContent {
		t.Fatalf("expected certificate from file, got: %s", gotPemData)
	}
}

func TestSSOSettingsUpdateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/inboundSamlSsoProfiles"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"inboundSamlSsoProfiles": []map[string]any{{
					"name": "inboundSamlSsoProfiles/profile-1",
				}},
			})
		case r.Method == http.MethodPatch:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-update-1",
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsUpdateCmd{SSOURL: "https://sso.example.com"}

	ctx := testContextJSONSSO(t)

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["profile"] != "inboundSamlSsoProfiles/profile-1" {
		t.Fatalf("unexpected profile in JSON: %v", result["profile"])
	}
}

func TestSSOSettingsUpdateCmd_NoUpdates(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsUpdateCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for no updates")
	}
	if !strings.Contains(err.Error(), "no updates specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSSOSettingsUpdateCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &SSOSettingsUpdateCmd{SSOURL: "https://sso.example.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestSSOSettingsUpdateCmd_EmptyCertificate(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "empty.pem")
	if err := os.WriteFile(tmpFile, []byte("   "), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/inboundSamlSsoProfiles") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"inboundSamlSsoProfiles": []map[string]any{{
					"name": "inboundSamlSsoProfiles/profile-1",
				}},
			})
			return
		}
		http.NotFound(w, r)
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOSettingsUpdateCmd{Certificate: "@" + tmpFile}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty certificate")
	}
	if !strings.Contains(err.Error(), "certificate is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// SSOAssignmentsListCmd Tests
// -----------------------------------------------------------------------------

func TestSSOAssignmentsListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/inboundSsoAssignments") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"inboundSsoAssignments": []map[string]any{
				{
					"name":          "inboundSsoAssignments/assignment-1",
					"ssoMode":       "SSO_OFF",
					"targetOrgUnit": "orgUnits/ou-123",
				},
			},
			"nextPageToken": "token-123",
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsListCmd{}

	ctx := testContextJSONSSO(t)

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result struct {
		InboundSsoAssignments []struct {
			Name          string `json:"name"`
			SsoMode       string `json:"ssoMode"`
			TargetOrgUnit string `json:"targetOrgUnit"`
		} `json:"inboundSsoAssignments"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result.NextPageToken != "token-123" {
		t.Fatalf("unexpected nextPageToken: %v", result.NextPageToken)
	}
	if len(result.InboundSsoAssignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(result.InboundSsoAssignments))
	}
	if result.InboundSsoAssignments[0].Name != "inboundSsoAssignments/assignment-1" {
		t.Fatalf("unexpected assignment name: %s", result.InboundSsoAssignments[0].Name)
	}
	if result.InboundSsoAssignments[0].SsoMode != "SSO_OFF" {
		t.Fatalf("unexpected assignment mode: %s", result.InboundSsoAssignments[0].SsoMode)
	}
}

func TestSSOAssignmentsListCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"inboundSsoAssignments": []map[string]any{},
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsListCmd{}

	// Should not error on empty list
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestSSOAssignmentsListCmd_WithTargetGroup(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"inboundSsoAssignments": []map[string]any{
				{
					"name":        "inboundSsoAssignments/assignment-2",
					"ssoMode":     "DOMAIN_WIDE_SAML_IF_ENABLED",
					"targetGroup": "groups/group-123",
				},
			},
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "groups/group-123") {
		t.Fatalf("expected group target in output: %s", out)
	}
}

func TestSSOAssignmentsListCmd_WithPagination(t *testing.T) {
	var gotPageSize, gotPageToken string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPageSize = r.URL.Query().Get("pageSize")
		gotPageToken = r.URL.Query().Get("pageToken")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"inboundSsoAssignments": []map[string]any{},
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsListCmd{Max: 50, Page: "next-page-token"}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if gotPageSize != "50" {
		t.Fatalf("expected pageSize 50, got: %s", gotPageSize)
	}
	if gotPageToken != "next-page-token" {
		t.Fatalf("expected pageToken, got: %s", gotPageToken)
	}
}

func TestSSOAssignmentsListCmd_NilAssignment(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// API might return null entries
		_ = json.NewEncoder(w).Encode(map[string]any{
			"inboundSsoAssignments": []any{
				nil,
				map[string]any{
					"name":          "inboundSsoAssignments/assignment-1",
					"ssoMode":       "SSO_OFF",
					"targetOrgUnit": "orgUnits/ou-123",
				},
			},
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "assignment-1") {
		t.Fatalf("expected assignment in output: %s", out)
	}
}

func TestSSOAssignmentsListCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &SSOAssignmentsListCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestSSOAssignmentsListCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    500,
				"message": "internal error",
			},
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsListCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// -----------------------------------------------------------------------------
// SSOAssignmentsCreateCmd Tests
// -----------------------------------------------------------------------------

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

func TestSSOAssignmentsCreateCmd_SSOOff(t *testing.T) {
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/orgunits/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"orgUnitId":   "ou-456",
				"orgUnitPath": "/Engineering",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, adminHandler)

	var gotMode string
	cloudHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/inboundSsoAssignments") {
			var payload struct {
				SsoMode string `json:"ssoMode"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			gotMode = payload.SsoMode
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-2",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubInboundSSO(t, cloudHandler)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsCreateCmd{OrgUnit: "/Engineering", Mode: "SSO_OFF"}

	if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if gotMode != "SSO_OFF" {
		t.Fatalf("expected mode SSO_OFF, got %q", gotMode)
	}
}

func TestSSOAssignmentsCreateCmd_DirectOrgUnitID(t *testing.T) {
	// When orgUnit already starts with "orgUnits/", no lookup needed
	var gotTarget string
	cloudHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/inboundSsoAssignments") {
			var payload struct {
				TargetOrgUnit string `json:"targetOrgUnit"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			gotTarget = payload.TargetOrgUnit
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-3",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubInboundSSO(t, cloudHandler)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsCreateCmd{OrgUnit: "orgUnits/direct-ou", Mode: "SSO_OFF"}

	if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if gotTarget != "orgUnits/direct-ou" {
		t.Fatalf("expected direct org unit, got %q", gotTarget)
	}
}

func TestSSOAssignmentsCreateCmd_ModeNone_ClearAssignments(t *testing.T) {
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/orgunits/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"orgUnitId":   "ou-789",
				"orgUnitPath": "/Marketing",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, adminHandler)

	var deleteCalled bool
	cloudHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/inboundSsoAssignments"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"inboundSsoAssignments": []map[string]any{
					{
						"name":          "inboundSsoAssignments/assign-to-delete",
						"targetOrgUnit": "orgUnits/ou-789",
					},
				},
			})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "assign-to-delete"):
			deleteCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-delete",
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubInboundSSO(t, cloudHandler)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsCreateCmd{OrgUnit: "/Marketing", Mode: "NONE"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleteCalled {
		t.Fatal("expected delete to be called for NONE mode")
	}
	if !strings.Contains(out, "Deleted 1 inbound SSO assignments") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSSOAssignmentsCreateCmd_ModeNone_NoAssignments(t *testing.T) {
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/orgunits/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"orgUnitId":   "ou-empty",
				"orgUnitPath": "/Empty",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, adminHandler)

	cloudHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/inboundSsoAssignments") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"inboundSsoAssignments": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
	})
	stubInboundSSO(t, cloudHandler)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsCreateCmd{OrgUnit: "/Empty", Mode: "NONE"}

	// Should not error when no assignments to delete
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestSSOAssignmentsCreateCmd_JSON(t *testing.T) {
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/orgunits/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"orgUnitId": "ou-json",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, adminHandler)

	cloudHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/inboundSsoAssignments") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-json",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubInboundSSO(t, cloudHandler)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsCreateCmd{OrgUnit: "/Test", Mode: "SSO_OFF"}

	ctx := testContextJSONSSO(t)

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["name"] != "operations/op-json" {
		t.Fatalf("unexpected operation name: %v", result["name"])
	}
}

func TestSSOAssignmentsCreateCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &SSOAssignmentsCreateCmd{OrgUnit: "/Test", Mode: "SSO_ON"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestSSOAssignmentsCreateCmd_EmptyOrgUnit(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsCreateCmd{OrgUnit: "  ", Mode: "SSO_ON"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty org unit")
	}
	if !strings.Contains(err.Error(), "--org-unit is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// SSOAssignmentsDeleteCmd Tests
// -----------------------------------------------------------------------------

func TestSSOAssignmentsDeleteCmd_JSON(t *testing.T) {
	var deleteCalled bool
	var deletedID string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/v1/inboundSsoAssignments/") {
			deleteCalled = true
			deletedID = strings.TrimPrefix(r.URL.Path, "/v1/inboundSsoAssignments/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-delete-json",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &SSOAssignmentsDeleteCmd{AssignmentID: "inboundSsoAssignments/assignment-json"}

	ctx := testContextJSONSSO(t)

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if !deleteCalled {
		t.Fatal("delete was not called")
	}
	if deletedID != "assignment-json" {
		t.Fatalf("unexpected deleted ID: %s", deletedID)
	}
	if result["name"] != "operations/op-delete-json" {
		t.Fatalf("unexpected operation name: %v", result["name"])
	}
}

func TestSSOAssignmentsDeleteCmd_RequiresConfirmation(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com", NoInput: true}
	cmd := &SSOAssignmentsDeleteCmd{AssignmentID: "inboundSsoAssignments/assignment-1"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing confirmation")
	}
}

func TestSSOAssignmentsDeleteCmd_EmptyID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &SSOAssignmentsDeleteCmd{AssignmentID: "  "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty assignment ID")
	}
	if !strings.Contains(err.Error(), "assignment ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSSOAssignmentsDeleteCmd_MissingAccount(t *testing.T) {
	flags := &RootFlags{}
	cmd := &SSOAssignmentsDeleteCmd{AssignmentID: "inboundSsoAssignments/assignment-1"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestSSOAssignmentsDeleteCmd_APIError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    404,
				"message": "assignment not found",
			},
		})
	})
	stubInboundSSO(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &SSOAssignmentsDeleteCmd{AssignmentID: "inboundSsoAssignments/nonexistent"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// -----------------------------------------------------------------------------
// Helper Function Tests
// -----------------------------------------------------------------------------

func TestMapInboundSSOMode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"SSO_OFF", "SSO_OFF", false},
		{"sso_off", "SSO_OFF", false},
		{"  SSO_OFF  ", "SSO_OFF", false},
		{"SSO_ON", "DOMAIN_WIDE_SAML_IF_ENABLED", false},
		{"sso_on", "DOMAIN_WIDE_SAML_IF_ENABLED", false},
		{"INVALID", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := mapInboundSSOMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("mapInboundSSOMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.expected {
				t.Fatalf("mapInboundSSOMode(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestReadValueOrFile(t *testing.T) {
	// Test empty value
	result, err := readValueOrFile("")
	if err != nil || result != "" {
		t.Fatalf("expected empty result for empty input, got: %q, err: %v", result, err)
	}

	// Test whitespace-only value
	result, err = readValueOrFile("   ")
	if err != nil || result != "" {
		t.Fatalf("expected empty result for whitespace input, got: %q, err: %v", result, err)
	}

	// Test direct value
	result, err = readValueOrFile("direct-value")
	if err != nil || result != "direct-value" {
		t.Fatalf("expected direct value, got: %q, err: %v", result, err)
	}

	// Test JSON object
	result, err = readValueOrFile(`{"key": "value"}`)
	if err != nil || result != `{"key": "value"}` {
		t.Fatalf("expected JSON object, got: %q, err: %v", result, err)
	}

	// Test JSON array
	result, err = readValueOrFile(`["a", "b"]`)
	if err != nil || result != `["a", "b"]` {
		t.Fatalf("expected JSON array, got: %q, err: %v", result, err)
	}

	// Test @file syntax
	tmpFile := filepath.Join(t.TempDir(), "testfile.txt")
	if err := os.WriteFile(tmpFile, []byte("file-content"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err = readValueOrFile("@" + tmpFile)
	if err != nil || result != "file-content" {
		t.Fatalf("expected file content, got: %q, err: %v", result, err)
	}

	// Test @file with empty path
	result, err = readValueOrFile("@")
	if err == nil {
		t.Fatal("expected error for empty @file path")
	}
	if !strings.Contains(err.Error(), "empty @file path") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test @file with spaces around path
	result, err = readValueOrFile("@  " + tmpFile + "  ")
	if err != nil || result != "file-content" {
		t.Fatalf("expected file content with trimmed path, got: %q, err: %v", result, err)
	}

	// Test file path detection (file exists)
	tmpFile2 := filepath.Join(t.TempDir(), "detectfile.txt")
	if err := os.WriteFile(tmpFile2, []byte("detected-content"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err = readValueOrFile(tmpFile2)
	if err != nil || result != "detected-content" {
		t.Fatalf("expected detected file content, got: %q, err: %v", result, err)
	}

	// Test nonexistent file in @syntax
	result, err = readValueOrFile("@/nonexistent/file.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent @file")
	}
}

func TestResolveOrgUnitResource_DirectID(t *testing.T) {
	flags := &RootFlags{Account: "admin@example.com"}

	result, err := resolveOrgUnitResource(context.Background(), flags, "orgUnits/direct-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "orgUnits/direct-123" {
		t.Fatalf("expected direct ID passthrough, got: %s", result)
	}
}

func TestResolveOrgUnitResource_Lookup(t *testing.T) {
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/orgunits/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"orgUnitId":   "resolved-ou-id",
				"orgUnitPath": "/Resolved",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, adminHandler)

	flags := &RootFlags{Account: "admin@example.com"}

	result, err := resolveOrgUnitResource(testContext(t), flags, "/Resolved")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "orgUnits/resolved-ou-id" {
		t.Fatalf("expected resolved org unit ID, got: %s", result)
	}
}

func TestResolveOrgUnitResource_EmptyOrgUnitID(t *testing.T) {
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/orgunits/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"orgUnitId":   "",
				"orgUnitPath": "/NoID",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, adminHandler)

	flags := &RootFlags{Account: "admin@example.com"}

	_, err := resolveOrgUnitResource(testContext(t), flags, "/NoID")
	if err == nil {
		t.Fatal("expected error for empty org unit ID")
	}
	if !strings.Contains(err.Error(), "has no ID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClearInboundSSOAssignments_JSON(t *testing.T) {
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/orgunits/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"orgUnitId": "ou-json-clear",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, adminHandler)

	cloudHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/inboundSsoAssignments"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"inboundSsoAssignments": []map[string]any{
					{
						"name":          "inboundSsoAssignments/assign-1",
						"targetOrgUnit": "orgUnits/ou-json-clear",
					},
				},
			})
		case r.Method == http.MethodDelete:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "operations/op-del",
			})
		default:
			http.NotFound(w, r)
		}
	})
	stubInboundSSO(t, cloudHandler)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &SSOAssignmentsCreateCmd{OrgUnit: "/Test", Mode: "NONE"}

	ctx := testContextJSONSSO(t)

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if result["targetOrgUnit"] != "orgUnits/ou-json-clear" {
		t.Fatalf("unexpected targetOrgUnit: %v", result["targetOrgUnit"])
	}
	deleted, ok := result["deleted"].([]any)
	if !ok || len(deleted) != 1 {
		t.Fatalf("expected 1 deleted assignment, got: %v", result["deleted"])
	}
}

// -----------------------------------------------------------------------------
// Test Helper Functions
// -----------------------------------------------------------------------------

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
