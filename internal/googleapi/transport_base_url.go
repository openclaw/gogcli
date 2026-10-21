package googleapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const apiBaseURLEnv = "GOG_GOOGLE_API_BASE_URL"

type apiBaseURLContextKey struct{}

var (
	errInvalidAPIBaseURL   = errors.New("want http(s)://host[:port] without path, query, or credentials")
	errCrossOriginRedirect = errors.New("refuse cross-origin redirect from " + apiBaseURLEnv + " request")
)

// WithAPIBaseURL validates a GOG_GOOGLE_API_BASE_URL value and stores it in
// ctx. A blank value leaves ctx unchanged. Path prefixes are rejected so the
// original request path is forwarded unchanged.
func WithAPIBaseURL(ctx context.Context, raw string) (context.Context, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ctx, nil
	}

	// The value may hold credentials, so errors never repeat it.
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return ctx, fmt.Errorf("invalid %s: %w", apiBaseURLEnv, errInvalidAPIBaseURL)
	}

	return context.WithValue(ctx, apiBaseURLContextKey{}, &url.URL{Scheme: u.Scheme, Host: u.Host}), nil
}

// WithAPIBaseURLFrom copies the GOG_GOOGLE_API_BASE_URL setting of src into
// ctx, for clients built from contexts that do not derive from the command.
func WithAPIBaseURLFrom(ctx, src context.Context) context.Context {
	if src == nil {
		return ctx
	}

	if baseURL, _ := src.Value(apiBaseURLContextKey{}).(*url.URL); baseURL != nil {
		return context.WithValue(ctx, apiBaseURLContextKey{}, baseURL)
	}

	return ctx
}

// APIBaseURLEnabled reports whether GOG_GOOGLE_API_BASE_URL is set, in which
// case gog needs no credentials of its own.
func APIBaseURLEnabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}

	baseURL, _ := ctx.Value(apiBaseURLContextKey{}).(*url.URL)

	return baseURL != nil
}

// apiBaseURLHTTPClient returns the client used for every Google API request
// while GOG_GOOGLE_API_BASE_URL is set. It has no credential provider: the
// relay or proxy behind the base URL adds credentials, so gog never sends
// OAuth tokens, service-account tokens, or API keys there.
func apiBaseURLHTTPClient(ctx context.Context) (*http.Client, bool) {
	if ctx == nil {
		return nil, false
	}

	baseURL, _ := ctx.Value(apiBaseURLContextKey{}).(*url.URL)
	if baseURL == nil {
		return nil, false
	}

	transport := &apiBaseURLTransport{base: quotaProjectTransportFromContext(ctx, newBaseTransport()), baseURL: baseURL}

	return &http.Client{Transport: readOnlyTransportFromContext(ctx, NewRetryTransport(transport))}, true
}

// apiBaseURLTransport sends https requests for googleapis.com and its
// subdomains to the configured base URL, keeping path and query. The
// original host goes in X-Forwarded-Host because many APIs share paths
// such as /v1/... across hosts. Redirects are refused unless they stay on
// the origin of the request that received them; callers may replace
// http.Client.CheckRedirect, so the check lives here.
type apiBaseURLTransport struct {
	base    http.RoundTripper
	baseURL *url.URL
}

func (t *apiBaseURLTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if prev := req.Response; prev != nil && prev.Request != nil && !sameOrigin(prev.Request.URL, req.URL) {
		if req.Body != nil {
			_ = req.Body.Close()
		}

		return nil, fmt.Errorf("%w: %s", errCrossOriginRedirect, req.URL.Redacted())
	}

	if !isGoogleAPIsURL(req.URL) {
		return t.base.RoundTrip(req) //nolint:wrapcheck // pass-through transport
	}

	out := req.Clone(req.Context())
	out.Header.Del("Authorization")
	out.Header.Set("X-Forwarded-Host", req.URL.Host)
	out.URL.Scheme = t.baseURL.Scheme
	out.URL.Host = t.baseURL.Host
	out.Host = t.baseURL.Host

	resp, err := t.base.RoundTrip(out)
	if resp != nil {
		// Callers see the Google URL they asked for. A redirect to the base
		// URL's own origin is mapped back to that Google origin, so it is
		// rewritten again and read-only and Discovery redirect checks still
		// see a Google URL.
		resp.Request = req
		if loc, locErr := resp.Location(); locErr == nil && sameOrigin(t.baseURL, loc) {
			loc.Scheme, loc.Host = req.URL.Scheme, req.URL.Host
			resp.Header.Set("Location", loc.String())
		}
	}

	return resp, err //nolint:wrapcheck // pass-through transport
}

func isGoogleAPIsURL(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())

	return u.Scheme == "https" && (host == "googleapis.com" || strings.HasSuffix(host, ".googleapis.com"))
}

func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && strings.EqualFold(a.Host, b.Host)
}
