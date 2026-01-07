//go:build !darwin

package secrets

// IsKeychainLockedError returns false on non-macOS platforms.
func IsKeychainLockedError(_ string) bool {
	return false
}

// CheckKeychainLocked returns false on non-macOS platforms.
func CheckKeychainLocked() bool {
	return false
}

// UnlockKeychain is a no-op on non-macOS platforms.
func UnlockKeychain() error {
	return nil
}

// EnsureKeychainAccess is a no-op on non-macOS platforms.
func EnsureKeychainAccess() error {
	return nil
}
