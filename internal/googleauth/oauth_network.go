package googleauth

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

var errInvalidListenAddr = errors.New("invalid listen address; use host or host:port")

func normalizeListenAddr(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "127.0.0.1:0", nil
	}

	if _, _, err := net.SplitHostPort(raw); err == nil {
		return raw, nil
	}

	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		return raw + ":0", nil
	}

	if strings.Count(raw, ":") == 0 {
		return net.JoinHostPort(raw, "0"), nil
	}

	return "", fmt.Errorf("%w: %q", errInvalidListenAddr, raw)
}

func redirectURIFromListener(ln net.Listener) string {
	port := ln.Addr().(*net.TCPAddr).Port
	return fmt.Sprintf("http://127.0.0.1:%d/oauth2/callback", port)
}

func resolveServerRedirectURI(ln net.Listener, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}

	return redirectURIFromListener(ln)
}
