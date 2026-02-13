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

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
)

func TestInfoViaDriveCmd_TextAndJSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/files/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           "id1",
				"name":         "Test File",
				"mimeType":     "application/pdf",
				"size":         "123",
				"createdTime":  "2025-12-01T00:00:00Z",
				"modifiedTime": "2025-12-02T00:00:00Z",
				"webViewLink":  "https://example.com/id1",
				"parents":      []string{"p1", "p2"},
			})
			return
		}
		http.NotFound(w, r)
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

	var outBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &outBuf, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	if err := infoViaDrive(ctx, flags, infoViaDriveOptions{ArgName: "id"}, "id1"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	text := outBuf.String()
	if !strings.Contains(text, "id\tid1") || !strings.Contains(text, "mime\tapplication/pdf") {
		t.Fatalf("unexpected text: %q", text)
	}

	jsonOut := captureStdout(t, func() {
		u2, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx2 := ui.WithUI(context.Background(), u2)
		ctx2 = outfmt.WithMode(ctx2, outfmt.Mode{JSON: true})

		if err := infoViaDrive(ctx2, flags, infoViaDriveOptions{ArgName: "id"}, "id1"); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		File struct {
			ID       string   `json:"id"`
			Name     string   `json:"name"`
			MimeType string   `json:"mimeType"`
			Parents  []string `json:"parents"`
		} `json:"file"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.File.ID != "id1" || parsed.File.MimeType != "application/pdf" || len(parsed.File.Parents) != 2 {
		t.Fatalf("unexpected json: %#v", parsed.File)
	}
}

func TestInfoViaDriveCmd_ExpectedMimeError(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/files/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "id1",
				"name":     "Test File",
				"mimeType": "application/pdf",
			})
			return
		}
		http.NotFound(w, r)
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
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := infoViaDrive(ctx, flags, infoViaDriveOptions{ArgName: "id", ExpectedMime: "application/vnd.google-apps.spreadsheet", KindLabel: "sheet"}, "id1"); err == nil || !strings.Contains(err.Error(), "not a sheet") {
		t.Fatalf("expected mime error, got: %v", err)
	}
}
