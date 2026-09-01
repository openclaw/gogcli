package googleapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/openclaw/gogcli/internal/authclient"
)

// Some Google APIs reject ADC and raw access tokens that carry no quota project.
const quotaProjectHeader = "X-Goog-User-Project"

type quotaProjectTransport struct {
	base    http.RoundTripper
	project string
}

func quotaProjectTransportFromContext(ctx context.Context, base http.RoundTripper) http.RoundTripper {
	project := authclient.QuotaProjectFromContext(ctx)
	if project == "" {
		return base
	}

	slog.Debug("using quota project", "project", project)

	if base == nil {
		base = http.DefaultTransport
	}

	return &quotaProjectTransport{base: base, project: project}
}

func (t *quotaProjectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get(quotaProjectHeader) == "" {
		req = req.Clone(req.Context())
		req.Header.Set(quotaProjectHeader, t.project)
	}

	return t.base.RoundTrip(req) //nolint:wrapcheck // wrapping would blame quota-project wiring for unrelated failures
}
