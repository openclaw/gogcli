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

func driveAllDrivesQueryError(r *http.Request) string {
	q := r.URL.Query()
	if q.Get("corpora") != "allDrives" {
		return "missing corpora=allDrives"
	}
	if q.Get("supportsAllDrives") != "true" {
		return "missing supportsAllDrives=true"
	}
	if q.Get("includeItemsFromAllDrives") != "true" {
		return "missing includeItemsFromAllDrives=true"
	}
	return ""
}

func TestDriveSearchCmd_TextAndJSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		if path != "/files" {
			http.NotFound(w, r)
			return
		}
		if errMsg := driveAllDrivesQueryError(r); errMsg != "" {
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{
				{
					"id":           "f1",
					"name":         "Report",
					"mimeType":     "application/pdf",
					"size":         "1024",
					"modifiedTime": "2025-12-12T14:37:47Z",
				},
			},
			"nextPageToken": "npt",
		})
	}))
	t.Cleanup(srv.Close)

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
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	textOut := captureStdout(t, func() {
		cmd := &DriveSearchCmd{}
		if execErr := runKong(t, cmd, []string{"hello"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if !strings.Contains(textOut, "Report") {
		t.Fatalf("unexpected output: %q", textOut)
	}
	if !strings.Contains(errBuf.String(), "--page npt") {
		t.Fatalf("missing next page hint: %q", errBuf.String())
	}

	jsonCtx := outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	jsonOut := captureStdout(t, func() {
		cmd := &DriveSearchCmd{}
		if execErr := runKong(t, cmd, []string{"hello"}, jsonCtx, flags); execErr != nil {
			t.Fatalf("execute json: %v", execErr)
		}
	})
	if !strings.Contains(jsonOut, "\"files\"") {
		t.Fatalf("unexpected json: %q", jsonOut)
	}
}

func TestDriveSearchCmd_NoResultsAndEmptyQuery(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if errMsg := driveAllDrivesQueryError(r); errMsg != "" {
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{},
		})
	}))
	t.Cleanup(srv.Close)

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
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	_ = captureStdout(t, func() {
		cmd := &DriveSearchCmd{}
		if execErr := runKong(t, cmd, []string{"empty"}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
	if !strings.Contains(errBuf.String(), "No results") {
		t.Fatalf("expected No results, got: %q", errBuf.String())
	}

	cmd := &DriveSearchCmd{}
	if err := runKong(t, cmd, []string{}, ctx, flags); err == nil {
		t.Fatalf("expected empty query error")
	}
}
