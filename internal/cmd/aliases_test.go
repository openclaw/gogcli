package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
)

func TestAliasesListCmd_User(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/aliases") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"aliases": []string{"alias@example.com"},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AliasesListCmd{User: "user@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "alias@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAliasesListCmd_Group(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/groups/") || !strings.Contains(r.URL.Path, "/aliases") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"aliases": []string{"group-alias@example.com", "another-alias@example.com"},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AliasesListCmd{Group: "mygroup@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "group-alias@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAliasesListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/aliases") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"aliases": []string{"json-alias@example.com"},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AliasesListCmd{User: "user@example.com"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "json-alias@example.com") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestAliasesListCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/aliases") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"aliases": []string{},
		})
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AliasesListCmd{User: "user@example.com"}

	// No error expected, just "no aliases" message
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAliasesListCmd_RequiresUserOrGroup(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AliasesListCmd{}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when neither User nor Group is provided")
	}
}

func TestAliasesListCmd_CannotProvideBoth(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AliasesListCmd{User: "user@example.com", Group: "group@example.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when both User and Group are provided")
	}
}

func TestAliasesCreateCmd_User(t *testing.T) {
	var gotAlias string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/users/") && strings.Contains(r.URL.Path, "/aliases") {
			var payload struct {
				Alias string `json:"alias"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			gotAlias = payload.Alias
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"alias": payload.Alias,
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AliasesCreateCmd{Alias: "newalias@example.com", User: "user@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotAlias != "newalias@example.com" {
		t.Fatalf("expected alias newalias@example.com, got %q", gotAlias)
	}
	if !strings.Contains(out, "Created alias") || !strings.Contains(out, "user") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAliasesCreateCmd_Group(t *testing.T) {
	var gotAlias string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/groups/") && strings.Contains(r.URL.Path, "/aliases") {
			var payload struct {
				Alias string `json:"alias"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			gotAlias = payload.Alias
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"alias": payload.Alias,
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AliasesCreateCmd{Alias: "groupalias@example.com", Group: "mygroup@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotAlias != "groupalias@example.com" {
		t.Fatalf("expected alias groupalias@example.com, got %q", gotAlias)
	}
	if !strings.Contains(out, "Created alias") || !strings.Contains(out, "group") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAliasesCreateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/users/") && strings.Contains(r.URL.Path, "/aliases") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"alias": "json-alias@example.com",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AliasesCreateCmd{Alias: "json-alias@example.com", User: "user@example.com"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "json-alias@example.com") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestAliasesCreateCmd_RequiresUserOrGroup(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AliasesCreateCmd{Alias: "alias@example.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when neither User nor Group is provided")
	}
}

func TestAliasesDeleteCmd_User(t *testing.T) {
	deleted := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/users/") && strings.Contains(r.URL.Path, "/aliases/") {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &AliasesDeleteCmd{Alias: "deleteme@example.com", User: "user@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleted {
		t.Fatal("expected delete API call")
	}
	if !strings.Contains(out, "Deleted alias") || !strings.Contains(out, "user") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAliasesDeleteCmd_Group(t *testing.T) {
	deleted := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/groups/") && strings.Contains(r.URL.Path, "/aliases/") {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &AliasesDeleteCmd{Alias: "deleteme@example.com", Group: "mygroup@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleted {
		t.Fatal("expected delete API call")
	}
	if !strings.Contains(out, "Deleted alias") || !strings.Contains(out, "group") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAliasesDeleteCmd_RequiresConfirmation(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", NoInput: true}
	cmd := &AliasesDeleteCmd{Alias: "deleteme@example.com", User: "user@example.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when NoInput is set without Force")
	}
}

func TestAliasesDeleteCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/users/") && strings.Contains(r.URL.Path, "/aliases/") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &AliasesDeleteCmd{Alias: "json-delete@example.com", User: "user@example.com"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "json-delete@example.com") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestAliasesDeleteCmd_RequiresUserOrGroup(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &AliasesDeleteCmd{Alias: "alias@example.com"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when neither User nor Group is provided")
	}
}

func TestResolveAliasTarget(t *testing.T) {
	tests := []struct {
		name      string
		user      string
		group     string
		wantUser  string
		wantGroup string
		wantErr   bool
	}{
		{
			name:      "user only",
			user:      "user@example.com",
			group:     "",
			wantUser:  "user@example.com",
			wantGroup: "",
			wantErr:   false,
		},
		{
			name:      "group only",
			user:      "",
			group:     "group@example.com",
			wantUser:  "",
			wantGroup: "group@example.com",
			wantErr:   false,
		},
		{
			name:      "neither provided",
			user:      "",
			group:     "",
			wantUser:  "",
			wantGroup: "",
			wantErr:   true,
		},
		{
			name:      "both provided",
			user:      "user@example.com",
			group:     "group@example.com",
			wantUser:  "",
			wantGroup: "",
			wantErr:   true,
		},
		{
			name:      "whitespace user",
			user:      "  user@example.com  ",
			group:     "",
			wantUser:  "user@example.com",
			wantGroup: "",
			wantErr:   false,
		},
		{
			name:      "whitespace only user",
			user:      "   ",
			group:     "",
			wantUser:  "",
			wantGroup: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUser, gotGroup, err := resolveAliasTarget(tt.user, tt.group)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveAliasTarget() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotUser != tt.wantUser {
				t.Errorf("resolveAliasTarget() gotUser = %v, want %v", gotUser, tt.wantUser)
			}
			if gotGroup != tt.wantGroup {
				t.Errorf("resolveAliasTarget() gotGroup = %v, want %v", gotGroup, tt.wantGroup)
			}
		})
	}
}
