package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDriveCommentsListCmd_TextAndJSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodGet && path == "/files/id1/comments":
			if r.URL.Query().Get("pageSize") != "1" {
				t.Fatalf("expected pageSize=1, got: %q", r.URL.RawQuery)
			}
			if r.URL.Query().Get("pageToken") != "p1" {
				t.Fatalf("expected pageToken=p1, got: %q", r.URL.RawQuery)
			}
			fields := r.URL.Query().Get("fields")
			if !strings.Contains(fields, "quotedFileContent") {
				t.Fatalf("expected quotedFileContent in fields, got: %q", fields)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"comments": []map[string]any{
					{
						"id":          "c1",
						"author":      map[string]any{"displayName": "Alice"},
						"content":     "Hello",
						"createdTime": "2025-01-01T00:00:00Z",
						"resolved":    true,
						"quotedFileContent": map[string]any{
							"value": "Quoted",
						},
						"replies": []map[string]any{{"id": "r1"}},
					},
				},
				"nextPageToken": "npt",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	var errBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	textOut := captureStdout(t, func() {
		cmd := &DriveCommentsListCmd{}
		if execErr := runKong(t, cmd, []string{"--max", "1", "--page", "p1", "--include-quoted", "id1"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if !strings.Contains(textOut, "QUOTED") || !strings.Contains(textOut, "Hello") || !strings.Contains(textOut, "Alice") {
		t.Fatalf("unexpected output: %q", textOut)
	}
	if !strings.Contains(errBuf.String(), "--page npt") {
		t.Fatalf("missing next page hint: %q", errBuf.String())
	}

	var errBuf2 bytes.Buffer
	u2, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf2, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx2 := ui.WithUI(context.Background(), u2)
	ctx2 = outfmt.WithMode(ctx2, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveCommentsListCmd{}
		if execErr := runKong(t, cmd, []string{"--max", "1", "--page", "p1", "--include-quoted", "id1"}, ctx2, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if errBuf2.String() != "" {
		t.Fatalf("expected no stderr in json mode, got: %q", errBuf2.String())
	}

	var parsed struct {
		FileID        string           `json:"fileId"`
		Comments      []*drive.Comment `json:"comments"`
		NextPageToken string           `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, jsonOut)
	}
	if parsed.FileID != "id1" || parsed.NextPageToken != "npt" || len(parsed.Comments) != 1 {
		t.Fatalf("unexpected json: %#v", parsed)
	}
	if parsed.Comments[0].QuotedFileContent == nil || parsed.Comments[0].QuotedFileContent.Value != "Quoted" {
		t.Fatalf("missing quoted content: %#v", parsed.Comments[0])
	}
}

func TestDriveCommentsCreateCmd_JSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case r.Method == http.MethodPost && path == "/files/id1/comments":
			fields := r.URL.Query().Get("fields")
			if !strings.Contains(fields, "quotedFileContent") {
				t.Fatalf("expected quotedFileContent in fields, got: %q", fields)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if payload["content"] != "Hello" {
				t.Fatalf("expected content Hello, got: %#v", payload["content"])
			}
			quoted, ok := payload["quotedFileContent"].(map[string]any)
			if !ok || quoted["value"] != "Quote" {
				t.Fatalf("expected quoted value Quote, got: %#v", payload["quotedFileContent"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "c1",
				"content":     "Hello",
				"createdTime": "2025-01-01T00:00:00Z",
				"quotedFileContent": map[string]any{
					"value": "Quote",
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx = ui.WithUI(ctx, u)

	out := captureStdout(t, func() {
		cmd := &DriveCommentsCreateCmd{}
		if execErr := runKong(t, cmd, []string{"--quoted", "Quote", "id1", "Hello"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	var parsed struct {
		Comment *drive.Comment `json:"comment"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v out=%q", err, out)
	}
	if parsed.Comment == nil || parsed.Comment.Id != "c1" || parsed.Comment.Content != "Hello" {
		t.Fatalf("unexpected comment: %#v", parsed.Comment)
	}
}
