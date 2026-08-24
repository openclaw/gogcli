package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	scriptapi "google.golang.org/api/script/v1"
)

func TestAppScriptDeploymentVersion(t *testing.T) {
	cases := []struct {
		name       string
		deployment *scriptapi.Deployment
		want       string
	}{
		{name: "nil deployment", deployment: nil, want: "HEAD"},
		{name: "no config", deployment: &scriptapi.Deployment{}, want: "HEAD"},
		{
			name:       "unversioned config tracks the editor",
			deployment: &scriptapi.Deployment{DeploymentConfig: &scriptapi.DeploymentConfig{}},
			want:       "HEAD",
		},
		{
			name:       "versioned",
			deployment: &scriptapi.Deployment{DeploymentConfig: &scriptapi.DeploymentConfig{VersionNumber: 7}},
			want:       "v7",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := appScriptDeploymentVersion(tc.deployment); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAppScriptWebAppURL(t *testing.T) {
	if got := appScriptWebAppURL(nil); got != "" {
		t.Fatalf("nil deployment: got %q", got)
	}

	deployment := &scriptapi.Deployment{
		EntryPoints: []*scriptapi.EntryPoint{
			nil,
			{EntryPointType: "EXECUTION_API"},
			{WebApp: &scriptapi.GoogleAppsScriptTypeWebAppEntryPoint{Url: "https://example.test/exec"}},
		},
	}
	if got := appScriptWebAppURL(deployment); got != "https://example.test/exec" {
		t.Fatalf("got %q", got)
	}
}

func TestExecute_AppScriptDeployments_AllFetchesEveryPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/deployments") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Query().Get("pageToken") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployments":   []map[string]any{{"deploymentId": "one"}},
				"nextPageToken": "page2",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"deployments": []map[string]any{{"deploymentId": "two"}},
		})
	}))
	defer srv.Close()

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "deployments", "script123", "--all",
	}, newAppScriptTestService(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stdout, "DEPLOYMENT_ID") ||
		!strings.Contains(result.stdout, "one") ||
		!strings.Contains(result.stdout, "two") {
		t.Fatalf("expected both pages in out=%q", result.stdout)
	}
}

func TestExecute_AppScriptVersions_JSONKeepsNextPageToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/versions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"versions":      []map[string]any{{"versionNumber": 1, "description": "first"}},
			"nextPageToken": "page2",
		})
	}))
	defer srv.Close()

	result := executeWithAppScriptTestService(t, []string{
		"--json", "--account", "a@b.com",
		"appscript", "versions", "script123",
	}, newAppScriptTestService(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["nextPageToken"] != "page2" {
		t.Fatalf("dropped nextPageToken: %#v", parsed)
	}
}

func TestExecute_AppScriptVersions_RejectsNonPositiveMax(t *testing.T) {
	result := executeWithAppScriptTestServiceFactory(t, []string{
		"--account", "a@b.com",
		"appscript", "versions", "script123", "--max", "0",
	}, unexpectedAppScriptTestService(t, "max validation should run before creating the appscript service"))
	if result.err == nil || !strings.Contains(result.err.Error(), "max must be > 0") {
		t.Fatalf("unexpected err: %v", result.err)
	}
}
