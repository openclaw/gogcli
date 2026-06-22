package googleapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var ErrReadOnly = errors.New("request blocked by --readonly")

type readOnlyContextKey struct{}

func WithReadOnly(ctx context.Context, enabled bool) context.Context {
	if !enabled {
		return ctx
	}

	return context.WithValue(ctx, readOnlyContextKey{}, true)
}

func ReadOnly(ctx context.Context) bool {
	if ctx == nil {
		return false
	}

	enabled, _ := ctx.Value(readOnlyContextKey{}).(bool)

	return enabled
}

type readOnlyTransport struct {
	base http.RoundTripper
}

func readOnlyTransportFromContext(ctx context.Context, base http.RoundTripper) http.RoundTripper {
	if !ReadOnly(ctx) {
		return base
	}

	if base == nil {
		base = http.DefaultTransport
	}

	return &readOnlyTransport{base: base}
}

func (t *readOnlyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if !ReadOnlyRequestAllowed(request) {
		method := ""
		path := ""

		if request != nil {
			method = request.Method
			if request.URL != nil {
				path = request.URL.Path
			}
		}

		return nil, fmt.Errorf("%w: %s %s", ErrReadOnly, method, path)
	}

	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, fmt.Errorf("read-only transport: %w", err)
	}

	return response, nil
}

// ReadOnlyRequestAllowed reports whether request is safe under runtime read-only enforcement.
func ReadOnlyRequestAllowed(request *http.Request) bool {
	if request == nil || request.URL == nil {
		return false
	}

	if strings.TrimSpace(request.Header.Get("X-HTTP-Method-Override")) != "" {
		return false
	}

	switch request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	case http.MethodPost:
		return readOnlyPOSTRequest(request)
	default:
		return false
	}
}

func readOnlyPOSTRequest(request *http.Request) bool {
	if request == nil || request.URL == nil || request.URL.Scheme != "https" {
		return false
	}

	host := strings.ToLower(strings.TrimSuffix(request.URL.Hostname(), "."))
	path := request.URL.Path

	switch host {
	case "www.googleapis.com", "www.mtls.googleapis.com", "calendar-json.googleapis.com", "calendar-json.mtls.googleapis.com":
		return strings.HasSuffix(path, "/calendar/v3/freeBusy")
	case "searchconsole.googleapis.com", "searchconsole.mtls.googleapis.com":
		return strings.HasSuffix(path, "/searchAnalytics/query") || strings.HasSuffix(path, "/urlInspection/index:inspect")
	case "photoslibrary.googleapis.com", "photoslibrary.mtls.googleapis.com":
		return strings.HasSuffix(path, "/v1/mediaItems:search")
	case "sheets.googleapis.com", "sheets.mtls.googleapis.com":
		return strings.HasSuffix(path, ":batchGetByDataFilter") || strings.HasSuffix(path, ":getByDataFilter")
	case "driveactivity.googleapis.com", "driveactivity.mtls.googleapis.com":
		return strings.HasSuffix(path, "/v2/activity:query")
	case "analyticsdata.googleapis.com", "analyticsdata.mtls.googleapis.com":
		return strings.HasSuffix(path, ":runReport") ||
			strings.HasSuffix(path, ":batchRunReports") ||
			strings.HasSuffix(path, ":runPivotReport") ||
			strings.HasSuffix(path, ":runRealtimeReport")
	default:
		return false
	}
}
