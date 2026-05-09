package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	formsapi "google.golang.org/api/forms/v1"
)

func TestFormsPublishCmd(t *testing.T) {
	origNew := newFormsService
	t.Cleanup(func() { newFormsService = origNew })

	var gotPublish formsapi.SetPublishSettingsRequest
	var gotPublishJSON map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/forms/form1:setPublishSettings"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read setPublishSettings: %v", err)
			}
			if err := json.Unmarshal(body, &gotPublish); err != nil {
				t.Fatalf("decode setPublishSettings: %v", err)
			}
			if err := json.Unmarshal(body, &gotPublishJSON); err != nil {
				t.Fatalf("decode setPublishSettings JSON: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"publishSettings": map[string]any{
					"publishState": map[string]any{
						"isPublished":          true,
						"isAcceptingResponses": true,
					},
				},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/forms/form1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"formId":       "form1",
				"responderUri": "https://docs.google.com/forms/d/e/form1/viewform",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	newFormsService = func(ctx context.Context, account string) (*formsapi.Service, error) {
		return newFormsTestService(t, ctx, srv), nil
	}

	err := runKong(t, &FormsPublishCmd{}, []string{"form1"}, newQuietUIContext(t), &RootFlags{Account: "a@b.com"})
	if err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotPublish.UpdateMask != "publish_state" {
		t.Fatalf("UpdateMask = %q, want publish_state", gotPublish.UpdateMask)
	}
	if gotPublish.PublishSettings == nil || gotPublish.PublishSettings.PublishState == nil {
		t.Fatalf("missing publish state: %#v", gotPublish.PublishSettings)
	}
	state := gotPublish.PublishSettings.PublishState
	if !state.IsPublished || !state.IsAcceptingResponses {
		t.Fatalf("unexpected publish state: %#v", state)
	}
	assertPublishStateJSON(t, gotPublishJSON, true, true)
}

func TestFormsPublishCmdUnpublish(t *testing.T) {
	origNew := newFormsService
	t.Cleanup(func() { newFormsService = origNew })

	var gotPublish formsapi.SetPublishSettingsRequest
	var gotPublishJSON map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/forms/form1:setPublishSettings"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read setPublishSettings: %v", err)
			}
			if err := json.Unmarshal(body, &gotPublish); err != nil {
				t.Fatalf("decode setPublishSettings: %v", err)
			}
			if err := json.Unmarshal(body, &gotPublishJSON); err != nil {
				t.Fatalf("decode setPublishSettings JSON: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"publishSettings": map[string]any{
					"publishState": map[string]any{
						"isPublished":          false,
						"isAcceptingResponses": false,
					},
				},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/forms/form1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"formId": "form1",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	newFormsService = func(ctx context.Context, account string) (*formsapi.Service, error) {
		return newFormsTestService(t, ctx, srv), nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{"--json", "--account", "a@b.com", "forms", "publish", "form1", "--unpublish"})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotPublish.UpdateMask != "publish_state" {
		t.Fatalf("UpdateMask = %q, want publish_state", gotPublish.UpdateMask)
	}
	if gotPublish.PublishSettings == nil || gotPublish.PublishSettings.PublishState == nil {
		t.Fatalf("missing publish state: %#v", gotPublish.PublishSettings)
	}
	state := gotPublish.PublishSettings.PublishState
	if state.IsPublished || state.IsAcceptingResponses {
		t.Fatalf("unexpected publish state: %#v", state)
	}
	assertPublishStateJSON(t, gotPublishJSON, false, false)

	var parsed struct {
		Published          bool   `json:"published"`
		AcceptingResponses bool   `json:"accepting_responses"`
		FormID             string `json:"form_id"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if parsed.Published || parsed.AcceptingResponses || parsed.FormID != "form1" {
		t.Fatalf("unexpected JSON payload: %#v", parsed)
	}
}

func TestFormsPublishCmdAcceptingResponsesFalseJSON(t *testing.T) {
	origNew := newFormsService
	t.Cleanup(func() { newFormsService = origNew })

	var gotPublish formsapi.SetPublishSettingsRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1/forms/form1:setPublishSettings"):
			if err := json.NewDecoder(r.Body).Decode(&gotPublish); err != nil {
				t.Fatalf("decode setPublishSettings: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"publishSettings": map[string]any{
					"publishState": map[string]any{
						"isPublished":          true,
						"isAcceptingResponses": false,
					},
				},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/forms/form1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"formId":       "form1",
				"responderUri": "https://docs.google.com/forms/d/e/form1/viewform",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	newFormsService = func(ctx context.Context, account string) (*formsapi.Service, error) {
		return newFormsTestService(t, ctx, srv), nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"forms", "publish", "form1",
				"--accepting-responses=false",
			})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if gotPublish.PublishSettings == nil ||
		gotPublish.PublishSettings.PublishState == nil ||
		!gotPublish.PublishSettings.PublishState.IsPublished ||
		gotPublish.PublishSettings.PublishState.IsAcceptingResponses {
		t.Fatalf("unexpected publish state: %#v", gotPublish.PublishSettings)
	}

	var parsed struct {
		Published          bool   `json:"published"`
		AcceptingResponses bool   `json:"accepting_responses"`
		FormID             string `json:"form_id"`
		ResponderURI       string `json:"responder_uri"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if !parsed.Published || parsed.AcceptingResponses || parsed.FormID != "form1" {
		t.Fatalf("unexpected JSON payload: %#v", parsed)
	}
	if parsed.ResponderURI != "https://docs.google.com/forms/d/e/form1/viewform" {
		t.Fatalf("responder_uri = %q", parsed.ResponderURI)
	}
}

func TestFormsPublishCmdDryRun(t *testing.T) {
	origNew := newFormsService
	t.Cleanup(func() { newFormsService = origNew })
	newFormsService = func(context.Context, string) (*formsapi.Service, error) {
		t.Fatalf("dry-run should not create forms service")
		return nil, errors.New("unexpected forms service call")
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{
				"--json",
				"--dry-run",
				"--account", "a@b.com",
				"forms", "publish", "form1",
			})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			FormID             string `json:"form_id"`
			Published          bool   `json:"published"`
			AcceptingResponses bool   `json:"accepting_responses"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if !parsed.DryRun || parsed.Op != "forms.publish" {
		t.Fatalf("unexpected dry-run payload: %#v", parsed)
	}
	if parsed.Request.FormID != "form1" || !parsed.Request.Published || !parsed.Request.AcceptingResponses {
		t.Fatalf("unexpected request payload: %#v", parsed.Request)
	}
}

func assertPublishStateJSON(t *testing.T, payload map[string]any, wantPublished, wantAccepting bool) {
	t.Helper()
	settings, _ := payload["publishSettings"].(map[string]any)
	state, _ := settings["publishState"].(map[string]any)
	if got, ok := state["isPublished"].(bool); !ok || got != wantPublished {
		t.Fatalf("JSON isPublished = %v (%t), want %t; payload=%#v", state["isPublished"], ok, wantPublished, payload)
	}
	if got, ok := state["isAcceptingResponses"].(bool); !ok || got != wantAccepting {
		t.Fatalf("JSON isAcceptingResponses = %v (%t), want %t; payload=%#v", state["isAcceptingResponses"], ok, wantAccepting, payload)
	}
}
