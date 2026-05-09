package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func newLabelsServer(t *testing.T, listLabels []map[string]any, handleCreate func(http.ResponseWriter, *http.Request)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isLabelsPath := strings.HasSuffix(r.URL.Path, "/users/me/labels") || strings.HasSuffix(r.URL.Path, "/gmail/v1/users/me/labels")

		switch {
		case isLabelsPath && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"labels": listLabels})
			return
		case isLabelsPath && r.Method == http.MethodPost && handleCreate != nil:
			handleCreate(w, r)
			return
		default:
			http.NotFound(w, r)
		}
	}))
}

func stubGmailService(t *testing.T, srv *httptest.Server) {
	t.Helper()

	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

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

func TestGmailLabelsGetCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users/me/labels") || strings.HasSuffix(r.URL.Path, "/gmail/v1/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
					{"id": "Label_1", "name": "Custom", "type": "user"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/users/me/labels/") || strings.Contains(r.URL.Path, "/gmail/v1/users/me/labels/"):
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if id == "inbox" {
				// command should map name->id, but tolerate.
				id = "INBOX"
			}
			if id != "INBOX" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "INBOX",
				"name":           "INBOX",
				"type":           "system",
				"messagesTotal":  123,
				"messagesUnread": 7,
				"threadsTotal":   50,
				"threadsUnread":  3,
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) {
		return svc, nil
	}

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailLabelsGetCmd{}
		if err := runKong(t, cmd, []string{"INBOX"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Label struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			MessagesTotal  int64  `json:"messagesTotal"`
			MessagesUnread int64  `json:"messagesUnread"`
		} `json:"label"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Label.ID != "INBOX" || parsed.Label.Name != "INBOX" {
		t.Fatalf("unexpected label: %#v", parsed.Label)
	}
	if parsed.Label.MessagesTotal != 123 || parsed.Label.MessagesUnread != 7 {
		t.Fatalf("unexpected counts: %#v", parsed.Label)
	}
}

func TestGmailLabelsListCmd_TextAndJSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users/me/labels") || strings.HasSuffix(r.URL.Path, "/gmail/v1/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
					{"id": "Label_1", "name": "Custom", "type": "user"},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	// Text output uses tabwriter to os.Stdout.
	textOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})

		cmd := &GmailLabelsListCmd{}
		if err := runKong(t, cmd, []string{}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !strings.Contains(textOut, "ID") || !strings.Contains(textOut, "NAME") {
		t.Fatalf("unexpected output: %q", textOut)
	}
	if !strings.Contains(textOut, "INBOX") || !strings.Contains(textOut, "Custom") {
		t.Fatalf("missing labels: %q", textOut)
	}

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailLabelsListCmd{}
		if err := runKong(t, cmd, []string{}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Labels []*gmail.Label `json:"labels"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if len(parsed.Labels) != 2 {
		t.Fatalf("unexpected labels: %#v", parsed.Labels)
	}
}

func TestGmailLabelsModifyCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/users/me/labels") || strings.HasSuffix(r.URL.Path, "/gmail/v1/users/me/labels")):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
					{"id": "Label_1", "name": "Custom", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPost && (strings.Contains(r.URL.Path, "/users/me/threads/") || strings.Contains(r.URL.Path, "/gmail/v1/users/me/threads/")) && strings.HasSuffix(r.URL.Path, "/modify"):
			parts := strings.Split(r.URL.Path, "/")
			threadID := parts[len(parts)-2]

			var body struct {
				AddLabelIds    []string `json:"addLabelIds"`
				RemoveLabelIds []string `json:"removeLabelIds"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.AddLabelIds) != 1 || body.AddLabelIds[0] != "INBOX" {
				http.Error(w, "bad addLabelIds", http.StatusBadRequest)
				return
			}
			if len(body.RemoveLabelIds) != 1 || body.RemoveLabelIds[0] != "Label_1" {
				http.Error(w, "bad removeLabelIds", http.StatusBadRequest)
				return
			}

			if threadID == "t2" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{"code": 500, "message": "boom"},
				})
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailLabelsModifyCmd{}
		if err := runKong(t, cmd, []string{"t1", "t2", "--add", "INBOX", "--remove", "Custom"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Results []struct {
			ThreadID string `json:"threadId"`
			Success  bool   `json:"success"`
			Error    string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Results) != 2 {
		t.Fatalf("unexpected results: %#v", parsed.Results)
	}
	if parsed.Results[0].ThreadID != "t1" || !parsed.Results[0].Success {
		t.Fatalf("unexpected result 0: %#v", parsed.Results[0])
	}
	if parsed.Results[1].ThreadID != "t2" || parsed.Results[1].Success || parsed.Results[1].Error == "" {
		t.Fatalf("unexpected result 1: %#v", parsed.Results[1])
	}
}

func TestGmailLabelsCreateCmd_JSON(t *testing.T) {
	srv := newLabelsServer(t, []map[string]any{}, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name                  string `json:"name"`
			LabelListVisibility   string `json:"labelListVisibility"`
			MessageListVisibility string `json:"messageListVisibility"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		if body.Name != "Test Label" {
			http.Error(w, "unexpected name", http.StatusBadRequest)
			return
		}
		if body.LabelListVisibility != "labelShow" || body.MessageListVisibility != "show" {
			http.Error(w, "unexpected visibility", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                    "Label_123",
			"name":                  body.Name,
			"type":                  "user",
			"labelListVisibility":   body.LabelListVisibility,
			"messageListVisibility": body.MessageListVisibility,
		})
	})
	defer srv.Close()
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "a@b.com"}

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailLabelsCreateCmd{}
		if err := runKong(t, cmd, []string{"Test Label"}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Label struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"label"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Label.ID != "Label_123" {
		t.Fatalf("unexpected id: %q", parsed.Label.ID)
	}
	if parsed.Label.Name != "Test Label" {
		t.Fatalf("unexpected name: %q", parsed.Label.Name)
	}
}

func TestGmailLabelsCreateCmd_Text(t *testing.T) {
	srv := newLabelsServer(t, []map[string]any{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "Label_456",
			"name": "My Label",
			"type": "user",
		})
	})
	defer srv.Close()
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "a@b.com"}

	var buf strings.Builder
	u, uiErr := ui.New(ui.Options{Stdout: &buf, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	cmd := &GmailLabelsCreateCmd{}
	if err := runKong(t, cmd, []string{"My Label"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Created label:") {
		t.Fatalf("missing 'Created label:' in output: %q", out)
	}
	if !strings.Contains(out, "My Label") {
		t.Fatalf("missing label name in output: %q", out)
	}
	if !strings.Contains(out, "Label_456") {
		t.Fatalf("missing label id in output: %q", out)
	}
}

func TestGmailLabelsCreateCmd_EmptyName(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	// Server shouldn't be called for empty name validation
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for empty name")
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailLabelsCreateCmd{Name: "   "} // whitespace-only name
	err = cmd.Run(ctx, flags)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "label name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGmailLabelsCreateCmd_DuplicateName_Preflight(t *testing.T) {
	srv := newLabelsServer(t, []map[string]any{
		{"id": "Label_9", "name": "My Label", "type": "user"},
	}, func(http.ResponseWriter, *http.Request) {
		t.Fatalf("create should not be called when label exists")
	})
	defer srv.Close()
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailLabelsCreateCmd{}
	if err := runKong(t, cmd, []string{"My Label"}, ctx, flags); err == nil {
		t.Fatal("expected error for duplicate label name")
	} else if !strings.Contains(err.Error(), "label already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGmailLabelsCreateCmd_DuplicateName_APIError(t *testing.T) {
	srv := newLabelsServer(t, []map[string]any{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    http.StatusConflict,
				"message": "Label name exists",
				"errors": []map[string]any{
					{"message": "Label name exists", "domain": "global", "reason": "duplicate"},
				},
			},
		})
	})
	defer srv.Close()
	stubGmailService(t, srv)

	flags := &RootFlags{Account: "a@b.com"}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailLabelsCreateCmd{}
	if err := runKong(t, cmd, []string{"My Label"}, ctx, flags); err == nil {
		t.Fatal("expected error for duplicate label name")
	} else if !strings.Contains(err.Error(), "label already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGmailLabelsStyleCmd_PatchColorAndVisibility(t *testing.T) {
	var gotPatch bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/users/me/labels") || strings.HasSuffix(r.URL.Path, "/gmail/v1/users/me/labels")):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{{"id": "Label_1", "name": "Custom", "type": "user"}},
			})
			return
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/labels/Custom"):
			http.NotFound(w, r)
			return
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/labels/Label_1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "Label_1",
				"name": "Custom",
				"type": "user",
				"color": map[string]any{
					"textColor":       "#000000",
					"backgroundColor": "#ffffff",
				},
			})
			return
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/labels/Label_1"):
			gotPatch = true
			var body struct {
				Color struct {
					TextColor       string `json:"textColor"`
					BackgroundColor string `json:"backgroundColor"`
				} `json:"color"`
				LabelListVisibility   string `json:"labelListVisibility"`
				MessageListVisibility string `json:"messageListVisibility"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode patch: %v", err)
			}
			if body.Color.TextColor != "#112233" || body.Color.BackgroundColor != "#ffffff" {
				t.Fatalf("unexpected color patch: %#v", body.Color)
			}
			if body.LabelListVisibility != "labelShowIfUnread" || body.MessageListVisibility != "hide" {
				t.Fatalf("unexpected visibility patch: %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                    "Label_1",
				"name":                  "Custom",
				"type":                  "user",
				"labelListVisibility":   body.LabelListVisibility,
				"messageListVisibility": body.MessageListVisibility,
				"color": map[string]any{
					"textColor":       body.Color.TextColor,
					"backgroundColor": body.Color.BackgroundColor,
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()
	stubGmailService(t, srv)

	out := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

		cmd := &GmailLabelsStyleCmd{}
		err := runKong(t, cmd, []string{
			"Custom",
			"--text-color", "#112233",
			"--label-list-visibility", "labelShowIfUnread",
			"--message-list-visibility", "hide",
		}, ctx, &RootFlags{Account: "a@b.com"})
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !gotPatch {
		t.Fatal("expected patch call")
	}
	if !strings.Contains(out, `"textColor": "#112233"`) {
		t.Fatalf("missing color in output: %q", out)
	}
}

func TestGmailLabelsStyleCmd_RejectsSystemLabel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/labels/INBOX") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "INBOX",
			"name": "INBOX",
			"type": "system",
		})
	}))
	defer srv.Close()
	stubGmailService(t, srv)

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &GmailLabelsStyleCmd{}
	err := runKong(t, cmd, []string{"INBOX", "--background-color", "#112233"}, ctx, &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "cannot style system label") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchLabelIDToName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.HasSuffix(r.URL.Path, "/users/me/labels") || strings.HasSuffix(r.URL.Path, "/gmail/v1/users/me/labels")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"labels": []map[string]any{
				{"id": "INBOX", "name": "INBOX", "type": "system"},
				{"id": "Label_1", "name": "Custom", "type": "user"},
				{"id": "Label_2", "type": "user"},
			},
		})
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	m, err := fetchLabelIDToName(svc)
	if err != nil {
		t.Fatalf("fetchLabelIDToName: %v", err)
	}
	if m["INBOX"] != "INBOX" {
		t.Fatalf("unexpected inbox: %q", m["INBOX"])
	}
	if m["Label_1"] != "Custom" {
		t.Fatalf("unexpected label1: %q", m["Label_1"])
	}
	// If name is missing, fall back to ID.
	if m["Label_2"] != "Label_2" {
		t.Fatalf("unexpected label2: %q", m["Label_2"])
	}
}
