package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	drivev2 "google.golang.org/api/drive/v2"
	"google.golang.org/api/drive/v3"
	gapi "google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	gogapi "github.com/steipete/gogcli/internal/googleapi"
)

func TestDriveUpload_Replace_JSON(t *testing.T) {
	var sawPatch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drive service is configured with endpoint srv.URL+"/", so API calls are rooted at /drive/v3.
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")

		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			id := strings.TrimPrefix(path, "/files/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          id,
				"name":        "Existing.pdf",
				"mimeType":    "application/pdf",
				"webViewLink": "https://example.com/" + id,
			})
			return
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/") && r.Method == http.MethodPatch:
			sawPatch = true
			if got := r.Header.Get("If-Match"); got != "" {
				t.Errorf("unexpected If-Match header for unconditional replacement: %q", got)
			}
			id := strings.TrimPrefix(r.URL.Path, "/upload/drive/v3/files/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          id,
				"name":        "Existing.pdf",
				"mimeType":    "application/pdf",
				"webViewLink": "https://example.com/" + id,
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

	local := filepath.Join(t.TempDir(), "upload.pdf")
	if writeErr := os.WriteFile(local, []byte("%PDF-1.4"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	flags := &RootFlags{Account: "a@b.com", Force: true}
	var out bytes.Buffer
	ctx := withDriveTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := &DriveUploadCmd{}
	if err := runKong(t, cmd, []string{local, "--replace", "file1"}, ctx, flags); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if !sawPatch {
		t.Fatalf("expected PATCH upload")
	}

	var got struct {
		File            *drive.File `json:"file"`
		Replaced        bool        `json:"replaced"`
		PreservedFileID bool        `json:"preservedFileId"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &got); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, out.String())
	}
	if got.File == nil || got.File.Id != "file1" {
		t.Fatalf("unexpected file: %#v", got.File)
	}
	if !got.Replaced {
		t.Fatalf("expected replaced=true")
	}
	if !got.PreservedFileID {
		t.Fatalf("expected preservedFileId=true")
	}
}

func TestDriveUpload_Replace_Text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")

		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "file1",
				"name":        "Existing.pdf",
				"mimeType":    "application/pdf",
				"webViewLink": "https://example.com/file1",
			})
			return
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/") && r.Method == http.MethodPatch:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "file1",
				"name":        "Renamed.pdf",
				"mimeType":    "application/pdf",
				"webViewLink": "https://example.com/file1",
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

	local := filepath.Join(t.TempDir(), "upload.pdf")
	if writeErr := os.WriteFile(local, []byte("%PDF-1.4"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	flags := &RootFlags{Account: "a@b.com", Force: true}
	var outBuf bytes.Buffer
	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, &outBuf, io.Discard), svc)

	cmd := &DriveUploadCmd{}
	if err := runKong(t, cmd, []string{local, "--replace", "file1", "--name", "Renamed.pdf"}, ctx, flags); err != nil {
		t.Fatalf("replace: %v", err)
	}

	out := outBuf.String()
	if !strings.Contains(out, "replaced\ttrue") {
		t.Fatalf("expected replaced=true in output, got: %q", out)
	}
	if !strings.Contains(out, "name\tRenamed.pdf") {
		t.Fatalf("expected updated name in output, got: %q", out)
	}
}

func TestDriveUpload_Replace_ParentValidation(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)

	tmp := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(tmp, []byte("abc"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := (&DriveUploadCmd{
		LocalPath:     tmp,
		Parent:        "p1",
		ReplaceFileID: "file1",
	}).Run(ctx, flags)
	if err == nil {
		t.Fatalf("expected error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected ExitError code 2, got %#v", err)
	}
}

func TestDriveUpload_Replace_GoogleWorkspaceUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "doc1",
				"name":     "Doc",
				"mimeType": "application/vnd.google-apps.document",
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

	local := filepath.Join(t.TempDir(), "upload.pdf")
	if writeErr := os.WriteFile(local, []byte("%PDF-1.4"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &DriveUploadCmd{}
	if err := runKong(t, cmd, []string{local, "--replace", "doc1"}, ctx, flags); err == nil {
		t.Fatalf("expected error")
	} else if !strings.Contains(err.Error(), "Google Workspace") {
		t.Fatalf("unexpected error: %v", err)
	} else if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}
}

func TestDriveUpload_Replace_ConvertValidation(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)

	tmp := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(tmp, []byte("abc"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := (&DriveUploadCmd{
		LocalPath:     tmp,
		ReplaceFileID: "file1",
		Convert:       true,
	}).Run(ctx, flags)
	if err == nil {
		t.Fatalf("expected error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected ExitError code 2, got %#v", err)
	}
}

func TestDriveUpload_Replace_KeepRevisionForeverAndMimeType(t *testing.T) {
	const customMimeType = "application/x-custom-pdf"
	var sawKeepRevisionForever bool
	var sawMimeType bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")

		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"mimeType": "application/pdf",
			})
			return
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/") && r.Method == http.MethodPatch:
			parsedKeepRevisionForever, parseBoolErr := strconv.ParseBool(r.URL.Query().Get("keepRevisionForever"))
			if parseBoolErr != nil {
				t.Fatalf("ParseBool: %v", parseBoolErr)
			}
			sawKeepRevisionForever = parsedKeepRevisionForever
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Fatalf("ReadAll: %v", readErr)
			}
			sawMimeType = strings.Contains(string(body), customMimeType)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"name":     "Existing.pdf",
				"mimeType": "application/pdf",
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

	local := filepath.Join(t.TempDir(), "upload.pdf")
	if writeErr := os.WriteFile(local, []byte("%PDF-1.4"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &DriveUploadCmd{}
	if err := runKong(t, cmd, []string{local, "--replace", "file1", "--keep-revision-forever", "--mime-type", customMimeType}, ctx, flags); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if !sawKeepRevisionForever {
		t.Fatalf("expected keepRevisionForever query param set")
	}
	if !sawMimeType {
		t.Fatalf("expected upload body to include custom mime type %q", customMimeType)
	}
}

func TestDriveUpload_IfVersionValidationAndDryRun(t *testing.T) {
	local := writeDriveUploadReplaceTestFile(t)

	t.Run("accepted with replace and included in dry-run", func(t *testing.T) {
		var out bytes.Buffer
		ctx := newCmdRuntimeJSONOutputContext(t, &out, io.Discard)
		err := runKong(t, &DriveUploadCmd{}, []string{local, "--replace", "file1", "--if-version", "42"}, ctx, &RootFlags{DryRun: true})
		if got := ExitCode(err); got != 0 {
			t.Fatalf("expected successful dry-run, got exit code %d (err=%v)", got, err)
		}

		var payload struct {
			Request map[string]any `json:"request"`
		}
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("Unmarshal: %v (out=%q)", err, out.String())
		}
		if got := payload.Request["if_version"]; got != float64(42) {
			t.Fatalf("if_version = %#v, want 42", got)
		}
	})

	t.Run("unconditional dry-run remains compatible", func(t *testing.T) {
		var out bytes.Buffer
		ctx := newCmdRuntimeJSONOutputContext(t, &out, io.Discard)
		err := runKong(t, &DriveUploadCmd{}, []string{local, "--replace", "file1"}, ctx, &RootFlags{DryRun: true})
		if got := ExitCode(err); got != 0 {
			t.Fatalf("expected successful dry-run, got exit code %d (err=%v)", got, err)
		}

		var payload struct {
			Request map[string]any `json:"request"`
		}
		if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
			t.Fatalf("Unmarshal: %v (out=%q)", err, out.String())
		}
		if _, ok := payload.Request["if_version"]; ok {
			t.Fatalf("unconditional dry-run unexpectedly included if_version: %#v", payload.Request)
		}
	})

	for _, tc := range []struct {
		name string
		cmd  DriveUploadCmd
		want string
	}{
		{
			name: "requires replace",
			cmd:  DriveUploadCmd{LocalPath: local, IfVersion: driveUploadVersionPtr(42)},
			want: "--if-version requires --replace",
		},
		{
			name: "zero version",
			cmd:  DriveUploadCmd{LocalPath: local, ReplaceFileID: "file1", IfVersion: driveUploadVersionPtr(0)},
			want: "positive integer",
		},
		{
			name: "negative version",
			cmd:  DriveUploadCmd{LocalPath: local, ReplaceFileID: "file1", IfVersion: driveUploadVersionPtr(-1)},
			want: "positive integer",
		},
		{
			name: "existing parent incompatibility",
			cmd:  DriveUploadCmd{LocalPath: local, Parent: "folder1", ReplaceFileID: "file1", IfVersion: driveUploadVersionPtr(42)},
			want: "--parent cannot be combined with --replace",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Run(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), &RootFlags{})
			if got := ExitCode(err); got != 2 {
				t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err, tc.want)
			}
		})
	}

	for _, value := range []string{"", "not-a-version"} {
		t.Run("parse rejects "+strconv.Quote(value), func(t *testing.T) {
			_ = captureStderr(t, func() {
				err := Execute([]string{"--dry-run", "drive", "upload", local, "--replace", "file1", "--if-version", value})
				if got := ExitCode(err); got != 2 {
					t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
				}
			})
		})
	}
}

func TestDriveUpload_ConditionalReplace_MatchingVersionUsesETag(t *testing.T) {
	const etag = `"etag-42"`
	var updateCount int
	var ifMatch string
	svc := newDriveUploadReplaceTestService(t, false, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v2")
		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			fields := r.URL.Query().Get("fields")
			if !strings.Contains(fields, "version") || !strings.Contains(fields, "etag") {
				t.Errorf("metadata fields %q do not request version and ETag", fields)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"mimeType": "application/pdf",
				"version":  "42",
				"etag":     etag,
			})
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v2/files/") && r.Method == http.MethodPut:
			updateCount++
			ifMatch = r.Header.Get("If-Match")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"title":    "Existing.pdf",
				"mimeType": "application/pdf",
			})
		default:
			http.NotFound(w, r)
		}
	})

	ctx := withDriveV2TestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	err := runKong(t, &DriveUploadCmd{}, []string{writeDriveUploadReplaceTestFile(t), "--replace", "file1", "--if-version", "42"}, ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err != nil {
		t.Fatalf("conditional replace: %v", err)
	}
	if updateCount != 1 {
		t.Fatalf("update count = %d, want 1", updateCount)
	}
	if ifMatch != etag {
		t.Fatalf("If-Match = %q, want %q", ifMatch, etag)
	}
}

func TestDriveUpload_ConditionalReplace_StaleVersionStopsBeforePatch(t *testing.T) {
	var updateCount int
	svc := newDriveUploadReplaceTestService(t, false, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v2")
		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"mimeType": "application/pdf",
				"version":  "43",
				"etag":     `"etag-43"`,
			})
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v2/files/") && r.Method == http.MethodPut:
			updateCount++
			http.Error(w, "unexpected update", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	})

	ctx := withDriveV2TestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	err := runKong(t, &DriveUploadCmd{}, []string{writeDriveUploadReplaceTestFile(t), "--replace", "file1", "--if-version", "42"}, ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err == nil {
		t.Fatal("expected conflict")
	}
	for _, want := range []string{"conflict", "expected version 42", "current version is 43", "re-read"} {
		if !strings.Contains(strings.ToLower(err.Error()), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
	if updateCount != 0 {
		t.Fatalf("update count = %d, want 0", updateCount)
	}
}

func TestDriveUpload_ConditionalReplace_PreconditionFailureIsConflict(t *testing.T) {
	const etag = `"etag-before-race"`
	var updateCount int
	svc := newDriveUploadReplaceTestService(t, true, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v2")
		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"mimeType": "application/pdf",
				"version":  "42",
				"etag":     etag,
			})
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v2/files/") && r.Method == http.MethodPut:
			updateCount++
			if got := r.Header.Get("If-Match"); got != etag {
				t.Errorf("If-Match = %q, want %q", got, etag)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPreconditionFailed)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    http.StatusPreconditionFailed,
					"message": "condition not met",
					"errors": []map[string]any{{
						"reason": "conditionNotMet",
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})

	var out bytes.Buffer
	ctx := withDriveV2TestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)
	err := runKong(t, &DriveUploadCmd{}, []string{writeDriveUploadReplaceTestFile(t), "--replace", "file1", "--if-version", "42"}, ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err == nil {
		t.Fatal("expected conflict")
	}
	for _, want := range []string{"conflict", "changed after metadata was read", "not applied", "re-read"} {
		if !strings.Contains(strings.ToLower(err.Error()), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
	if updateCount != 1 {
		t.Fatalf("update count = %d, want exactly 1", updateCount)
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected success output: %q", out.String())
	}
}

func TestDriveUpload_ConditionalReplace_RetryableFailureIsNotRetried(t *testing.T) {
	const etag = `"etag-before-write"`
	var getCount int
	var updateCount int
	var fallbackCount int
	svc := newDriveUploadReplaceTestService(t, true, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v2")
		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			getCount++
			if getCount == 1 {
				http.Error(w, "retry metadata read", http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"mimeType": "application/pdf",
				"version":  "42",
				"etag":     etag,
			})
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v2/files/") && r.Method == http.MethodPut:
			updateCount++
			if got := r.Header.Get("If-Match"); got != etag {
				fallbackCount++
			}
			w.Header().Set("Content-Type", "application/json")
			if updateCount == 1 {
				http.Error(w, "ambiguous server failure", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"title":    "Existing.pdf",
				"mimeType": "application/pdf",
			})
		case r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodPost:
			fallbackCount++
			http.Error(w, "unexpected fallback", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	})

	var out bytes.Buffer
	ctx := withDriveV2TestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)
	err := runKong(t, &DriveUploadCmd{}, []string{writeDriveUploadReplaceTestFile(t), "--replace", "file1", "--if-version", "42"}, ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err == nil {
		t.Fatal("expected server error")
	}
	if getCount != 2 {
		t.Fatalf("metadata GET count = %d, want 2 to preserve normal retries", getCount)
	}
	if updateCount != 1 {
		t.Fatalf("update count = %d, want exactly 1", updateCount)
	}
	if fallbackCount != 0 {
		t.Fatalf("fallback count = %d, want 0", fallbackCount)
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected success output: %q", out.String())
	}
}

func TestDriveUpload_ConditionalReplace_LargeMediaUsesSingleRequest(t *testing.T) {
	const etag = `"etag-large"`
	var updateCount int
	var uploadType string
	svc := newDriveUploadReplaceTestService(t, true, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v2")
		switch {
		case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"mimeType": "application/pdf",
				"version":  "42",
				"etag":     etag,
			})
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v2/files/") && r.Method == http.MethodPut:
			updateCount++
			uploadType = r.URL.Query().Get("uploadType")
			if got := r.Header.Get("If-Match"); got != etag {
				t.Errorf("If-Match = %q, want %q", got, etag)
			}
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Errorf("read upload body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "file1",
				"title":    "Large.pdf",
				"mimeType": "application/pdf",
			})
		default:
			http.NotFound(w, r)
		}
	})

	local := filepath.Join(t.TempDir(), "large.pdf")
	if err := os.WriteFile(local, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Truncate(local, int64(gapi.DefaultUploadChunkSize)+1); err != nil {
		t.Fatalf("Truncate: %v", err)
	}

	ctx := withDriveV2TestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
	err := runKong(t, &DriveUploadCmd{}, []string{local, "--replace", "file1", "--if-version", "42"}, ctx, &RootFlags{Account: "a@b.com", Force: true})
	if err != nil {
		t.Fatalf("conditional replace: %v", err)
	}
	if updateCount != 1 {
		t.Fatalf("update count = %d, want exactly 1", updateCount)
	}
	if uploadType != "multipart" {
		t.Fatalf("uploadType = %q, want multipart single-request upload", uploadType)
	}
}

func TestDriveUpload_ConditionalReplace_MissingPreconditionMaterialFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name        string
		version     string
		etag        string
		wantMessage string
	}{
		{name: "missing version", etag: `"etag-42"`, wantMessage: "did not include a version"},
		{name: "missing ETag", version: "42", wantMessage: "did not include an ETag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var updateCount int
			svc := newDriveUploadReplaceTestService(t, false, func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/drive/v2")
				switch {
				case strings.HasPrefix(path, "/files/") && r.Method == http.MethodGet:
					w.Header().Set("Content-Type", "application/json")
					response := map[string]any{
						"id":       "file1",
						"mimeType": "application/pdf",
					}
					if tc.version != "" {
						response["version"] = tc.version
					}
					if tc.etag != "" {
						response["etag"] = tc.etag
					}
					_ = json.NewEncoder(w).Encode(response)
				case strings.HasPrefix(r.URL.Path, "/upload/drive/v2/files/") && r.Method == http.MethodPut:
					updateCount++
					http.Error(w, "unexpected update", http.StatusInternalServerError)
				default:
					http.NotFound(w, r)
				}
			})

			ctx := withDriveV2TestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
			err := runKong(t, &DriveUploadCmd{}, []string{writeDriveUploadReplaceTestFile(t), "--replace", "file1", "--if-version", "42"}, ctx, &RootFlags{Account: "a@b.com", Force: true})
			if err == nil || !strings.Contains(err.Error(), tc.wantMessage) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantMessage)
			}
			if !strings.Contains(err.Error(), "no update was attempted") {
				t.Fatalf("error should state that no update was attempted: %v", err)
			}
			if updateCount != 0 {
				t.Fatalf("update count = %d, want 0", updateCount)
			}
		})
	}
}

func driveUploadVersionPtr(version int64) *int64 {
	return &version
}

func writeDriveUploadReplaceTestFile(t *testing.T) string {
	t.Helper()
	local := filepath.Join(t.TempDir(), "upload.pdf")
	if err := os.WriteFile(local, []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return local
}

func newDriveUploadReplaceTestService(t *testing.T, withRetryTransport bool, handler http.HandlerFunc) *drivev2.Service {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := srv.Client()
	if withRetryTransport {
		retryTransport := gogapi.NewRetryTransport(client.Transport)
		retryTransport.BaseDelay = 0
		client = &http.Client{Transport: retryTransport}
	}
	svc, err := drivev2.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(client),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestDriveUpload_Create_KeepRevisionForever(t *testing.T) {
	var sawKeepRevisionForever bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/upload/drive/v3/files" && r.Method == http.MethodPost:
			parsedKeepRevisionForever, parseBoolErr := strconv.ParseBool(r.URL.Query().Get("keepRevisionForever"))
			if parseBoolErr != nil {
				t.Fatalf("ParseBool: %v", parseBoolErr)
			}
			sawKeepRevisionForever = parsedKeepRevisionForever
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "new1",
				"name": "upload.pdf",
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

	local := filepath.Join(t.TempDir(), "upload.pdf")
	if writeErr := os.WriteFile(local, []byte("%PDF-1.4"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	flags := &RootFlags{Account: "a@b.com", Force: true}
	ctx := withDriveTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &DriveUploadCmd{}
	if err := runKong(t, cmd, []string{local, "--keep-revision-forever"}, ctx, flags); err != nil {
		t.Fatalf("create: %v", err)
	}
	if !sawKeepRevisionForever {
		t.Fatalf("expected keepRevisionForever query param set")
	}
}
