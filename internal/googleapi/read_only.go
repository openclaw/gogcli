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
	if !readOnlyHTTPRequest(request) {
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

func readOnlyHTTPRequest(request *http.Request) bool {
	if request == nil || request.URL == nil {
		return false
	}

	switch request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	case http.MethodPost:
		return readOnlyPOSTPath(request.URL.Path)
	default:
		return false
	}
}

func readOnlyPOSTPath(path string) bool {
	if path == "" {
		return false
	}

	for _, suffix := range []string{
		"/freeBusy",
		"/searchAnalytics/query",
		"/urlInspection/index:inspect",
		"/mediaItems:search",
		":batchGetByDataFilter",
		":getByDataFilter",
		":query",
		":runReport",
		":batchRunReports",
		":runPivotReport",
		":runRealtimeReport",
	} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}

	return false
}
