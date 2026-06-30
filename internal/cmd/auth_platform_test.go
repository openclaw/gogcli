package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/outfmt"
)

func TestNormalizeTesterEmail(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "lowercases", in: "Admin@Example.COM", want: "admin@example.com"},
		{name: "trim", in: " admin@example.com ", want: "admin@example.com"},
		{name: "display rejected", in: "Admin <admin@example.com>", wantErr: true},
		{name: "not email", in: "not-email", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeTesterEmail(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestAuthPlatformTestersAddIdempotent(t *testing.T) {
	state := []string{"existing@example.com"}
	server := authPlatformTestServer(t, &state)
	t.Setenv("GOG_AUTH_PLATFORM_BASE_URL", server.URL)

	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	ctx = authclient.WithAccessToken(ctx, "token")
	cmd := &AuthPlatformTestersAddCmd{
		authPlatformProjectFlags: authPlatformProjectFlags{Project: "arc-forge-console", ProjectNumber: "35664692003"},
		Email:                    "existing@example.com",
	}
	if err := cmd.Run(ctx, &RootFlags{AccessToken: "token", Force: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(state) != 1 || state[0] != "existing@example.com" {
		t.Fatalf("state changed: %#v", state)
	}
}

func TestAuthPlatformTestersAddWritesAndVerifies(t *testing.T) {
	state := []string{"existing@example.com"}
	server := authPlatformTestServer(t, &state)
	t.Setenv("GOG_AUTH_PLATFORM_BASE_URL", server.URL)

	var out strings.Builder
	ctx := newCmdRuntimeOutputContext(t, &out, io.Discard)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	ctx = authclient.WithAccessToken(ctx, "token")
	cmd := &AuthPlatformTestersAddCmd{
		authPlatformProjectFlags: authPlatformProjectFlags{Project: "arc-forge-console", ProjectNumber: "35664692003"},
		Email:                    "admin@horizonprodental.com.au",
	}
	if err := cmd.Run(ctx, &RootFlags{AccessToken: "token", Force: true, JSON: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !containsEmailFold(state, "admin@horizonprodental.com.au") || !containsEmailFold(state, "existing@example.com") {
		t.Fatalf("state = %#v", state)
	}
	var parsed authPlatformTesterResult
	if err := json.Unmarshal([]byte(out.String()), &parsed); err != nil {
		t.Fatalf("json output: %v\n%s", err, out.String())
	}
	if !parsed.Changed || parsed.Email != "admin@horizonprodental.com.au" {
		t.Fatalf("unexpected output: %#v", parsed)
	}
}

func TestAuthPlatformTestersRemovePreservesOthers(t *testing.T) {
	state := []string{"existing@example.com", "remove@example.com"}
	server := authPlatformTestServer(t, &state)
	t.Setenv("GOG_AUTH_PLATFORM_BASE_URL", server.URL)

	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	ctx = authclient.WithAccessToken(ctx, "token")
	cmd := &AuthPlatformTestersRemoveCmd{
		authPlatformProjectFlags: authPlatformProjectFlags{Project: "arc-forge-console", ProjectNumber: "35664692003"},
		Email:                    "remove@example.com",
	}
	if err := cmd.Run(ctx, &RootFlags{AccessToken: "token", Force: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if containsEmailFold(state, "remove@example.com") || !containsEmailFold(state, "existing@example.com") {
		t.Fatalf("state = %#v", state)
	}
}

func TestAuthPlatformTestersAddDryRunDoesNotCallServer(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("GOG_AUTH_PLATFORM_BASE_URL", server.URL)

	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	cmd := &AuthPlatformTestersAddCmd{
		authPlatformProjectFlags: authPlatformProjectFlags{Project: "arc-forge-console", ProjectNumber: "35664692003"},
		Email:                    "admin@example.com",
	}
	err := cmd.Run(ctx, &RootFlags{AccessToken: "token", DryRun: true})
	if err == nil || ExitCode(err) != 0 {
		t.Fatalf("expected dry-run exit, got %v", err)
	}
	if called {
		t.Fatalf("server was called during dry run")
	}
}

func authPlatformTestServer(t *testing.T, state *[]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		var req struct {
			OperationName string `json:"operationName"`
			Variables     struct {
				TrustedUserList []string `json:"trustedUserList"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.OperationName {
		case authPlatformReadOp:
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"results": []map[string]any{{
					"data": map[string]any{"getTrustedUserList": map[string]any{"userAccount": *state}},
				}},
			}})
		case authPlatformWriteOp:
			*state = append([]string(nil), req.Variables.TrustedUserList...)
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"results": []map[string]any{{
					"data": map[string]any{"setTrustedUserList": map[string]any{"trustedUserList": map[string]any{"userAccount": *state}}},
				}},
			}})
		default:
			t.Fatalf("operation = %s", req.OperationName)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
