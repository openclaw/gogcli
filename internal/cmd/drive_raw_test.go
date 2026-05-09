package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/drive/v3"
)

type driveRawHit struct {
	lastFields atomic.Value // string
}

func newDriveRawTestServer(t *testing.T, status int, body map[string]any, hit *driveRawHit) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		if !strings.HasPrefix(path, "/files/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if hit != nil {
			hit.lastFields.Store(r.URL.Query().Get("fields"))
		}
		if status != 0 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": status, "message": "mock error"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func installMockDriveService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	svc := newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", drive.NewService)
	stubGoogleTestService(t, &newDriveService, svc)
}

// sensitiveDriveFile returns a File response containing every sensitive
// field the audit says should be redacted by default.
func sensitiveDriveFile(id string) map[string]any {
	return map[string]any{
		"id":             id,
		"name":           "secrets.txt",
		"mimeType":       "text/plain",
		"thumbnailLink":  "https://drive.google.com/thumb/XYZ?token=LEAK",
		"webContentLink": "https://drive.google.com/download?id=XYZ",
		"exportLinks":    map[string]any{"application/pdf": "https://drive.google.com/export?id=XYZ"},
		"resourceKey":    "rk-CAPABILITY-SECRET",
		"appProperties":  map[string]any{"api_token": "sk-live-0000"},
		"properties":     map[string]any{"webhook": "https://hooks.example/private"},
		"webViewLink":    "https://docs.google.com/open?id=XYZ",
		"createdTime":    "2026-01-01T00:00:00Z",
		"modifiedTime":   "2026-01-02T00:00:00Z",
	}
}

func TestDriveRaw_DefaultRedactsSensitiveFields(t *testing.T) {
	hit := &driveRawHit{}
	srv := newDriveRawTestServer(t, 0, sensitiveDriveFile("f1"), hit)
	defer srv.Close()
	installMockDriveService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		if err := runKong(t, &DriveRawCmd{}, []string{"f1"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	// Default must request fields=* from the API.
	if got, _ := hit.lastFields.Load().(string); got != "*" {
		t.Fatalf("expected fields=* by default, got: %q", got)
	}

	var fileOut map[string]any
	if err := json.Unmarshal([]byte(out), &fileOut); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	// Safe fields remain.
	if fileOut["id"] != "f1" {
		t.Fatalf("expected id=f1, got: %v", fileOut["id"])
	}
	if fileOut["name"] != "secrets.txt" {
		t.Fatalf("expected name present, got: %v", fileOut["name"])
	}
	// Sensitive fields stripped.
	for _, key := range []string{"thumbnailLink", "webContentLink", "exportLinks", "resourceKey", "appProperties", "properties"} {
		if _, present := fileOut[key]; present {
			t.Fatalf("expected %q to be redacted from default output, got: %v", key, fileOut[key])
		}
	}
}

func TestDriveRaw_ExplicitFieldsHonorsUserChoice(t *testing.T) {
	hit := &driveRawHit{}
	srv := newDriveRawTestServer(t, 0, sensitiveDriveFile("f1"), hit)
	defer srv.Close()
	installMockDriveService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	out := captureStdout(t, func() {
		if err := runKong(t, &DriveRawCmd{}, []string{"f1", "--fields", "id,name,thumbnailLink"}, ctx, flags); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	// The Google client library can munge the fields value slightly, but it
	// must contain the user's requested field names.
	got, _ := hit.lastFields.Load().(string)
	if !strings.Contains(got, "thumbnailLink") {
		t.Fatalf("expected thumbnailLink in fields query, got: %q", got)
	}

	var fileOut map[string]any
	if err := json.Unmarshal([]byte(out), &fileOut); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	// User-named field must NOT be redacted.
	if fileOut["thumbnailLink"] != "https://drive.google.com/thumb/XYZ?token=LEAK" {
		t.Fatalf("expected thumbnailLink to be preserved when user named it, got: %v", fileOut["thumbnailLink"])
	}
}

func TestDriveRaw_APIError(t *testing.T) {
	srv := newDriveRawTestServer(t, http.StatusInternalServerError, nil, nil)
	defer srv.Close()
	installMockDriveService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &DriveRawCmd{}, []string{"f1"}, ctx, flags); err == nil {
			t.Fatalf("expected error on 500")
		}
	})
}

func TestDriveRaw_NotFound(t *testing.T) {
	srv := newDriveRawTestServer(t, http.StatusNotFound, nil, nil)
	defer srv.Close()
	installMockDriveService(t, srv)

	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	_ = captureStdout(t, func() {
		if err := runKong(t, &DriveRawCmd{}, []string{"f1"}, ctx, flags); err == nil {
			t.Fatalf("expected error on 404")
		}
	})
}

func TestDriveRaw_EmptyID(t *testing.T) {
	ctx := rawTestContext(t)
	flags := &RootFlags{Account: "a@b.com"}
	if err := (&DriveRawCmd{}).Run(ctx, flags); err == nil {
		t.Fatalf("expected error on empty id")
	}
}
