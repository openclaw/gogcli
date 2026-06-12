package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestDriveShortcutCreateCmd_DefaultNameAndJSON(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	var createBody drive.File
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/files/target1"):
			requireQuery(t, r, "supportsAllDrives", "true")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "target1",
				"name": "Target Folder",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/files"):
			requireQuery(t, r, "supportsAllDrives", "true")
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "shortcut1",
				"name":     "Target Folder",
				"mimeType": driveMimeShortcut,
				"parents":  []string{"folder1"},
				"shortcutDetails": map[string]any{
					"targetId":       "target1",
					"targetMimeType": driveMimeFolder,
				},
				"webViewLink": "https://drive.example/shortcut1",
			})
		default:
			http.NotFound(w, r)
		}
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	out := captureStdout(t, func() {
		cmd := &DriveShortcutCreateCmd{}
		if execErr := runKong(t, cmd, []string{"target1", "--parent", "folder1"}, ctx, &RootFlags{Account: "a@example.com"}); execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	})

	if createBody.Name != "Target Folder" || createBody.MimeType != driveMimeShortcut {
		t.Fatalf("unexpected create body: %#v", createBody)
	}
	if len(createBody.Parents) != 1 || createBody.Parents[0] != "folder1" {
		t.Fatalf("unexpected parents: %#v", createBody.Parents)
	}
	if createBody.ShortcutDetails == nil || createBody.ShortcutDetails.TargetId != "target1" {
		t.Fatalf("unexpected shortcut details: %#v", createBody.ShortcutDetails)
	}

	var parsed struct {
		Shortcut *drive.File `json:"shortcut"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, out)
	}
	if parsed.Shortcut == nil || parsed.Shortcut.Id != "shortcut1" {
		t.Fatalf("unexpected output: %#v", parsed.Shortcut)
	}
	if got := driveShortcutTargetID(parsed.Shortcut); got != "target1" {
		t.Fatalf("target id = %q, want target1", got)
	}
}

func TestDriveShortcutCreateCmd_ExplicitNameSkipsTargetLookup(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/files") {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body drive.File
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode create body: %v", err)
		}
		if body.Name != "Alias" {
			t.Fatalf("name = %q, want Alias", body.Name)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "shortcut1",
			"name": body.Name,
			"shortcutDetails": map[string]any{
				"targetId": "target1",
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

	var stdout bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	cmd := &DriveShortcutCreateCmd{}
	if execErr := runKong(t, cmd, []string{"target1", "--parent", "folder1", "--name", "Alias"}, ctx, &RootFlags{Account: "a@example.com"}); execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if !strings.Contains(stdout.String(), "target_id\ttarget1") {
		t.Fatalf("missing target output: %q", stdout.String())
	}
}

func TestDriveShortcutCreateCmd_ValidationAndDryRun(t *testing.T) {
	ctx := newCmdOutputContext(t, io.Discard, io.Discard)
	flags := &RootFlags{Account: "a@example.com"}
	for _, tc := range []struct {
		name string
		cmd  DriveShortcutCreateCmd
		want string
	}{
		{name: "target", cmd: DriveShortcutCreateCmd{Parent: "folder1"}, want: "empty targetId"},
		{name: "parent", cmd: DriveShortcutCreateCmd{TargetID: "target1"}, want: "missing --parent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Run(ctx, flags)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	dryCtx := outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	out := captureStdout(t, func() {
		err := (&DriveShortcutCreateCmd{
			TargetID: "target1",
			Parent:   "folder1",
		}).Run(dryCtx, &RootFlags{DryRun: true})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 0 {
			t.Fatalf("dry run exit = %v", err)
		}
	})
	if !strings.Contains(out, "drive.shortcut.create") {
		t.Fatalf("unexpected dry-run output: %q", out)
	}
}

func TestDriveGetCmd_ShortcutDetails(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/files/shortcut1") {
			http.NotFound(w, r)
			return
		}
		if fields := r.URL.Query().Get("fields"); !strings.Contains(fields, "shortcutDetails") {
			http.Error(w, "missing shortcutDetails field mask", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "shortcut1",
			"name":     "Alias",
			"mimeType": driveMimeShortcut,
			"parents":  []string{"folder1"},
			"shortcutDetails": map[string]any{
				"targetId":          "target1",
				"targetMimeType":    driveMimeFolder,
				"targetResourceKey": "resource-key",
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

	var stdout bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	if err := (&DriveGetCmd{FileID: "shortcut1"}).Run(ctx, &RootFlags{Account: "a@example.com"}); err != nil {
		t.Fatalf("get shortcut: %v", err)
	}
	text := stdout.String()
	for _, want := range []string{
		"type\t" + driveMimeShortcut,
		"parents\tfolder1",
		"target_id\ttarget1",
		"target_type\t" + driveMimeFolder,
		"target_resource_key\tresource-key",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in output: %q", want, text)
		}
	}
}

func TestDriveMoveCmd_ReplacesAllLegacyParents(t *testing.T) {
	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "file1",
				"name":    "Legacy file",
				"parents": []string{"old-a", "old-b"},
			})
		case http.MethodPatch:
			requireQuery(t, r, "addParents", "new-parent")
			requireQuery(t, r, "removeParents", "old-a,old-b")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "file1",
				"name":    "Legacy file",
				"parents": []string{"new-parent"},
			})
		default:
			http.NotFound(w, r)
		}
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

	ctx := newCmdOutputContext(t, io.Discard, io.Discard)
	if err := (&DriveMoveCmd{FileID: "file1", Parent: "new-parent"}).Run(ctx, &RootFlags{Account: "a@example.com"}); err != nil {
		t.Fatalf("move legacy file: %v", err)
	}
}
