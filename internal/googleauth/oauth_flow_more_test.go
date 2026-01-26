package googleauth

import (
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
