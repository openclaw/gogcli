//nolint:wsl_v5
package zoom

import (
	"net/url"
	"os"
	"strings"
)

const includePasswordsEnv = "GOG_ZOOM_INCLUDE_PASSWORDS" //nolint:gosec // env var name, not a credential value.

func IncludePasswordsFromEnv() bool {
	return os.Getenv(includePasswordsEnv) == "1"
}

func RedactZoomURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return redactZoomPasswordFallback(raw)
	}
	q := u.Query()
	if _, ok := q["pwd"]; !ok {
		return raw
	}
	q.Set("pwd", "REDACTED")
	u.RawQuery = q.Encode()
	return u.String()
}

func redactZoomPasswordFallback(raw string) string {
	const key = "pwd="
	i := strings.Index(raw, key)
	if i < 0 {
		return raw
	}
	start := i + len(key)
	end := len(raw)
	if j := strings.IndexAny(raw[start:], "&#"); j >= 0 {
		end = start + j
	}
	return raw[:start] + "REDACTED" + raw[end:]
}
