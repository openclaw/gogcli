package cmd

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func validateGmailBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid GOG_GMAIL_BASE_URL: must be an absolute URL")
	}

	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return value, nil
	case "http":
		hostname := parsed.Hostname()
		ip := net.ParseIP(hostname)
		if strings.EqualFold(hostname, "localhost") || ip != nil && ip.IsLoopback() {
			return value, nil
		}
	}

	return "", fmt.Errorf("invalid GOG_GMAIL_BASE_URL: must use HTTPS or loopback HTTP")
}
