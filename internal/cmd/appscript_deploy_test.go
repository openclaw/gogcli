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

func TestExecute_AppScriptDeploy_CutsVersionThenDeploys(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "/versions"):
			_ = json.NewEncoder(w).Encode(map[string]any{"scriptId": "script123", "versionNumber": 4})
		case strings.HasSuffix(r.URL.Path, "/deployments"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deploymentId": "AKfyc123",
				"entryPoints": []map[string]any{
					{"webApp": map[string]any{"url": "https://example.test/exec"}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "deploy", "script123", "--description", "  nightly  ",
	}, newAppScriptTestService(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	if len(paths) != 2 || !strings.Contains(paths[0], "/versions") || !strings.Contains(paths[1], "/deployments") {
		t.Fatalf("unexpected call order: %v", paths)
	}
	if !strings.Contains(result.stdout, "version\t4") ||
		!strings.Contains(result.stdout, "deployment_id\tAKfyc123") ||
		!strings.Contains(result.stdout, "web_app_url\thttps://example.test/exec") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}

func TestExecute_AppScriptDeploy_DeploymentIDUpdatesInPlace(t *testing.T) {
	var deploymentMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(r.URL.Path, "/versions") {
			_ = json.NewEncoder(w).Encode(map[string]any{"versionNumber": 9})
			return
		}

		deploymentMethod = r.Method
		_ = json.NewEncoder(w).Encode(map[string]any{"deploymentId": "AKfyc123"})
	}))
	defer srv.Close()

	result := executeWithAppScriptTestService(t, []string{
		"--account", "a@b.com",
		"appscript", "deploy", "script123", "--deployment-id", "AKfyc123",
	}, newAppScriptTestService(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if deploymentMethod != http.MethodPut {
		t.Fatalf("expected an in-place update (PUT), got %s", deploymentMethod)
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

// Deleting a deployment permanently breaks its web app URL, so a
// non-interactive run must opt in explicitly.
func TestExecute_AppScriptUndeploy_RequiresForceWhenNonInteractive(t *testing.T) {
	result := executeWithAppScriptTestServiceFactory(t, []string{
		"--account", "a@b.com",
		"appscript", "undeploy", "script123", "AKfyc123",
	}, unexpectedAppScriptTestService(t, "undeploy should stop at the confirmation gate"))
	if result.err == nil || !strings.Contains(result.err.Error(), "without --force") {
		t.Fatalf("unexpected err: %v", result.err)
	}
}

func TestExecute_AppScriptUndeploy_ForceDeletes(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	result := executeWithAppScriptTestService(t, []string{
		"--force", "--account", "a@b.com",
		"appscript", "undeploy", "script123", "AKfyc123",
	}, newAppScriptTestService(t, srv))
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if method != http.MethodDelete || !strings.Contains(path, "deployments/AKfyc123") {
		t.Fatalf("unexpected request: %s %s", method, path)
	}
	if !strings.Contains(result.stdout, "deleted\ttrue") {
		t.Fatalf("unexpected out=%q", result.stdout)
	}
}
