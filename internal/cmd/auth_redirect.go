package cmd

import (
	"fmt"
	"strings"
)

func redirectURIFromHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", usage("empty redirect host")
	}
	return fmt.Sprintf("https://%s/oauth2/callback", host), nil
}
