package gmailwatch

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
)

type AuthConfig struct {
	VerifyOIDC   bool
	OIDCEmail    string
	OIDCAudience string
	SharedToken  string
}

type OIDCVerifier func(context.Context, string, string, string) (bool, error)

type Authorizer struct {
	Config AuthConfig
	Verify OIDCVerifier
	Warnf  func(string, ...any)
}

func (a *Authorizer) Authorize(request *http.Request) bool {
	if a.Config.VerifyOIDC {
		bearer := BearerToken(request)
		if bearer != "" && a.Verify != nil {
			if ok, err := a.Verify(request.Context(), bearer, Audience(request, a.Config.OIDCAudience), a.Config.OIDCEmail); ok {
				return true
			} else if err != nil {
				a.warnf("watch: oidc verify failed: %v", err)
			}
		}

		if a.Config.SharedToken != "" {
			return SharedTokenMatches(request, a.Config.SharedToken)
		}

		return false
	}

	if a.Config.SharedToken == "" {
		return true
	}

	return SharedTokenMatches(request, a.Config.SharedToken)
}

func Audience(request *http.Request, explicit string) string {
	if explicit != "" {
		return explicit
	}

	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}

	if forwarded := firstForwardedValue(request.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}

	host := request.Host
	if forwarded := firstForwardedValue(request.Header.Get("X-Forwarded-Host")); forwarded != "" {
		host = forwarded
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, request.URL.Path)
}

func BearerToken(request *http.Request) string {
	authorization := request.Header.Get("Authorization")
	if authorization == "" {
		return ""
	}

	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

func SharedTokenMatches(request *http.Request, expected string) bool {
	if expected == "" {
		return false
	}

	token := request.Header.Get("x-gog-token")
	if token == "" {
		token = request.URL.Query().Get("token")
	}

	if token == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func firstForwardedValue(raw string) string {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 {
		return ""
	}

	return strings.TrimSpace(parts[0])
}

func (a *Authorizer) warnf(format string, args ...any) {
	if a.Warnf != nil {
		a.Warnf(format, args...)
	}
}
