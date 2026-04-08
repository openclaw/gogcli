package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type gmailRawHit struct {
	lastFormat atomic.Value // string
}

func newGmailRawTestServer(t *testing.T, status int, body map[string]any, hit *gmailRawHit) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/users/me/messages/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if hit != nil {
			hit.lastFormat.Store(r.URL.Query().Get("format"))
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

func installMockGmailService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := newGmailService
	t.Cleanup(func() { newGmailService = orig })

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }
}

func fullGmailMessageResponse(id string) map[string]any {
	return map[string]any{
		"id":       id,
		"threadId": "t1",
		"labelIds": []string{"INBOX"},
		"snippet":  "hello world",
		"payload": map[string]any{
			"mimeType": "text/plain",
			"headers": []map[string]any{
				{"name": "From", "value": "a@b.com"},
				{"name": "Subject", "value": "hi"},
			},
		},
	}
}

func TestGmailRaw_HappyPath_DefaultFormatFull(t *testing.T) {
	hit := &gmailRawHit{}
	srv := newGmailRawTestServer(t, 0, fullGmailMessageResponse("m1"), hit)
	defer srv.Close()
	installMockGmailService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		if err := runKong(t, &GmailRawCmd{}, []string{"m1"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	if got, _ := hit.lastFormat.Load().(string); got != "full" {
		t.Fatalf("expected default format=full, got: %q", got)
	}

	var fileOut map[string]any
	if err := json.Unmarshal([]byte(out), &fileOut); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if fileOut["id"] != "m1" {
		t.Fatalf("expected id=m1, got: %v", fileOut["id"])
	}
	// Bare struct — no wrapper.
	if _, wrapped := fileOut["message"]; wrapped {
		t.Fatalf("raw output must not be wrapped, got: %v", fileOut)
	}
}

func TestGmailRaw_FormatPropagation(t *testing.T) {
	for _, fmt := range []string{"full", "metadata", "minimal", "raw"} {
		t.Run(fmt, func(t *testing.T) {
			hit := &gmailRawHit{}
			srv := newGmailRawTestServer(t, 0, fullGmailMessageResponse("m1"), hit)
			defer srv.Close()
			installMockGmailService(t, srv)

			ctx := rawTestContext(t)
			flags := &RootFlags{Account: "a@b.com"}
			_ = captureStdout(t, func() {
				if err := runKong(t, &GmailRawCmd{}, []string{"m1", "--format", fmt}, ctx, flags); err != nil {
					t.Fatalf("run: %v", err)
				}
			})
			if got, _ := hit.lastFormat.Load().(string); got != fmt {
				t.Fatalf("expected format=%s in request, got: %q", fmt, got)
			}
		})
	}
}

func TestGmailRaw_InvalidFormat(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	err := (&GmailRawCmd{MessageID: "m1", Format: "bogus"}).Run(ctx, flags)
	if err == nil {
		t.Fatalf("expected error on invalid format")
	}
}

func TestGmailRaw_APIError(t *testing.T) {
	srv := newGmailRawTestServer(t, http.StatusInternalServerError, nil, nil)
	defer srv.Close()
	installMockGmailService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &GmailRawCmd{}, []string{"m1"}, ctx, flags); err == nil {
			t.Fatalf("expected error on 500")
		}
	})
}

func TestGmailRaw_NotFound(t *testing.T) {
	srv := newGmailRawTestServer(t, http.StatusNotFound, nil, nil)
	defer srv.Close()
	installMockGmailService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &GmailRawCmd{}, []string{"m1"}, ctx, flags); err == nil {
			t.Fatalf("expected error on 404")
		}
	})
}

func TestGmailRaw_EmptyID(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	if err := (&GmailRawCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error on empty id")
	}
}
