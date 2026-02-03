package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/forms/v1"
	"google.golang.org/api/option"
)

func TestFormsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{
				{"id": "f1", "name": "Survey", "createdTime": "2026-01-01T00:00:00Z", "owners": []map[string]any{{"emailAddress": "owner@example.com"}}},
			},
		})
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Survey") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestFormsCreateCmd(t *testing.T) {
	var gotTitle string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v1/forms") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Info struct {
				Title string `json:"title"`
			} `json:"info"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotTitle = payload.Info.Title
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"formId": "form-1",
			"info":   map[string]any{"title": payload.Info.Title},
		})
	})
	stubForms(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsCreateCmd{Title: "Survey"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotTitle != "Survey" {
		t.Fatalf("unexpected title: %q", gotTitle)
	}
	if !strings.Contains(out, "Created form") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubForms(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newFormsService
	svc, err := forms.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new forms service: %v", err)
	}
	newFormsService = func(context.Context, string) (*forms.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newFormsService = orig
		srv.Close()
	})
	return srv
}

func stubDrive(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newDriveService
	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new drive service: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newDriveService = orig
		srv.Close()
	})
	return srv
}
