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

func TestCAALevelsGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/accessPolicies/123/accessLevels/mylevel") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "accessPolicies/123/accessLevels/mylevel",
			"title":       "mylevel",
			"description": "Test level",
			"basic": map[string]any{
				"conditions": []map[string]any{{"ipSubnetworks": []string{"10.0.0.0/24"}}},
			},
		})
	})
	stubAccessContextManager(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CAALevelsGetCmd{Name: "mylevel", Policy: "123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Name:") || !strings.Contains(out, "mylevel") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "Type:") || !strings.Contains(out, "basic") {
		t.Fatalf("expected basic type in output: %s", out)
	}
}

func TestCAALevelsGetCmd_FullName(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/accessPolicies/456/accessLevels/fulllevel") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":        "accessPolicies/456/accessLevels/fulllevel",
			"title":       "fulllevel",
			"description": "Full name test",
			"custom": map[string]any{
				"expr": map[string]any{"expression": "device.os_type == \"DESKTOP_MAC\""},
			},
		})
	})
	stubAccessContextManager(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CAALevelsGetCmd{Name: "accessPolicies/456/accessLevels/fulllevel"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "fulllevel") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "custom") {
		t.Fatalf("expected custom type in output: %s", out)
	}
}

func TestCAALevelsUpdateCmd(t *testing.T) {
	var gotUpdateMask string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/accessPolicies/123/accessLevels/mylevel") {
			http.NotFound(w, r)
			return
		}
		gotUpdateMask = r.URL.Query().Get("updateMask")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "operations/op-2",
		})
	})
	stubAccessContextManager(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	desc := "Updated description"
	cmd := &CAALevelsUpdateCmd{
		Name:        "mylevel",
		Policy:      "123",
		Description: &desc,
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(gotUpdateMask, "description") {
		t.Fatalf("unexpected updateMask: %q", gotUpdateMask)
	}
	if !strings.Contains(out, "Updated access level") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCAALevelsUpdateCmd_WithConditions(t *testing.T) {
	var gotBody map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || !strings.Contains(r.URL.Path, "/accessPolicies/123/accessLevels/mylevel") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "operations/op-3",
		})
	})
	stubAccessContextManager(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CAALevelsUpdateCmd{
		Name:       "mylevel",
		Policy:     "123",
		Conditions: []string{"{\"ipSubnetworks\":[\"192.168.0.0/16\"]}"},
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotBody["basic"] == nil {
		t.Fatalf("expected basic conditions in body: %v", gotBody)
	}
	if !strings.Contains(out, "Updated access level") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCAALevelsDeleteCmd(t *testing.T) {
	var deletedName string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/accessPolicies/123/accessLevels/mylevel") {
			http.NotFound(w, r)
			return
		}
		deletedName = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "operations/op-delete",
		})
	})
	stubAccessContextManager(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &CAALevelsDeleteCmd{Name: "mylevel", Policy: "123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(deletedName, "mylevel") {
		t.Fatalf("expected mylevel in deleted path: %s", deletedName)
	}
	if !strings.Contains(out, "Deleted access level") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCAALevelsCreateCmd_Custom(t *testing.T) {
	var gotBody map[string]any
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/accessPolicies/123/accessLevels") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "operations/op-custom",
		})
	})
	stubAccessContextManager(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &CAALevelsCreateCmd{
		Name:   "customlevel",
		Policy: "123",
		Custom: true,
		Expr:   "device.os_type == \"DESKTOP_MAC\"",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotBody["custom"] == nil {
		t.Fatalf("expected custom in body: %v", gotBody)
	}
	if !strings.Contains(out, "Created access level") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestNormalizeAccessPolicy(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"123", "accessPolicies/123"},
		{"accessPolicies/123", "accessPolicies/123"},
		{"  456  ", "accessPolicies/456"},
		{"", ""},
	}
	for _, tc := range tests {
		got := normalizeAccessPolicy(tc.input)
		if got != tc.want {
			t.Errorf("normalizeAccessPolicy(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeAccessLevelName(t *testing.T) {
	tests := []struct {
		policy  string
		name    string
		want    string
		wantErr bool
	}{
		{"123", "level1", "accessPolicies/123/accessLevels/level1", false},
		{"accessPolicies/123", "level1", "accessPolicies/123/accessLevels/level1", false},
		{"", "accessPolicies/123/accessLevels/level1", "accessPolicies/123/accessLevels/level1", false},
		{"", "level1", "", true},
		{"123", "", "", true},
	}
	for _, tc := range tests {
		got, err := normalizeAccessLevelName(tc.policy, tc.name)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeAccessLevelName(%q, %q) expected error", tc.policy, tc.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeAccessLevelName(%q, %q) error: %v", tc.policy, tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeAccessLevelName(%q, %q) = %q, want %q", tc.policy, tc.name, got, tc.want)
		}
	}
}

func TestAccessLevelType(t *testing.T) {
	tests := []struct {
		name  string
		level *accesscontextmanager.AccessLevel
		want  string
	}{
		{"nil", nil, ""},
		{"basic", &accesscontextmanager.AccessLevel{Basic: &accesscontextmanager.BasicLevel{}}, "basic"},
		{"custom", &accesscontextmanager.AccessLevel{Custom: &accesscontextmanager.CustomLevel{}}, "custom"},
		{"unknown", &accesscontextmanager.AccessLevel{}, "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := accessLevelType(tc.level)
			if got != tc.want {
				t.Errorf("accessLevelType() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAccessLevelTitle(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"accessPolicies/123/accessLevels/mylevel", "mylevel"},
		{"mylevel", "mylevel"},
		{"", ""},
		// When name ends with /, function returns full name (no empty suffix)
		{"accessPolicies/123/accessLevels/", "accessPolicies/123/accessLevels/"},
	}
	for _, tc := range tests {
		got := accessLevelTitle(tc.name)
		if got != tc.want {
			t.Errorf("accessLevelTitle(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func stubAccessContextManager(t *testing.T, handler http.Handler) {
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
}
