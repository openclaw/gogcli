package config

import "testing"

func TestIsNoSendAccount(t *testing.T) {
	cfg := File{
		NoSendAccounts: map[string]bool{
			"blocked@gmail.com": true,
		},
	}

	tests := []struct {
		email string
		want  bool
	}{
		{"blocked@gmail.com", true},
		{"BLOCKED@gmail.com", true},
		{" blocked@gmail.com ", true},
		{"allowed@gmail.com", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := IsNoSendAccount(cfg, tc.email); got != tc.want {
			t.Errorf("IsNoSendAccount(%q) = %v, want %v", tc.email, got, tc.want)
		}
	}
}

func TestIsNoSendAccount_NilMap(t *testing.T) {
	cfg := File{}

	if IsNoSendAccount(cfg, "any@gmail.com") {
		t.Error("expected false for nil map")
	}
}

func TestSetNoSendAccount(t *testing.T) {
	cfg := File{}

	if err := SetNoSendAccount(&cfg, "user@gmail.com", true); err != nil {
		t.Fatal(err)
	}

	if !cfg.NoSendAccounts["user@gmail.com"] {
		t.Error("expected account to be blocked")
	}

	if err := SetNoSendAccount(&cfg, "user@gmail.com", false); err != nil {
		t.Fatal(err)
	}

	if cfg.NoSendAccounts != nil {
		t.Errorf("expected nil map after removing only entry, got %v", cfg.NoSendAccounts)
	}
}

func TestSetNoSendAccount_EmptyEmail(t *testing.T) {
	cfg := File{}

	if err := SetNoSendAccount(&cfg, "", true); err == nil {
		t.Error("expected error for empty email")
	}
}

func TestListNoSendAccounts(t *testing.T) {
	cfg := File{
		NoSendAccounts: map[string]bool{
			"b@gmail.com": true,
			"a@gmail.com": true,
		},
	}
	got := ListNoSendAccounts(cfg)

	if len(got) != 2 || got[0] != "a@gmail.com" || got[1] != "b@gmail.com" {
		t.Errorf("ListNoSendAccounts = %v, want [a@gmail.com b@gmail.com]", got)
	}
}

func TestListNoSendAccounts_Empty(t *testing.T) {
	cfg := File{}
	got := ListNoSendAccounts(cfg)

	if len(got) != 0 {
		t.Errorf("expected empty list, got %v", got)
	}
}
