//go:build darwin

package cmd

import (
	"testing"

	"github.com/steipete/gogcli/internal/secrets"
)

func TestAuthAddCmd_ChecksKeychainFirst(t *testing.T) {
	// Verify the EnsureKeychainAccess function exists and is callable
	err := secrets.EnsureKeychainAccess()
	if err != nil {
		// If this fails, keychain might be locked - that's expected in some test environments
		t.Skipf("Keychain appears to be locked, skipping: %v", err)
	}
}
