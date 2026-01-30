package places

import (
	"testing"
)

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "empty", key: "", want: ""},
		{name: "whitespace only", key: "   ", want: ""},
		{name: "short key 1 char", key: "a", want: "****"},
		{name: "short key 4 chars", key: "abcd", want: "****"},
		{name: "5 chars shows last 4", key: "abcde", want: "****bcde"},
		{name: "typical api key", key: "FAKE-TEST-KEY-abcdefghijklmnopqrstuvwxyz", want: "****wxyz"},
		{name: "key with leading whitespace", key: "  abcdefgh", want: "****efgh"},
		{name: "key with trailing whitespace", key: "abcdefgh  ", want: "****efgh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskAPIKey(tt.key)
			if got != tt.want {
				t.Errorf("MaskAPIKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestSaveAPIKeyKeychain_EmptyKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "empty", key: ""},
		{name: "whitespace only", key: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SaveAPIKeyKeychain(tt.key)
			if err == nil {
				t.Error("expected error for empty key")
			}

			if err.Error() != "empty places api key" {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSaveAPIKeyConfig_EmptyKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "empty", key: ""},
		{name: "whitespace only", key: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SaveAPIKeyConfig(tt.key)
			if err == nil {
				t.Error("expected error for empty key")
			}

			if err.Error() != "empty places api key" {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAPIKeySource_Constants(t *testing.T) {
	if APIKeySourceNone != "none" {
		t.Errorf("APIKeySourceNone = %q, want %q", APIKeySourceNone, "none")
	}

	if APIKeySourceEnv != "env" {
		t.Errorf("APIKeySourceEnv = %q, want %q", APIKeySourceEnv, "env")
	}

	if APIKeySourceKeychain != "keychain" {
		t.Errorf("APIKeySourceKeychain = %q, want %q", APIKeySourceKeychain, "keychain")
	}

	if APIKeySourceConfig != "config" {
		t.Errorf("APIKeySourceConfig = %q, want %q", APIKeySourceConfig, "config")
	}
}

func TestEnvAPIKey_Constant(t *testing.T) {
	if EnvAPIKey != "GOOGLE_PLACES_API_KEY" {
		t.Errorf("EnvAPIKey = %q, want %q", EnvAPIKey, "GOOGLE_PLACES_API_KEY")
	}
}
