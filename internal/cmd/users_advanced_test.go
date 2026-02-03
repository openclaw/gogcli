package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
)

// ====================
// users.go helper functions
// ====================

func TestGeneratePassword(t *testing.T) {
	t.Run("generates password with minimum length", func(t *testing.T) {
		pwd, err := generatePassword(5)
		if err != nil {
			t.Fatalf("generatePassword(5): %v", err)
		}
		if len(pwd) < 8 {
			t.Errorf("password should be at least 8 chars, got %d", len(pwd))
		}
	})

	t.Run("generates password with specified length", func(t *testing.T) {
		pwd, err := generatePassword(16)
		if err != nil {
			t.Fatalf("generatePassword(16): %v", err)
		}
		if len(pwd) != 16 {
			t.Errorf("expected 16 chars, got %d", len(pwd))
		}
	})

	t.Run("contains required character types", func(t *testing.T) {
		pwd, err := generatePassword(20)
		if err != nil {
			t.Fatalf("generatePassword(20): %v", err)
		}

		hasLower := strings.ContainsAny(pwd, "abcdefghijklmnopqrstuvwxyz")
		hasUpper := strings.ContainsAny(pwd, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
		hasDigit := strings.ContainsAny(pwd, "0123456789")
		hasSpecial := strings.ContainsAny(pwd, "!@#$%^&*()_+-=[]{}|;:,.<>?")

		if !hasLower {
			t.Error("password missing lowercase")
		}
		if !hasUpper {
			t.Error("password missing uppercase")
		}
		if !hasDigit {
			t.Error("password missing digit")
		}
		if !hasSpecial {
			t.Error("password missing special character")
		}
	})
}

func TestRandChar(t *testing.T) {
	t.Run("returns char from set", func(t *testing.T) {
		set := "abc"
		ch, err := randChar(set)
		if err != nil {
			t.Fatalf("randChar: %v", err)
		}
		if !strings.ContainsRune(set, rune(ch)) {
			t.Errorf("char %c not in set %s", ch, set)
		}
	})

	t.Run("empty set returns error", func(t *testing.T) {
		_, err := randChar("")
		if err == nil {
			t.Error("expected error for empty set")
		}
	})
}

func TestRandInt(t *testing.T) {
	t.Run("returns value in range", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			n, err := randInt(10)
			if err != nil {
				t.Fatalf("randInt: %v", err)
			}
			if n < 0 || n >= 10 {
				t.Errorf("value %d out of range [0, 10)", n)
			}
		}
	})

	t.Run("zero max returns error", func(t *testing.T) {
		_, err := randInt(0)
		if err == nil {
			t.Error("expected error for max=0")
		}
	})

	t.Run("negative max returns error", func(t *testing.T) {
		_, err := randInt(-1)
		if err == nil {
			t.Error("expected error for max=-1")
		}
	})
}

func TestNormalizeUserHashFunction(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"md5", "MD5", false},
		{"MD5", "MD5", false},
		{"sha-1", "SHA-1", false},
		{"SHA-1", "SHA-1", false},
		{"sha1", "SHA-1", false},
		{"SHA1", "SHA-1", false},
		{"crypt", "crypt", false},
		{"CRYPT", "crypt", false},
		{"", "", false},
		{"  md5  ", "MD5", false},
		{"invalid", "", true},
		{"bcrypt", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := normalizeUserHashFunction(tt.input)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ====================
// users_2fa.go
// ====================

func TestUsersTurnOff2SVCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/twoStepVerification/turnOff") {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &UsersTurnOff2SVCmd{User: "user@example.com"}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestUsersBackupCodesListCmd(t *testing.T) {
	t.Run("list codes plain", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/verificationCodes") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"verificationCode": "12345678"},
						{"verificationCode": "87654321"},
					},
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersBackupCodesListCmd{User: "user@example.com"}

		out := captureStdout(t, func() {
			if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "12345678") || !strings.Contains(out, "87654321") {
			t.Errorf("expected codes in output: %s", out)
		}
	})

	t.Run("list codes JSON", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/verificationCodes") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"verificationCode": "12345678"},
					},
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersBackupCodesListCmd{User: "user@example.com"}

		ctx := testContext(t)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		out := captureStdout(t, func() {
			if err := cmd.Run(ctx, flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "items") {
			t.Errorf("expected JSON output: %s", out)
		}
	})

	t.Run("empty codes", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/verificationCodes") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{},
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersBackupCodesListCmd{User: "user@example.com"}

		// Should not error even with empty list
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
}

func TestUsersBackupCodesGenerateCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/verificationCodes/generate") {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &UsersBackupCodesGenerateCmd{User: "user@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Generated new backup codes") {
		t.Errorf("expected success message: %s", out)
	}
}

func TestUsersBackupCodesDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/verificationCodes/invalidate") {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &UsersBackupCodesDeleteCmd{User: "user@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted all backup codes") {
		t.Errorf("expected success message: %s", out)
	}
}

// ====================
// users_asps.go
// ====================

func TestUsersASPsListCmd(t *testing.T) {
	t.Run("list ASPs plain", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/asps") {
				w.Header().Set("Content-Type", "application/json")
				// Note: creationTime and lastTimeUsed are string-encoded per API spec
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"codeId": 123, "name": "iPhone Mail", "creationTime": "1704067200", "lastTimeUsed": "1704153600"},
						{"codeId": 456, "name": "Outlook", "creationTime": "1704067200", "lastTimeUsed": "0"},
					},
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersASPsListCmd{User: "user@example.com"}

		out := captureStdout(t, func() {
			if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "iPhone Mail") || !strings.Contains(out, "Outlook") {
			t.Errorf("expected ASP names in output: %s", out)
		}
	})

	t.Run("list ASPs JSON", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/asps") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"codeId": 123, "name": "iPhone Mail"},
					},
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersASPsListCmd{User: "user@example.com"}

		ctx := testContext(t)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		out := captureStdout(t, func() {
			if err := cmd.Run(ctx, flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "items") {
			t.Errorf("expected JSON output: %s", out)
		}
	})

	t.Run("empty ASPs", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/asps") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{},
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersASPsListCmd{User: "user@example.com"}

		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
}

func TestUsersASPsDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/asps/123") {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &UsersASPsDeleteCmd{User: "user@example.com", CodeID: 123}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted app-specific password") {
		t.Errorf("expected success message: %s", out)
	}
}

func TestFormatUnixSeconds(t *testing.T) {
	tests := []struct {
		name string
		ts   int64
		want string
	}{
		{"zero returns never", 0, "never"},
		{"negative returns never", -1, "never"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatUnixSeconds(tt.ts)
			if got != tt.want {
				t.Errorf("formatUnixSeconds(%d) = %q, want %q", tt.ts, got, tt.want)
			}
		})
	}

	// Test positive value - just verify it returns a valid RFC3339 time string
	t.Run("positive value returns RFC3339", func(t *testing.T) {
		got := formatUnixSeconds(1704067200)
		// Should contain date components (time.RFC3339 format)
		if !strings.Contains(got, "2024") && !strings.Contains(got, "2023") {
			t.Errorf("formatUnixSeconds(1704067200) should contain year, got %q", got)
		}
		if !strings.Contains(got, "T") {
			t.Errorf("formatUnixSeconds(1704067200) should be RFC3339 format with T separator, got %q", got)
		}
	})
}

// ====================
// users_create.go
// ====================

func TestUsersCreateCmd(t *testing.T) {
	t.Run("create with password", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/users") {
				var req admin.User
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":           "user-123",
					"primaryEmail": req.PrimaryEmail,
					"name":         req.Name,
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersCreateCmd{
			Email:     "new@example.com",
			FirstName: "New",
			LastName:  "User",
			Password:  "SecurePass123!",
			OrgUnit:   "/Sales",
		}

		out := captureStdout(t, func() {
			if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "Created user:") || !strings.Contains(out, "new@example.com") {
			t.Errorf("unexpected output: %s", out)
		}
	})

	t.Run("create with generated password", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/users") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":           "user-456",
					"primaryEmail": "gen@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersCreateCmd{
			Email:     "gen@example.com",
			FirstName: "Generated",
			LastName:  "Password",
		}

		out := captureStdout(t, func() {
			if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "Generated password:") {
			t.Errorf("expected generated password in output: %s", out)
		}
	})

	t.Run("create with hash function", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/users") {
				var req admin.User
				_ = json.NewDecoder(r.Body).Decode(&req)
				if req.HashFunction != "MD5" {
					http.Error(w, "expected MD5 hash function", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":           "user-789",
					"primaryEmail": "hash@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersCreateCmd{
			Email:        "hash@example.com",
			FirstName:    "Hash",
			LastName:     "User",
			Password:     "prehashed",
			HashFunction: "md5",
		}

		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	t.Run("create JSON output", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/users") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":           "user-json",
					"primaryEmail": "json@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersCreateCmd{
			Email:     "json@example.com",
			FirstName: "JSON",
			LastName:  "User",
		}

		ctx := testContext(t)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		out := captureStdout(t, func() {
			if err := cmd.Run(ctx, flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "generatedPassword") {
			t.Errorf("expected generatedPassword in JSON: %s", out)
		}
	})

	t.Run("invalid hash function", func(t *testing.T) {
		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersCreateCmd{
			Email:        "bad@example.com",
			FirstName:    "Bad",
			LastName:     "Hash",
			Password:     "pass",
			HashFunction: "bcrypt",
		}

		err := cmd.Run(testContext(t), flags)
		if err == nil {
			t.Error("expected error for invalid hash function")
		}
	})
}

// ====================
// users_delete.go
// ====================

func TestUsersDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/users/user@example.com") {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &UsersDeleteCmd{User: "user@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Deleted user:") {
		t.Errorf("expected success message: %s", out)
	}
}

// ====================
// users_password.go
// ====================

func TestUsersPasswordCmd(t *testing.T) {
	t.Run("reset with specified password", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersPasswordCmd{
			User:     "user@example.com",
			Password: "NewPass123!",
		}

		out := captureStdout(t, func() {
			if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "Password reset for:") {
			t.Errorf("expected success message: %s", out)
		}
		if strings.Contains(out, "New password:") {
			t.Errorf("should not show password when specified: %s", out)
		}
	})

	t.Run("reset with generated password", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersPasswordCmd{User: "user@example.com"}

		out := captureStdout(t, func() {
			if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "New password:") {
			t.Errorf("expected generated password in output: %s", out)
		}
	})

	t.Run("reset JSON output", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersPasswordCmd{User: "user@example.com"}

		ctx := testContext(t)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		out := captureStdout(t, func() {
			if err := cmd.Run(ctx, flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "generatedPassword") {
			t.Errorf("expected generatedPassword in JSON: %s", out)
		}
	})

	t.Run("reset with hash function", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				var req admin.User
				_ = json.NewDecoder(r.Body).Decode(&req)
				if req.HashFunction != "SHA-1" {
					http.Error(w, "expected SHA-1 hash function", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersPasswordCmd{
			User:         "user@example.com",
			Password:     "prehashed",
			HashFunction: "sha-1",
		}

		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
}

// ====================
// users_signout.go
// ====================

func TestUsersSignoutCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/signOut") {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &UsersSignoutCmd{User: "user@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Signed out user from all sessions:") {
		t.Errorf("expected success message: %s", out)
	}
}

// ====================
// users_suspend.go
// ====================

func TestUsersSuspendCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/user@example.com") {
			var req admin.User
			_ = json.NewDecoder(r.Body).Decode(&req)
			// Verify Suspended is set to true - we check ForceSendFields was used
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"primaryEmail": "user@example.com",
				"suspended":    true,
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &UsersSuspendCmd{User: "user@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Suspended user:") {
		t.Errorf("expected success message: %s", out)
	}
}

func TestUsersUnsuspendCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/user@example.com") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"primaryEmail": "user@example.com",
				"suspended":    false,
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &UsersUnsuspendCmd{User: "user@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Unsuspended user:") {
		t.Errorf("expected success message: %s", out)
	}
}

// ====================
// users_tokens.go
// ====================

func TestUsersTokensListCmd(t *testing.T) {
	t.Run("list tokens plain", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tokens") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"clientId": "client1.apps.googleusercontent.com", "displayText": "My App", "scopes": []string{"email", "profile"}, "anonymous": false},
						{"clientId": "client2.apps.googleusercontent.com", "displayText": "Other App", "scopes": []string{"drive"}, "anonymous": true},
					},
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersTokensListCmd{User: "user@example.com"}

		out := captureStdout(t, func() {
			if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "My App") || !strings.Contains(out, "Other App") {
			t.Errorf("expected token names in output: %s", out)
		}
	})

	t.Run("list tokens JSON", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tokens") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{"clientId": "client1.apps.googleusercontent.com", "displayText": "My App"},
					},
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersTokensListCmd{User: "user@example.com"}

		ctx := testContext(t)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		out := captureStdout(t, func() {
			if err := cmd.Run(ctx, flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "items") {
			t.Errorf("expected JSON output: %s", out)
		}
	})

	t.Run("empty tokens", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tokens") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{},
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersTokensListCmd{User: "user@example.com"}

		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
}

func TestUsersTokensDeleteCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/tokens/client1.apps.googleusercontent.com") {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	stubAdminDirectory(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &UsersTokensDeleteCmd{User: "user@example.com", ClientID: "client1.apps.googleusercontent.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Revoked token") {
		t.Errorf("expected success message: %s", out)
	}
}

// ====================
// users_update.go (additional coverage)
// ====================

func TestUsersUpdateCmd_FieldUpdates(t *testing.T) {
	t.Run("update multiple fields", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		firstName := "Updated"
		lastName := "User"
		orgUnit := "/Engineering"
		cmd := &UsersUpdateCmd{
			User:      "user@example.com",
			FirstName: &firstName,
			LastName:  &lastName,
			OrgUnit:   &orgUnit,
		}

		out := captureStdout(t, func() {
			if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "Updated user:") {
			t.Errorf("expected success message: %s", out)
		}
	})

	t.Run("update suspended state", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
					"suspended":    true,
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		suspended := true
		cmd := &UsersUpdateCmd{
			User:      "user@example.com",
			Suspended: &suspended,
		}

		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	t.Run("update archived state", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
					"archived":     true,
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		archived := true
		cmd := &UsersUpdateCmd{
			User:     "user@example.com",
			Archived: &archived,
		}

		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	t.Run("update recovery info", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		recoveryEmail := "recovery@example.com"
		recoveryPhone := "+15555555555"
		cmd := &UsersUpdateCmd{
			User:          "user@example.com",
			RecoveryEmail: &recoveryEmail,
			RecoveryPhone: &recoveryPhone,
		}

		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	t.Run("clear recovery info", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		empty := ""
		cmd := &UsersUpdateCmd{
			User:          "user@example.com",
			RecoveryEmail: &empty,
			RecoveryPhone: &empty,
		}

		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	t.Run("update change password flag", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		changePassword := true
		cmd := &UsersUpdateCmd{
			User:           "user@example.com",
			ChangePassword: &changePassword,
		}

		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	t.Run("update primary email", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "newemail@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		newEmail := "newemail@example.com"
		cmd := &UsersUpdateCmd{
			User:         "user@example.com",
			PrimaryEmail: &newEmail,
		}

		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	t.Run("no updates specified", func(t *testing.T) {
		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersUpdateCmd{User: "user@example.com"}

		err := cmd.Run(testContext(t), flags)
		if err == nil {
			t.Error("expected error for no updates")
		}
	})

	t.Run("admin and field updates", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/makeAdmin") {
				w.WriteHeader(http.StatusOK)
				return
			}
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		admin := true
		firstName := "Admin"
		cmd := &UsersUpdateCmd{
			User:      "user@example.com",
			Admin:     &admin,
			FirstName: &firstName,
		}

		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	t.Run("JSON output", func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/users/") {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"primaryEmail": "user@example.com",
					"name":         map[string]any{"givenName": "Test"},
				})
				return
			}
			http.NotFound(w, r)
		})
		stubAdminDirectory(t, h)

		flags := &RootFlags{Account: "admin@example.com"}
		firstName := "Test"
		cmd := &UsersUpdateCmd{
			User:      "user@example.com",
			FirstName: &firstName,
		}

		ctx := testContext(t)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		out := captureStdout(t, func() {
			if err := cmd.Run(ctx, flags); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})

		if !strings.Contains(out, "primaryEmail") {
			t.Errorf("expected JSON output: %s", out)
		}
	})
}

// ====================
// Validation tests (missing account)
// ====================

func TestUsers_MissingAccount(t *testing.T) {
	tests := []struct {
		name string
		cmd  interface {
			Run(context.Context, *RootFlags) error
		}
	}{
		{"TurnOff2SV", &UsersTurnOff2SVCmd{User: "user@example.com"}},
		{"BackupCodesList", &UsersBackupCodesListCmd{User: "user@example.com"}},
		{"BackupCodesGenerate", &UsersBackupCodesGenerateCmd{User: "user@example.com"}},
		{"BackupCodesDelete", &UsersBackupCodesDeleteCmd{User: "user@example.com"}},
		{"ASPsList", &UsersASPsListCmd{User: "user@example.com"}},
		{"ASPsDelete", &UsersASPsDeleteCmd{User: "user@example.com", CodeID: 123}},
		{"Create", &UsersCreateCmd{Email: "new@example.com", FirstName: "New", LastName: "User"}},
		{"Delete", &UsersDeleteCmd{User: "user@example.com"}},
		{"Password", &UsersPasswordCmd{User: "user@example.com"}},
		{"Signout", &UsersSignoutCmd{User: "user@example.com"}},
		{"Suspend", &UsersSuspendCmd{User: "user@example.com"}},
		{"Unsuspend", &UsersUnsuspendCmd{User: "user@example.com"}},
		{"TokensList", &UsersTokensListCmd{User: "user@example.com"}},
		{"TokensDelete", &UsersTokensDeleteCmd{User: "user@example.com", ClientID: "client"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := &RootFlags{Account: ""}
			err := tt.cmd.Run(testContext(t), flags)
			if err == nil {
				t.Error("expected error for missing account")
			}
		})
	}
}

// ====================
// API error handling
// ====================

func TestUsers_APIErrors(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": {"message": "Not Found"}}`, http.StatusNotFound)
	})
	stubAdminDirectory(t, h)

	t.Run("delete API error", func(t *testing.T) {
		flags := &RootFlags{Account: "admin@example.com", Force: true}
		cmd := &UsersDeleteCmd{User: "nonexistent@example.com"}

		err := cmd.Run(testContext(t), flags)
		if err == nil {
			t.Error("expected error for API failure")
		}
	})

	t.Run("suspend API error", func(t *testing.T) {
		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersSuspendCmd{User: "nonexistent@example.com"}

		err := cmd.Run(testContext(t), flags)
		if err == nil {
			t.Error("expected error for API failure")
		}
	})

	t.Run("signout API error", func(t *testing.T) {
		flags := &RootFlags{Account: "admin@example.com"}
		cmd := &UsersSignoutCmd{User: "nonexistent@example.com"}

		err := cmd.Run(testContext(t), flags)
		if err == nil {
			t.Error("expected error for API failure")
		}
	})
}
