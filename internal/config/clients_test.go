package config

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempConfigDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	return dir
}

func TestNormalizeClientName(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cases := []struct {
			in   string
			want string
		}{
			{in: "Default", want: "default"},
			{in: "foo_bar", want: "foo_bar"},
			{in: "foo-bar", want: "foo-bar"},
			{in: "foo.bar", want: "foo.bar"},
			{in: "foo123", want: "foo123"},
			{in: "MiXeD-01", want: "mixed-01"},
			{in: " spaced ", want: "spaced"},
			{in: "under_01", want: "under_01"},
			{in: "dot._-01", want: "dot._-01"},
			{in: "caps.DOT", want: "caps.dot"},
			{in: "lowercase", want: "lowercase"},
		}
		for _, c := range cases {
			got, err := NormalizeClientName(c.in)
			if err != nil {
				t.Fatalf("NormalizeClientName(%q): %v", c.in, err)
			}

			if got != c.want {
				t.Fatalf("NormalizeClientName(%q) = %q, want %q", c.in, got, c.want)
			}
		}
	})

	t.Run("invalid", func(t *testing.T) {
		cases := []string{
			"",
			" ",
			"foo bar",
			"foo/bar",
			"foo!",
			"foo@bar",
		}
		for _, in := range cases {
			if _, err := NormalizeClientName(in); err == nil {
				t.Fatalf("expected error for %q", in)
			}
		}
	})
}

func TestNormalizeDomainAndEmail(t *testing.T) {
	t.Run("normalize domain", func(t *testing.T) {
		cases := []struct {
			in   string
			want string
		}{
			{in: "Example.COM", want: "example.com"},
			{in: "@example.com", want: "example.com"},
			{in: " sub.domain ", want: "sub.domain"},
		}
		for _, c := range cases {
			got, err := NormalizeDomain(c.in)
			if err != nil {
				t.Fatalf("NormalizeDomain(%q): %v", c.in, err)
			}

			if got != c.want {
				t.Fatalf("NormalizeDomain(%q) = %q, want %q", c.in, got, c.want)
			}
		}
	})

	t.Run("invalid domain", func(t *testing.T) {
		cases := []string{"", "example", "exa mple.com", "bad!domain.com"}
		for _, in := range cases {
			if _, err := NormalizeDomain(in); err == nil {
				t.Fatalf("expected error for %q", in)
			}
		}
	})

	t.Run("domain from email", func(t *testing.T) {
		cases := map[string]string{
			"user@example.com":      "example.com",
			"USER@EXAMPLE.COM":      "example.com",
			"not-an-email":          "",
			"user@":                 "",
			"user@multi@domain.com": "",
		}
		for in, want := range cases {
			got := DomainFromEmail(in)
			if got != want {
				t.Fatalf("DomainFromEmail(%q) = %q, want %q", in, got, want)
			}
		}
	})
}

func TestAccountAndDomainMappings(t *testing.T) {
	cfg := File{}

	if err := SetAccountClient(&cfg, "", "work"); err == nil {
		t.Fatalf("expected empty email error")
	}

	if err := SetAccountClient(&cfg, "USER@EXAMPLE.COM", "Work"); err != nil {
		t.Fatalf("SetAccountClient: %v", err)
	}

	if got, ok := AccountClient(cfg, "user@example.com"); !ok || got != "work" {
		t.Fatalf("AccountClient = %q,%v", got, ok)
	}

	cfg.AccountClients["bad@example.com"] = "bad!"
	if _, ok := AccountClient(cfg, "bad@example.com"); ok {
		t.Fatalf("expected invalid account client to be ignored")
	}

	if err := SetClientDomain(&cfg, "example.com", "Org"); err != nil {
		t.Fatalf("SetClientDomain: %v", err)
	}

	if got, ok := ClientForDomain(cfg, "EXAMPLE.COM"); !ok || got != "org" {
		t.Fatalf("ClientForDomain = %q,%v", got, ok)
	}

	if err := SetClientDomain(&cfg, "invalid", "org"); err == nil {
		t.Fatalf("expected invalid domain error")
	}
}

func TestResolveClientForAccount(t *testing.T) {
	dir := withTempConfigDir(t)

	t.Run("override wins", func(t *testing.T) {
		cfg := File{
			AccountClients: map[string]string{"user@example.com": "work"},
			ClientDomains:  map[string]string{"example.com": "domain"},
		}

		got, err := ResolveClientForAccount(cfg, "user@example.com", "Custom")
		if err != nil {
			t.Fatalf("ResolveClientForAccount: %v", err)
		}

		if got != "custom" {
			t.Fatalf("got %q, want custom", got)
		}
	})

	t.Run("account mapping", func(t *testing.T) {
		cfg := File{AccountClients: map[string]string{"user@example.com": "work"}}

		got, err := ResolveClientForAccount(cfg, "USER@example.com", "")
		if err != nil {
			t.Fatalf("ResolveClientForAccount: %v", err)
		}

		if got != "work" {
			t.Fatalf("got %q, want work", got)
		}
	})

	t.Run("domain mapping", func(t *testing.T) {
		cfg := File{ClientDomains: map[string]string{"example.com": "domain"}}

		got, err := ResolveClientForAccount(cfg, "user@example.com", "")
		if err != nil {
			t.Fatalf("ResolveClientForAccount: %v", err)
		}

		if got != "domain" {
			t.Fatalf("got %q, want domain", got)
		}
	})

	t.Run("credentials by domain", func(t *testing.T) {
		path := filepath.Join(dir, "credentials-example.com.json")
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		cfg := File{}

		got, err := ResolveClientForAccount(cfg, "user@example.com", "")
		if err != nil {
			t.Fatalf("ResolveClientForAccount: %v", err)
		}

		if got != "example.com" {
			t.Fatalf("got %q, want example.com", got)
		}
	})

	t.Run("default fallback", func(t *testing.T) {
		cfg := File{}

		got, err := ResolveClientForAccount(cfg, "user@nomap.com", "")
		if err != nil {
			t.Fatalf("ResolveClientForAccount: %v", err)
		}

		if got != DefaultClientName {
			t.Fatalf("got %q, want %q", got, DefaultClientName)
		}
	})

	t.Run("client name without @ is used as-is", func(t *testing.T) {
		// When a bare client name like "company.ai" is passed (no @),
		// it should be returned directly as the client name.
		cfg := File{}
		got, err := ResolveClientForAccount(cfg, "company.ai", "")
		if err != nil {
			t.Fatalf("ResolveClientForAccount: %v", err)
		}
		if got != "company.ai" {
			t.Fatalf("got %q, want %q", got, "company.ai")
		}
	})

	t.Run("subdomain email routes via account_clients mapping", func(t *testing.T) {
		// Emails with subdomain domains (e.g. user@sub.company.ai) should be
		// routable via account_clients config to the correct client.
		cfg := File{
			AccountClients: map[string]string{
				"user@sub.company.ai": "company.ai",
			},
		}
		got, err := ResolveClientForAccount(cfg, "user@sub.company.ai", "")
		if err != nil {
			t.Fatalf("ResolveClientForAccount: %v", err)
		}
		if got != "company.ai" {
			t.Fatalf("got %q, want %q", got, "company.ai")
		}
	})
}

func TestListClientCredentials(t *testing.T) {
	dir := withTempConfigDir(t)

	files := []string{
		"credentials.json",
		"credentials-work.json",
		"credentials-bad!.json",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	list, err := ListClientCredentials()
	if err != nil {
		t.Fatalf("ListClientCredentials: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}

	if list[0].Client != DefaultClientName || !list[0].Default {
		t.Fatalf("expected default credentials first, got %+v", list[0])
	}

	if list[1].Client != "work" || list[1].Default {
		t.Fatalf("expected work credentials second, got %+v", list[1])
	}
}
