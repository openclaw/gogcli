package config

import "os"

// EnvWithFallback reads the primary env var; if empty, falls back to legacy.
// This supports the GOG_* → RATA_* rename with backward compatibility.
func EnvWithFallback(primary, legacy string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}

	return os.Getenv(legacy)
}
