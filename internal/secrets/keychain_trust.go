package secrets

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// KeychainTrustApplicationInfo describes whether macOS Keychain should trust
// the current application when creating an item.
type KeychainTrustApplicationInfo struct {
	Enabled    bool
	Applicable bool
	Forced     bool
}

const codesignTimeout = 5 * time.Second

var cachedStableExecutableSignature = sync.OnceValue(func() bool {
	return executableHasStableSignature(runCodesign)
})

// ResolveKeychainTrustApplication resolves GOG_KEYCHAIN_TRUST_APPLICATION for
// a macOS Keychain backend. Invalid and empty values use automatic detection.
func ResolveKeychainTrustApplication(options OpenOptions, backendInfo KeyringBackendInfo) KeychainTrustApplicationInfo {
	if options.GOOS != goosDarwin || (backendInfo.Value != keyringBackendAuto && backendInfo.Value != keyringBackendKeychain) {
		return KeychainTrustApplicationInfo{}
	}

	switch strings.ToLower(strings.TrimSpace(options.KeychainTrustApplication)) {
	case "true", "1":
		return KeychainTrustApplicationInfo{Enabled: true, Applicable: true, Forced: true}
	case "false", "0":
		return KeychainTrustApplicationInfo{Applicable: true, Forced: true}
	}

	runner := options.codesignRunner
	if runner == nil {
		return KeychainTrustApplicationInfo{Enabled: cachedStableExecutableSignature(), Applicable: true}
	}

	return KeychainTrustApplicationInfo{Enabled: executableHasStableSignature(runner), Applicable: true}
}

func executableHasStableSignature(runner func(string) ([]byte, error)) bool {
	executable, err := os.Executable()
	if err != nil {
		return false
	}

	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return false
	}

	output, err := runner(executable)
	if err != nil {
		return false
	}

	return codesignOutputHasStableIdentity(output)
}

func runCodesign(path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), codesignTimeout)
	defer cancel()

	// path comes from os.Executable after symlink resolution, not user input.
	output, err := exec.CommandContext(ctx, "/usr/bin/codesign", "-dv", path).CombinedOutput() //nolint:gosec
	if err != nil {
		return output, fmt.Errorf("inspect executable signature: %w", err)
	}

	return output, nil
}

func codesignOutputHasStableIdentity(output []byte) bool {
	teamIdentifier := ""

	for line := range strings.SplitSeq(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.EqualFold(line, "Signature=adhoc") {
			return false
		}

		if value, ok := strings.CutPrefix(line, "TeamIdentifier="); ok {
			teamIdentifier = strings.TrimSpace(value)
		}
	}

	return teamIdentifier != "" && !strings.EqualFold(teamIdentifier, "not set")
}
