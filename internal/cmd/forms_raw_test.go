package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	formsapi "google.golang.org/api/forms/v1"
)

func newFormsRawTestServer(t *testing.T, status int, body map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/forms/") || r.Method != http.MethodGet {
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

func installMockFormsService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", formsapi.NewService)
	stubGoogleTestService(t, &newFormsService, svc)
}

func fullFormResponse(id string) map[string]any {
	return map[string]any{
		"formId": id,
		"info": map[string]any{
			"title":       "Survey",
			"description": "Tell us what you think",
		},
		"items": []map[string]any{
			{"itemId": "q1", "title": "How are you?"},
		},
	}
}

func TestFormsRaw_HappyPath(t *testing.T) {
	srv := newFormsRawTestServer(t, 0, fullFormResponse("form1"))
	defer srv.Close()
	installMockFormsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		if err := runKong(t, &FormsRawCmd{}, []string{"form1"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if got["formId"] != "form1" {
		t.Fatalf("expected formId=form1, got: %v", got["formId"])
	}
	if _, ok := got["items"]; !ok {
		t.Fatalf("expected items in raw output")
	}
}

func TestFormsRaw_APIError(t *testing.T) {
	srv := newFormsRawTestServer(t, http.StatusInternalServerError, nil)
	defer srv.Close()
	installMockFormsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &FormsRawCmd{}, []string{"form1"}, ctx, flags); err == nil {
			t.Fatalf("expected error on 500")
		}
	})
}

func TestFormsRaw_NotFound(t *testing.T) {
	srv := newFormsRawTestServer(t, http.StatusNotFound, nil)
	defer srv.Close()
	installMockFormsService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &FormsRawCmd{}, []string{"form1"}, ctx, flags); err == nil {
			t.Fatalf("expected error on 404")
		}
	})
}

func TestFormsRaw_EmptyID(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	if err := (&FormsRawCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error on empty id")
	}
}
