package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
)

type driveShortcutDownloadCalls struct {
	downloads []string
	exports   []string
}

// runDriveShortcutDownload serves metadata for shortcut sc1, which points at
// target, and records which file IDs gog downloads or exports.
func runDriveShortcutDownload(t *testing.T, target map[string]any, args ...string) (driveShortcutDownloadCalls, string, error) {
	t.Helper()
	var calls driveShortcutDownloadCalls
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/files/sc1"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "sc1",
				"name":     "Shortcut to report",
				"mimeType": driveMimeShortcut,
				"shortcutDetails": map[string]any{
					"targetId":       "t1",
					"targetMimeType": target["mimeType"],
				},
			})
		case strings.HasSuffix(r.URL.Path, "/files/t1") && target != nil:
			_ = json.NewEncoder(w).Encode(target)
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 404, "message": "File not found: t1.", "errors": []map[string]any{{"reason": "notFound"}}}})
		}
	})
	svc, cleanup := newDriveTestService(t, handler)
	t.Cleanup(cleanup)

	download := func(_ context.Context, _ *drive.Service, fileID string) (*http.Response, error) {
		calls.downloads = append(calls.downloads, fileID)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("target-bytes"))}, nil
	}
	export := func(_ context.Context, _ *drive.Service, fileID, _ string) (*http.Response, error) {
		calls.exports = append(calls.exports, fileID)
		if fileID != "t1" {
			// Drive exports only Docs Editors files; a shortcut is not one.
			return &http.Response{StatusCode: http.StatusForbidden, Status: "403 Forbidden", Body: io.NopCloser(strings.NewReader(`{"error":{"code":403,"message":"Export only supports Docs Editors files."}}`))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("exported-pdf"))}, nil
	}

	dir := t.TempDir()
	ctx := withDriveTestOperations(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc, download, export)
	err := runKong(t, &DriveDownloadCmd{}, append([]string{"sc1", "--out", dir + string(os.PathSeparator)}, args...), ctx, &RootFlags{Account: "a@b.com"})
	return calls, dir, err
}

func TestDriveDownloadCmd_ShortcutDownloadsTargetFile(t *testing.T) {
	calls, dir, err := runDriveShortcutDownload(t, map[string]any{"id": "t1", "name": "report.pdf", "mimeType": "application/pdf"})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if len(calls.exports) != 0 || len(calls.downloads) != 1 || calls.downloads[0] != "t1" {
		t.Fatalf("calls = %#v, want one download of target t1", calls)
	}
	data, readErr := os.ReadFile(filepath.Join(dir, "t1_report.pdf"))
	if readErr != nil || string(data) != "target-bytes" {
		t.Fatalf("t1_report.pdf = %q, %v", data, readErr)
	}
}

func TestDriveDownloadCmd_ShortcutExportsTargetDoc(t *testing.T) {
	calls, dir, err := runDriveShortcutDownload(t, map[string]any{"id": "t1", "name": "Plan", "mimeType": driveMimeGoogleDoc})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if len(calls.downloads) != 0 || len(calls.exports) != 1 || calls.exports[0] != "t1" {
		t.Fatalf("calls = %#v, want one export of target t1", calls)
	}
	if data, readErr := os.ReadFile(filepath.Join(dir, "t1_Plan.pdf")); readErr != nil || string(data) != "exported-pdf" {
		t.Fatalf("t1_Plan.pdf = %q, %v", data, readErr)
	}
}

func TestDriveDownloadCmd_BrokenShortcutIsNotFound(t *testing.T) {
	calls, _, err := runDriveShortcutDownload(t, nil)
	var exitErr *ExitError
	if !errors.As(stableExitCode(err), &exitErr) || exitErr.Code != 5 {
		t.Fatalf("err = %v, want not_found exit 5", err)
	}
	if !strings.Contains(err.Error(), "shortcut") {
		t.Fatalf("err = %v, want it to say the shortcut target is missing", err)
	}
	if len(calls.downloads) != 0 || len(calls.exports) != 0 {
		t.Fatalf("calls = %#v, want no content request", calls)
	}
}
