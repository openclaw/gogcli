package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailMessagesModifyCmd_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
					{"id": "TRASH", "name": "TRASH", "type": "system"},
					{"id": "Label_1", "name": "Custom", "type": "user"},
				},
			})
			return
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/users/me/messages/") && strings.HasSuffix(r.URL.Path, "/modify"):
			var body struct {
				AddLabelIds    []string `json:"addLabelIds"`
				RemoveLabelIds []string `json:"removeLabelIds"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.AddLabelIds) != 1 || body.AddLabelIds[0] != "TRASH" {
				http.Error(w, "bad addLabelIds", http.StatusBadRequest)
				return
			}
			if len(body.RemoveLabelIds) != 1 || body.RemoveLabelIds[0] != "INBOX" {
				http.Error(w, "bad removeLabelIds", http.StatusBadRequest)
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

		if err := runKong(t, &GmailMessagesModifyCmd{}, []string{
			"msg1",
			"--add", "TRASH",
			"--remove", "INBOX",
		}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Modified      string   `json:"modified"`
		AddedLabels   []string `json:"addedLabels"`
		RemovedLabels []string `json:"removedLabels"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Modified != "msg1" {
		t.Fatalf("unexpected modified: %q", parsed.Modified)
	}
	if len(parsed.AddedLabels) != 1 || parsed.AddedLabels[0] != "TRASH" {
		t.Fatalf("unexpected added labels: %#v", parsed.AddedLabels)
	}
	if len(parsed.RemovedLabels) != 1 || parsed.RemovedLabels[0] != "INBOX" {
		t.Fatalf("unexpected removed labels: %#v", parsed.RemovedLabels)
	}

	plainOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)

		if err := runKong(t, &GmailMessagesModifyCmd{}, []string{
			"msg1",
			"--add", "TRASH",
			"--remove", "INBOX",
		}, ctx, flags); err != nil {
			t.Fatalf("execute plain: %v", err)
		}
	})
	if !strings.Contains(plainOut, "Modified message") {
		t.Fatalf("unexpected plain output: %q", plainOut)
	}
}

func TestGmailMessagesModifyCmd_ValidationErrors(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	svc, err := gmail.NewService(context.Background(), option.WithoutAuthentication())
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

	t.Run("no labels", func(t *testing.T) {
		err := runKong(t, &GmailMessagesModifyCmd{}, []string{"msg1"}, ctx, flags)
		if err == nil || !strings.Contains(err.Error(), "must specify --add and/or --remove") {
			t.Fatalf("expected validation error, got %v", err)
		}
	})
}
