//go:build darwin

package secrets

import (
	"strings"
	"testing"
)

func TestIsKeychainLockedError(t *testing.T) {
	tests := []struct {
		name     string
		err      string
		expected bool
	}{
		{"locked error", "store token: User Interaction is not allowed. (-25308)", true},
		{"just error code", "some error (-25308)", true},
		{"different error", "store token: some other error", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsKeychainLockedError(tt.err)
			if got != tt.expected {
				t.Errorf("IsKeychainLockedError(%q) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestKeychainPath(t *testing.T) {
	path := loginKeychainPath()
	if path == "" {
		t.Error("keychainPath() returned empty string")
	}

	if !strings.HasSuffix(path, "login.keychain-db") {
		t.Errorf("unexpected keychain path: %s", path)
	}
}

func TestEnsureKeychainAccess_UnlockedKeychain(t *testing.T) {
	// On a normal dev machine, keychain should be unlocked
	err := EnsureKeychainAccess()
	if err != nil {
		t.Skipf("Keychain appears to be locked, skipping: %v", err)
	}
}
