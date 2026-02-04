package googleapi

import "testing"

func TestEscapeDriveQueryValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no special chars", "hello", "hello"},
		{"single quote", "'", "\\'"},
		{"backslash", "\\", "\\\\"},
		{"backslash then quote", "\\'", "\\\\\\'"},
		{"multiple quotes", "a'b'c", "a\\'b\\'c"},
		{"already escaped", "a\\\\'b", "a\\\\\\\\\\'b"},
		{"email address", "user@example.com", "user@example.com"},
		{"folder ID", "1A2B3C_xyz", "1A2B3C_xyz"},
		{"mixed", "it's a \\path", "it\\'s a \\\\path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeDriveQueryValue(tt.in)
			if got != tt.want {
				t.Errorf("EscapeDriveQueryValue(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
