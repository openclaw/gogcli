package googleauth

import (
	"context"
	"net"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestAuthURLParams(t *testing.T) {
	t.Parallel()

	cfg := oauth2.Config{
		ClientID:    "id",
		Endpoint:    oauth2.Endpoint{AuthURL: "https://example.com/auth"},
		RedirectURL: "http://localhost",
		Scopes:      []string{"s1"},
	}

	u1 := cfg.AuthCodeURL("state", authURLParams(false, true)...)
	var parsed1 *url.URL

	if p, err := url.Parse(u1); err != nil {
		t.Fatalf("parse: %v", err)
	} else {
		parsed1 = p
	}

	if accessType := parsed1.Query().Get("access_type"); accessType != "offline" {
		t.Fatalf("expected offline, got: %q", accessType)
	}

	if includeScopes := parsed1.Query().Get("include_granted_scopes"); includeScopes != "true" {
		t.Fatalf("expected include_granted_scopes=true, got: %q", includeScopes)
	}

	if prompt := parsed1.Query().Get("prompt"); prompt != "" {
		t.Fatalf("expected no prompt, got: %q", prompt)
	}

	u2 := cfg.AuthCodeURL("state", authURLParams(true, true)...)
	var parsed2 *url.URL

	if p, err := url.Parse(u2); err != nil {
		t.Fatalf("parse: %v", err)
	} else {
		parsed2 = p
	}

	if parsed2.Query().Get("prompt") != "consent" {
		t.Fatalf("expected consent prompt, got: %q", parsed2.Query().Get("prompt"))
	}

	u3 := cfg.AuthCodeURL("state", authURLParams(false, false)...)
	var parsed3 *url.URL

	if p, err := url.Parse(u3); err != nil {
		t.Fatalf("parse: %v", err)
	} else {
		parsed3 = p
	}

	if includeScopes := parsed3.Query().Get("include_granted_scopes"); includeScopes != "" {
		t.Fatalf("expected no include_granted_scopes, got: %q", includeScopes)
	}
}

func TestRandomState(t *testing.T) {
	t.Parallel()

	var s1 string

	if state, err := randomState(); err != nil {
		t.Fatalf("randomState: %v", err)
	} else {
		s1 = state
	}

	var s2 string

	if state, err := randomState(); err != nil {
		t.Fatalf("randomState: %v", err)
	} else {
		s2 = state
	}

	if s1 == "" || s2 == "" || s1 == s2 {
		t.Fatalf("expected two non-empty distinct states")
	}
	// base64 RawURLEncoding charset should not include '+' or '/' or '='.
	if strings.ContainsAny(s1, "+/=") || strings.ContainsAny(s2, "+/=") {
		t.Fatalf("unexpected charset: %q %q", s1, s2)
	}
}

func TestNormalizeRedirectURI(t *testing.T) {
	t.Parallel()

	got, err := normalizeRedirectURI("https://host.example/oauth2/callback")
	if err != nil {
		t.Fatalf("normalizeRedirectURI: %v", err)
	}

	if got != "https://host.example/oauth2/callback" {
		t.Fatalf("unexpected redirect uri: %q", got)
	}

	got, err = normalizeRedirectURI("https://host.example")
	if err != nil {
		t.Fatalf("normalizeRedirectURI host-only: %v", err)
	}

	if got != "https://host.example/" {
		t.Fatalf("expected trailing slash for host-only uri, got: %q", got)
	}

	if _, err := normalizeRedirectURI("host-only/path"); err == nil {
		t.Fatalf("expected error for invalid redirect uri")
	}

	if _, err := normalizeRedirectURI("https://host.example/cb?x=1"); err == nil {
		t.Fatalf("expected error when redirect uri has query")
	}
}

func TestAuthorize_InvalidRedirectURI(t *testing.T) {
	t.Parallel()

	_, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:      []string{"s1"},
		Manual:      true,
		RedirectURI: "host-only/path",
	})
	if err == nil {
		t.Fatalf("expected invalid redirect uri error")
	}

	if !strings.Contains(err.Error(), "parse redirect uri") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeListenAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "127.0.0.1:0"},
		{in: "0.0.0.0", want: "0.0.0.0:0"},
		{in: "0.0.0.0:8080", want: "0.0.0.0:8080"},
		{in: "[::1]", want: "[::1]:0"},
		{in: "[::1]:9090", want: "[::1]:9090"},
	}
	for _, tt := range tests {
		got, err := normalizeListenAddr(tt.in)
		if err != nil {
			t.Fatalf("normalizeListenAddr(%q): %v", tt.in, err)
		}

		if got != tt.want {
			t.Fatalf("normalizeListenAddr(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}

	if _, err := normalizeListenAddr("::1"); err == nil {
		t.Fatalf("expected raw IPv6 without brackets to be rejected")
	}
}

func TestResolveServerRedirectURI(t *testing.T) {
	t.Parallel()

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	got := resolveServerRedirectURI(ln, "https://host.example/oauth2/callback")
	if got != "https://host.example/oauth2/callback" {
		t.Fatalf("unexpected redirect override: %q", got)
	}

	got = resolveServerRedirectURI(ln, "")
	if !strings.Contains(got, "127.0.0.1:") {
		t.Fatalf("expected local listener redirect, got %q", got)
	}
}
