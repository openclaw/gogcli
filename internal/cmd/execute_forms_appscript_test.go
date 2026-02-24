package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	formsapi "google.golang.org/api/forms/v1"
	"google.golang.org/api/option"
	scriptapi "google.golang.org/api/script/v1"
)

func TestExecute_FormsGet_Text(t *testing.T) {
	origNew := newFormsService
	t.Cleanup(func() { newFormsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/forms/form123") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"formId":       "form123",
			"responderUri": "https://docs.google.com/forms/d/e/resp",
			"info": map[string]any{
				"title":       "Survey",
				"description": "Weekly check-in",
			},
		})
	}))
	defer srv.Close()

	svc, err := formsapi.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newFormsService = func(context.Context, string) (*formsapi.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "forms", "get", "form123"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "id\tform123") || !strings.Contains(out, "title\tSurvey") || !strings.Contains(out, "edit_url\thttps://docs.google.com/forms/d/form123/edit") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_FormsResponsesList_JSON(t *testing.T) {
	origNew := newFormsService
	t.Cleanup(func() { newFormsService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/forms/form123/responses") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"responses": []map[string]any{
				{
					"responseId":        "r1",
					"lastSubmittedTime": "2026-02-14T00:00:00Z",
					"respondentEmail":   "user@example.com",
				},
			},
			"nextPageToken": "next123",
		})
	}))
	defer srv.Close()

	svc, err := formsapi.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newFormsService = func(context.Context, string) (*formsapi.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "forms", "responses", "list", "form123", "--max", "1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		FormID    string `json:"form_id"`
		Responses []struct {
			ResponseID string `json:"responseId"`
		} `json:"responses"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.FormID != "form123" || len(parsed.Responses) != 1 || parsed.Responses[0].ResponseID != "r1" || parsed.NextPageToken != "next123" {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
}

func TestExecute_FormsResponsesList_RejectsNonPositiveMax(t *testing.T) {
	origNew := newFormsService
	t.Cleanup(func() { newFormsService = origNew })
	newFormsService = func(context.Context, string) (*formsapi.Service, error) {
		t.Fatalf("expected validation to fail before creating forms service")
		return nil, errors.New("unexpected forms service call")
	}

	_ = captureStderr(t, func() {
		err := Execute([]string{"--account", "a@b.com", "forms", "responses", "list", "form123", "--max", "0"})
		if err == nil || !strings.Contains(err.Error(), "--max must be > 0") {
			t.Fatalf("unexpected err: %v", err)
		}
	})
}

func TestExecute_FormsCreate_DryRun_JSON(t *testing.T) {
	origNew := newFormsService
	t.Cleanup(func() { newFormsService = origNew })
	newFormsService = func(context.Context, string) (*formsapi.Service, error) {
		t.Fatalf("dry-run should not create forms service")
		return nil, errors.New("unexpected forms service call")
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--dry-run",
				"--account", "a@b.com",
				"forms", "create",
				"--title", "Weekly Check-in",
				"--description", "Friday async update",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !parsed.DryRun || parsed.Op != "forms.create" {
		t.Fatalf("unexpected dry-run payload: %#v", parsed)
	}
	if parsed.Request.Title != "Weekly Check-in" || parsed.Request.Description != "Friday async update" {
		t.Fatalf("unexpected request payload: %#v", parsed.Request)
	}
}

func TestExecute_AppScriptRun_JSON(t *testing.T) {
	origNew := newAppScriptService
	t.Cleanup(func() { newAppScriptService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/scripts/script123:run") && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["function"] != "myFunc" {
			t.Fatalf("unexpected function: %#v", req["function"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"done": true,
			"response": map[string]any{
				"result": map[string]any{
					"ok": true,
				},
			},
		})
	}))
	defer srv.Close()

	svc, err := scriptapi.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAppScriptService = func(context.Context, string) (*scriptapi.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "appscript", "run", "script123", "myFunc", "--params", "[\"x\"]"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	op, ok := parsed["operation"].(map[string]any)
	if !ok {
		t.Fatalf("missing operation: %#v", parsed)
	}
	if done, _ := op["done"].(bool); !done {
		t.Fatalf("expected done=true: %#v", op)
	}
}

func TestExecute_AppScriptCreate_DryRun_Text(t *testing.T) {
	origNew := newAppScriptService
	t.Cleanup(func() { newAppScriptService = origNew })
	newAppScriptService = func(context.Context, string) (*scriptapi.Service, error) {
		t.Fatalf("dry-run should not create appscript service")
		return nil, errors.New("unexpected appscript service call")
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--dry-run",
				"--account", "a@b.com",
				"appscript", "create",
				"--title", "Automation Helpers",
				"--parent-id", "drive123",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Dry run: would appscript.create") ||
		!strings.Contains(out, `"title": "Automation Helpers"`) ||
		!strings.Contains(out, `"parent_id": "drive123"`) {
		t.Fatalf("unexpected dry-run out=%q", out)
	}
}

func TestExecute_AppScriptRun_RejectsNonArrayParams(t *testing.T) {
	origNew := newAppScriptService
	t.Cleanup(func() { newAppScriptService = origNew })
	newAppScriptService = func(context.Context, string) (*scriptapi.Service, error) {
		t.Fatalf("expected params validation to fail before creating appscript service")
		return nil, errors.New("unexpected appscript service call")
	}

	_ = captureStderr(t, func() {
		err := Execute([]string{"--account", "a@b.com", "appscript", "run", "script123", "myFunc", "--params", `{"x":1}`})
		if err == nil || !strings.Contains(err.Error(), "invalid --params JSON array") {
			t.Fatalf("unexpected err: %v", err)
		}
	})
}

func TestExecute_AppScriptRun_TextErrorDetails(t *testing.T) {
	origNew := newAppScriptService
	t.Cleanup(func() { newAppScriptService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/scripts/script123:run") && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"done": true,
			"error": map[string]any{
				"code":    3,
				"message": "Script execution failed",
				"details": []map[string]any{
					{
						"@type":        "type.googleapis.com/google.apps.script.type.ExecutionError",
						"errorType":    "TypeError",
						"errorMessage": "boom",
					},
				},
			},
		})
	}))
	defer srv.Close()

	svc, err := scriptapi.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newAppScriptService = func(context.Context, string) (*scriptapi.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "appscript", "run", "script123", "myFunc"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "done\ttrue") ||
		!strings.Contains(out, "error_code\t3") ||
		!strings.Contains(out, "error\tScript execution failed") ||
		!strings.Contains(out, "error_type\tTypeError") ||
		!strings.Contains(out, "error_message\tboom") {
		t.Fatalf("unexpected out=%q", out)
	}
}
