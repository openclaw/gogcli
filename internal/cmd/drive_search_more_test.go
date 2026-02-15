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
		if errMsg := driveAllDrivesQueryError(r, true); errMsg != "" {
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
		if errMsg := driveAllDrivesQueryError(r, true); errMsg != "" {
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

func TestDriveSearchCmd_NoAllDrives(t *testing.T) {
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
		if errMsg := driveAllDrivesQueryError(r, false); errMsg != "" {
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{}})
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
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DriveSearchCmd{}
	if execErr := runKong(t, cmd, []string{"hello", "--no-all-drives"}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
}

func TestDriveSearchCmd_PassesThroughDriveFilterQueries(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	const query = "mimeType = 'application/vnd.google-apps.document'"
	const wantQ = query + " and trashed = false"

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
		if errMsg := driveAllDrivesQueryError(r, true); errMsg != "" {
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}
		if got := r.URL.Query().Get("q"); got != wantQ {
			http.Error(w, "unexpected query: "+got, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"files": []map[string]any{
				{
					"id":           "f1",
					"name":         "Doc",
					"mimeType":     "application/vnd.google-apps.document",
					"modifiedTime": "2025-12-12T14:37:47Z",
				},
			},
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
		if execErr := runKong(t, cmd, []string{query}, ctx, flags); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})
}

func TestDriveSearchCmd_RawQueryBypassesFullTextWrapping(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	const query = "hello world"
	const wantQ = query + " and trashed = false"

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
		if errMsg := driveAllDrivesQueryError(r, true); errMsg != "" {
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}
		if got := r.URL.Query().Get("q"); got != wantQ {
			http.Error(w, "unexpected query: "+got, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{}})
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
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &DriveSearchCmd{}
	if execErr := runKong(t, cmd, []string{query, "--raw-query"}, ctx, flags); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
}
