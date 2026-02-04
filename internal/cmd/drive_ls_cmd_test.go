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

func TestDriveLsCmd_TextAndJSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/drive/v3/files" || r.URL.Path == "/files"):
			if errMsg := driveAllDrivesQueryError(r); errMsg != "" {
				http.Error(w, errMsg, http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"files": []map[string]any{
					{
						"id":           "f1",
						"name":         "Doc",
						"mimeType":     "application/pdf",
						"size":         "1024",
						"modifiedTime": "2025-12-12T14:37:47Z",
					},
					{
						"id":           "d1",
						"name":         "Folder",
						"mimeType":     "application/vnd.google-apps.folder",
						"size":         "0",
						"modifiedTime": "2025-12-11T00:00:00Z",
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

	// Text mode: table to stdout + next page hint to stderr.
	var errBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	textOut := captureStdout(t, func() {
		cmd := &DriveLsCmd{}
		if execErr := runKong(t, cmd, []string{}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	if !strings.Contains(textOut, "ID") || !strings.Contains(textOut, "NAME") {
		t.Fatalf("unexpected table header: %q", textOut)
	}
	if !strings.Contains(textOut, "f1") || !strings.Contains(textOut, "Doc") || !strings.Contains(textOut, "1.0 KB") {
		t.Fatalf("missing file row: %q", textOut)
	}
	if !strings.Contains(textOut, "d1") || !strings.Contains(textOut, "Folder") || !strings.Contains(textOut, "folder") {
		t.Fatalf("missing folder row: %q", textOut)
	}
	if !strings.Contains(errBuf.String(), "--page npt") {
		t.Fatalf("missing next page hint: %q", errBuf.String())
	}

	// JSON mode: JSON to stdout and no next-page hint to stderr.
	var errBuf2 bytes.Buffer
	u2, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf2, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx2 := ui.WithUI(context.Background(), u2)
	ctx2 = outfmt.WithMode(ctx2, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		cmd := &DriveLsCmd{}
		if execErr := runKong(t, cmd, []string{}, ctx2, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if errBuf2.String() != "" {
		t.Fatalf("expected no stderr in json mode, got: %q", errBuf2.String())
	}

	var parsed struct {
		Files         []*drive.File `json:"files"`
		NextPageToken string        `json:"nextPageToken"`
	}
	if unmarshalErr := json.Unmarshal([]byte(jsonOut), &parsed); unmarshalErr != nil {
		t.Fatalf("json parse: %v\nout=%q", unmarshalErr, jsonOut)
	}
	if parsed.NextPageToken != "npt" || len(parsed.Files) != 2 {
		t.Fatalf("unexpected json: %#v", parsed)
	}

	// Plain mode: stable TSV (tabs preserved).
	var errBuf3 bytes.Buffer
	u3, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf3, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx3 := ui.WithUI(context.Background(), u3)
	ctx3 = outfmt.WithMode(ctx3, outfmt.Mode{Plain: true})

	plainOut := captureStdout(t, func() {
		cmd := &DriveLsCmd{}
		if execErr := runKong(t, cmd, []string{}, ctx3, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if !strings.Contains(plainOut, "ID\tNAME\tTYPE\tSIZE\tMODIFIED") {
		t.Fatalf("expected TSV header, got: %q", plainOut)
	}
}
