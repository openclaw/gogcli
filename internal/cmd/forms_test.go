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

	"github.com/steipete/gogcli/internal/outfmt"
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

func TestFormsListCmd_JSON(t *testing.T) {
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

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "files") || !strings.Contains(out, "Survey") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestFormsListCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{},
		})
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsListCmd{}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestFormsListCmd_Pagination(t *testing.T) {
	var gotPageToken, gotPageSize string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}
		gotPageToken = r.URL.Query().Get("pageToken")
		gotPageSize = r.URL.Query().Get("pageSize")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{
				{"id": "f1", "name": "Form 1", "createdTime": "2026-01-01T00:00:00Z", "owners": []map[string]any{{"emailAddress": "owner@example.com"}}},
			},
			"nextPageToken": "next-token",
		})
	})
	stubDrive(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsListCmd{Max: 10, Page: "token123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPageToken != "token123" {
		t.Errorf("expected page token 'token123', got %q", gotPageToken)
	}
	if gotPageSize != "10" {
		t.Errorf("expected page size '10', got %q", gotPageSize)
	}
	if !strings.Contains(out, "Form 1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestFormsListCmd_WithUser(t *testing.T) {
	var usedAccount string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/files") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{
				{"id": "f1", "name": "User Form", "createdTime": "2026-01-01T00:00:00Z", "owners": []map[string]any{{"emailAddress": "user2@example.com"}}},
			},
		})
	})

	srv := httptest.NewServer(h)
	orig := newDriveService
	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new drive service: %v", err)
	}
	newDriveService = func(_ context.Context, account string) (*drive.Service, error) {
		usedAccount = account
		return svc, nil
	}
	t.Cleanup(func() {
		newDriveService = orig
		srv.Close()
	})

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &FormsListCmd{User: "user2@example.com"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if usedAccount != "user2@example.com" {
		t.Errorf("expected account 'user2@example.com', got %q", usedAccount)
	}
	if !strings.Contains(out, "User Form") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestFormsGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/forms/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"formId":       "form-1",
			"info":         map[string]any{"title": "Test Form"},
			"responderUri": "https://docs.google.com/forms/d/e/form-1/viewform",
		})
	})
	stubForms(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsGetCmd{FormID: "form-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Test Form") || !strings.Contains(out, "ID:") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestFormsGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/forms/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"formId": "form-1",
			"info":   map[string]any{"title": "Test Form"},
		})
	})
	stubForms(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsGetCmd{FormID: "form-1"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "formId") || !strings.Contains(out, "form-1") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestFormsGetCmd_EmptyID(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsGetCmd{FormID: "   "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty form ID")
	}
	if !strings.Contains(err.Error(), "form ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormsGetCmd_NoTitle(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1/forms/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"formId": "form-1",
		})
	})
	stubForms(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsGetCmd{FormID: "form-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "ID:") || !strings.Contains(out, "form-1") {
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

func TestFormsCreateCmd_JSON(t *testing.T) {
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
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"formId": "form-1",
			"info":   map[string]any{"title": payload.Info.Title},
		})
	})
	stubForms(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsCreateCmd{Title: "Survey"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "formId") || !strings.Contains(out, "form-1") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestFormsCreateCmd_MissingTitle(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsCreateCmd{Title: ""}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
	if !strings.Contains(err.Error(), "--title is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormsCreateCmd_WhitespaceTitle(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsCreateCmd{Title: "   "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for whitespace-only title")
	}
	if !strings.Contains(err.Error(), "--title is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormsResponsesCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/responses") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"responses": []map[string]any{
				{
					"responseId":        "resp-1",
					"respondentEmail":   "respondent@example.com",
					"createTime":        "2026-01-01T10:00:00Z",
					"lastSubmittedTime": "2026-01-01T10:05:00Z",
				},
			},
		})
	})
	stubForms(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsResponsesCmd{FormID: "form-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "resp-1") || !strings.Contains(out, "respondent@example.com") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestFormsResponsesCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/responses") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"responses": []map[string]any{
				{
					"responseId":        "resp-1",
					"respondentEmail":   "respondent@example.com",
					"createTime":        "2026-01-01T10:00:00Z",
					"lastSubmittedTime": "2026-01-01T10:05:00Z",
				},
			},
		})
	})
	stubForms(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsResponsesCmd{FormID: "form-1"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "responses") || !strings.Contains(out, "resp-1") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestFormsResponsesCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/responses") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"responses": []map[string]any{},
		})
	})
	stubForms(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsResponsesCmd{FormID: "form-1"}

	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestFormsResponsesCmd_EmptyFormID(t *testing.T) {
	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsResponsesCmd{FormID: "   "}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error for empty form ID")
	}
	if !strings.Contains(err.Error(), "form ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormsResponsesCmd_Pagination(t *testing.T) {
	var gotPageToken, gotPageSize string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/responses") {
			http.NotFound(w, r)
			return
		}
		gotPageToken = r.URL.Query().Get("pageToken")
		gotPageSize = r.URL.Query().Get("pageSize")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"responses": []map[string]any{
				{
					"responseId":        "resp-1",
					"respondentEmail":   "user1@example.com",
					"createTime":        "2026-01-01T10:00:00Z",
					"lastSubmittedTime": "2026-01-01T10:05:00Z",
				},
			},
			"nextPageToken": "next-token",
		})
	})
	stubForms(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &FormsResponsesCmd{FormID: "form-1", Max: 5, Page: "token-abc"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotPageToken != "token-abc" {
		t.Errorf("expected page token 'token-abc', got %q", gotPageToken)
	}
	if gotPageSize != "5" {
		t.Errorf("expected page size '5', got %q", gotPageSize)
	}
	if !strings.Contains(out, "resp-1") {
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
