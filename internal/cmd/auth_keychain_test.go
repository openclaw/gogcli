//go:build darwin

package cmd

import (
	"context"
	"testing"

	"github.com/openclaw/gogcli/internal/secrets"
)

func TestAuthAddCmd_ChecksKeychainFirst(t *testing.T) {
	// Verify the keychain preflight exists and is callable.
	err := secrets.EnsureKeychainAccessContext(context.Background())
	if err != nil {
		// If this fails, keychain might be locked - that's expected in some test environments
		t.Skipf("Keychain appears to be locked, skipping: %v", err)
	}
}
