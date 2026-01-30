package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseGoogleOAuthClientJSON(t *testing.T) {
	t.Run("installed", func(t *testing.T) {
		got, err := ParseGoogleOAuthClientJSON([]byte(`{"installed":{"client_id":"id","client_secret":"sec"}}`))
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		if got.ClientID != "id" || got.ClientSecret != "sec" {
			t.Fatalf("unexpected: %#v", got)
		}
	})

	t.Run("web", func(t *testing.T) {
		got, err := ParseGoogleOAuthClientJSON([]byte(`{"web":{"client_id":"id","client_secret":"sec"}}`))
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		if got.ClientID != "id" || got.ClientSecret != "sec" {
			t.Fatalf("unexpected: %#v", got)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, err := ParseGoogleOAuthClientJSON([]byte(`{"nope":{}}`))
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if _, err := ParseGoogleOAuthClientJSON([]byte("{")); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestClientCredentials_Roundtrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	in := ClientCredentials{ClientID: "id", ClientSecret: "secret"}
	if err := WriteClientCredentials(in); err != nil {
		t.Fatalf("WriteClientCredentials: %v", err)
	}

	p, err := ClientCredentialsPath()
	if err != nil {
		t.Fatalf("ClientCredentialsPath: %v", err)
	}

	if filepath.Base(p) != "credentials.json" {
		t.Fatalf("unexpected base: %q", filepath.Base(p))
	}

	if _, statErr := os.Stat(p); statErr != nil {
		t.Fatalf("stat credentials: %v", statErr)
	}

	out, err := ReadClientCredentials()
	if err != nil {
		t.Fatalf("ReadClientCredentials: %v", err)
	}

	if out.ClientID != in.ClientID || out.ClientSecret != in.ClientSecret {
		t.Fatalf("mismatch: %#v != %#v", out, in)
	}
}

func TestReadClientCredentials_EnvVars(t *testing.T) {
	t.Run("env vars take precedence", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

		// Write file credentials first
		fileIn := ClientCredentials{ClientID: "file-id", ClientSecret: "file-secret"}
		if err := WriteClientCredentials(fileIn); err != nil {
			t.Fatalf("WriteClientCredentials: %v", err)
		}

		// Set env vars
		t.Setenv("GOG_CLIENT_ID", "env-id")
		t.Setenv("GOG_CLIENT_SECRET", "env-secret")

		// Env vars should win
		out, err := ReadClientCredentials()
		if err != nil {
			t.Fatalf("ReadClientCredentials: %v", err)
		}

		if out.ClientID != "env-id" || out.ClientSecret != "env-secret" {
			t.Fatalf("expected env vars to take precedence, got: %#v", out)
		}
	})

	t.Run("env vars only (no file)", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
		t.Setenv("GOG_CLIENT_ID", "env-only-id")
		t.Setenv("GOG_CLIENT_SECRET", "env-only-secret")

		out, err := ReadClientCredentials()
		if err != nil {
			t.Fatalf("ReadClientCredentials: %v", err)
		}

		if out.ClientID != "env-only-id" || out.ClientSecret != "env-only-secret" {
			t.Fatalf("unexpected: %#v", out)
		}
	})

	t.Run("partial env vars fall through to file", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

		// Only set one env var
		t.Setenv("GOG_CLIENT_ID", "env-id")
		// GOG_CLIENT_SECRET not set

		// Should fall through to file (and fail since no file exists)
		_, err := ReadClientCredentials()
		if err == nil {
			t.Fatalf("expected error when partial env and no file")
		}

		var missingErr *CredentialsMissingError
		if !errors.As(err, &missingErr) {
			t.Fatalf("expected CredentialsMissingError, got %T: %v", err, err)
		}
	})
}

func TestReadClientCredentials_Errors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	_, err := ReadClientCredentials()
	if err == nil {
		t.Fatalf("expected error")
	}
	var missingErr *CredentialsMissingError

	if !errors.As(err, &missingErr) {
		t.Fatalf("expected CredentialsMissingError, got %T", err)
	}

	path, pathErr := ClientCredentialsPath()
	if pathErr != nil {
		t.Fatalf("ClientCredentialsPath: %v", pathErr)
	}

	if _, dirErr := EnsureDir(); dirErr != nil {
		t.Fatalf("EnsureDir: %v", dirErr)
	}

	if writeErr := os.WriteFile(path, []byte(`{"client_id":""}`), 0o600); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	if _, err := ReadClientCredentials(); err == nil {
		t.Fatalf("expected missing field error")
	}
}
