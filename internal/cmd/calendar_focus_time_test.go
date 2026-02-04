package cmd

import "testing"

func TestValidateAutoDeclineMode(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"none", "declineNone", false},
		{"all", declineAllConflicting, false},
		{"new", "declineOnlyNewConflictingInvitations", false},
		{"", "declineNone", false},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := validateAutoDeclineMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAutoDeclineMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validateAutoDeclineMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateChatStatus(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"available", "available", false},
		{"doNotDisturb", "doNotDisturb", false},
		{"dnd", "doNotDisturb", false},
		{"", "available", false},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := validateChatStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateChatStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validateChatStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
