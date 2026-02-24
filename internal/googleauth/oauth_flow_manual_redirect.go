package googleauth

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

func randomManualRedirectURI(ctx context.Context) (string, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen for manual redirect port: %w", err)
	}

	defer func() { _ = ln.Close() }()

	port := ln.Addr().(*net.TCPAddr).Port

	return fmt.Sprintf("http://127.0.0.1:%d/oauth2/callback", port), nil
}

func redirectURIFromParsedURL(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}

	return fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, path)
}

func parseRedirectURL(rawURL string) (code string, state string, redirectURI string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", "", fmt.Errorf("parse redirect url: %w", err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return "", "", "", fmt.Errorf("parse redirect url: %w", errInvalidRedirectURL)
	}

	redirectURI = redirectURIFromParsedURL(parsed)

	code = parsed.Query().Get("code")
	if code == "" {
		return "", "", "", errNoCodeInURL
	}
	state = parsed.Query().Get("state")

	return code, state, redirectURI, nil
}

func extractCodeAndState(rawURL string) (code string, state string, err error) {
	code, state, _, err = parseRedirectURL(rawURL)
	return code, state, err
}
