package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/slides/v1"
)

func newSlidesRawTestServer(t *testing.T, status int, body map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/slides/v1")
		path = strings.TrimPrefix(path, "/v1")
		if !strings.HasPrefix(path, "/presentations/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": status, "message": "mock error"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func newMockSlidesService(t *testing.T, srv *httptest.Server) *slides.Service {
	t.Helper()
	return newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", slides.NewService)
}

func fullPresentationResponse(id string) map[string]any {
	return map[string]any{
		"presentationId": id,
		"title":          "Full Deck",
		"slides": []map[string]any{
			{
				"objectId": "slide1",
				"pageElements": []map[string]any{
					{
						"objectId": "e1",
						"shape": map[string]any{
							"shapeType": "TEXT_BOX",
						},
					},
				},
			},
		},
		"masters": []map[string]any{{"objectId": "master1"}},
		"layouts": []map[string]any{{"objectId": "layout1"}},
	}
}

func TestSlidesRaw_HappyPath(t *testing.T) {
	srv := newSlidesRawTestServer(t, 0, fullPresentationResponse("p1"))
	defer srv.Close()

	var out bytes.Buffer
	ctx := withSlidesTestService(
		newCmdRuntimeOutputContext(t, &out, io.Discard),
		newMockSlidesService(t, srv),
	)
	ctx = withRawTestHTTP(t, ctx, srv.URL)
	flags := &RootFlags{Account: "a@b.com"}
	if err := runKong(t, &SlidesRawCmd{}, []string{"p1"}, ctx, flags); err != nil {
		t.Fatalf("run: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out.String())
	}
	if got["presentationId"] != "p1" {
		t.Fatalf("expected presentationId=p1, got: %v", got["presentationId"])
	}
	if _, ok := got["slides"]; !ok {
		t.Fatalf("expected slides in raw output")
	}
}

func TestSlidesRaw_APIError(t *testing.T) {
	srv := newSlidesRawTestServer(t, http.StatusInternalServerError, nil)
	defer srv.Close()

	ctx := withSlidesTestService(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		newMockSlidesService(t, srv),
	)
	ctx = withRawTestHTTP(t, ctx, srv.URL)
	flags := &RootFlags{Account: "a@b.com"}
	if err := runKong(t, &SlidesRawCmd{}, []string{"p1"}, ctx, flags); err == nil {
		t.Fatalf("expected error on 500")
	}
}

func TestSlidesRaw_NotFound(t *testing.T) {
	srv := newSlidesRawTestServer(t, http.StatusNotFound, nil)
	defer srv.Close()

	ctx := withSlidesTestService(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		newMockSlidesService(t, srv),
	)
	ctx = withRawTestHTTP(t, ctx, srv.URL)
	flags := &RootFlags{Account: "a@b.com"}
	if err := runKong(t, &SlidesRawCmd{}, []string{"p1"}, ctx, flags); err == nil {
		t.Fatalf("expected error on 404")
	}
}

func TestSlidesRaw_EmptyID(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	if err := (&SlidesRawCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error on empty id")
	}
}
