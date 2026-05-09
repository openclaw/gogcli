package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type fakeDriveCommands struct {
	run         func(args ...string) string
	uploadMetas func() []map[string]any
	uploadMedia func() []string
}

func newFakeDriveCommands(t *testing.T) *fakeDriveCommands {
	t.Helper()

	origNew := newDriveService
	t.Cleanup(func() { newDriveService = origNew })

	uploadMetas := make([]map[string]any, 0, 4)
	uploadMedia := make([]string, 0, 4)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/drive/v3")
		isUpload := strings.HasPrefix(r.URL.Path, "/upload/drive/v3")
		if isUpload {
			path = strings.TrimPrefix(r.URL.Path, "/upload/drive/v3")
		}
		switch {
		case r.Method == http.MethodGet && path == "/files":
			q := r.URL.Query().Get("q")
			if strings.Contains(q, "fullText contains") {
				if got := r.URL.Query().Get("corpora"); got != "allDrives" {
					t.Fatalf("expected corpora=allDrives, got: %q", r.URL.RawQuery)
				}
			}
			if strings.Contains(q, "fullText contains") {
				if errMsg := driveAllDrivesQueryError(r, true); errMsg != "" {
					t.Fatalf("%s: %q", errMsg, r.URL.RawQuery)
				}
			}
			if strings.Contains(q, "empty") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"files": []map[string]any{},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"nextPageToken": "next",
				"files": []map[string]any{
					{
						"id":           "file1",
						"name":         "File One",
						"mimeType":     "text/plain",
						"size":         "12",
						"modifiedTime": "2025-01-01T00:00:00Z",
					},
				},
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/files/") && strings.HasSuffix(path, "/permissions"):
			if r.URL.Query().Get("pageToken") == "empty" {
				_ = json.NewEncoder(w).Encode(map[string]any{"permissions": []map[string]any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"permissions": []map[string]any{
					{
						"id":           "perm1",
						"type":         "user",
						"role":         "reader",
						"emailAddress": "p@example.com",
					},
				},
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/files/"):
			id := strings.TrimPrefix(path, "/files/")
			if strings.Contains(id, "/") {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           id,
				"name":         "File " + id,
				"mimeType":     "text/plain",
				"size":         "5",
				"createdTime":  "2025-01-01T00:00:00Z",
				"modifiedTime": "2025-01-02T00:00:00Z",
				"description":  "desc",
				"starred":      true,
				"parents":      []string{"old-parent"},
				"webViewLink":  "https://drive.example/" + id,
			})
			return
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/copy"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "copy1",
				"name": "Copy",
			})
			return
		case r.Method == http.MethodPost && path == "/files":
			respName := "New"
			respMimeType := "text/plain"
			if isUpload {
				meta, media, err := readUploadRequest(r)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				uploadMetas = append(uploadMetas, meta)
				uploadMedia = append(uploadMedia, media)
				if v, ok := meta["name"].(string); ok && strings.TrimSpace(v) != "" {
					respName = v
				}
				if v, ok := meta["mimeType"].(string); ok && strings.TrimSpace(v) != "" {
					respMimeType = v
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "new1",
				"name":        respName,
				"mimeType":    respMimeType,
				"webViewLink": "https://drive.example/new1",
			})
			return
		case r.Method == http.MethodPatch && strings.HasPrefix(path, "/files/"):
			id := strings.TrimPrefix(path, "/files/")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          id,
				"name":        "Updated",
				"parents":     []string{"parent"},
				"webViewLink": "https://drive.example/" + id,
			})
			return
		case r.Method == http.MethodDelete && strings.HasPrefix(path, "/files/") && !strings.Contains(path, "/permissions"):
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/permissions"):
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			typ, _ := req["type"].(string)
			role, _ := req["role"].(string)
			if role == "" {
				role = "reader"
			}

			switch typ {
			case "user":
				email, _ := req["emailAddress"].(string)
				if email == "" {
					http.Error(w, "missing emailAddress", http.StatusBadRequest)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":           "perm1",
					"type":         "user",
					"role":         role,
					"emailAddress": email,
				})
				return
			case "domain":
				domain, _ := req["domain"].(string)
				if domain == "" {
					http.Error(w, "missing domain", http.StatusBadRequest)
					return
				}
				resp := map[string]any{
					"id":     "perm1",
					"type":   "domain",
					"role":   role,
					"domain": domain,
				}
				if afd, ok := req["allowFileDiscovery"].(bool); ok {
					resp["allowFileDiscovery"] = afd
				}
				_ = json.NewEncoder(w).Encode(resp)
				return
			case "anyone":
				resp := map[string]any{
					"id":   "perm1",
					"type": "anyone",
					"role": role,
				}
				if afd, ok := req["allowFileDiscovery"].(bool); ok {
					resp["allowFileDiscovery"] = afd
				}
				_ = json.NewEncoder(w).Encode(resp)
				return
			default:
				http.Error(w, "invalid type", http.StatusBadRequest)
				return
			}
		case r.Method == http.MethodDelete && strings.Contains(path, "/permissions/"):
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
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

	run := func(args ...string) string {
		t.Helper()
		return captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if execErr := Execute(args); execErr != nil {
					t.Fatalf("Execute %v: %v", args, execErr)
				}
			})
		})
	}

	return &fakeDriveCommands{
		run:         run,
		uploadMetas: func() []map[string]any { return uploadMetas },
		uploadMedia: func() []string { return uploadMedia },
	}
}

func TestDriveCommands_ListSearchGetCopy(t *testing.T) {
	fake := newFakeDriveCommands(t)
	run := fake.run

	_ = run("--account", "a@b.com", "drive", "ls", "--query", "empty")
	out := run("--json", "--account", "a@b.com", "drive", "ls")
	if !strings.Contains(out, "\"files\"") {
		t.Fatalf("unexpected ls json: %q", out)
	}

	_ = run("--account", "a@b.com", "drive", "search", "empty")
	out = run("--json", "--account", "a@b.com", "drive", "search", "hello")
	if !strings.Contains(out, "\"files\"") {
		t.Fatalf("unexpected search json: %q", out)
	}

	out = run("--json", "--account", "a@b.com", "drive", "get", "file1")
	if !strings.Contains(out, "\"file\"") {
		t.Fatalf("unexpected get json: %q", out)
	}

	out = run("--json", "--account", "a@b.com", "drive", "copy", "file1", "Copy")
	if !strings.Contains(out, "\"file\"") {
		t.Fatalf("unexpected copy json: %q", out)
	}
}

func TestDriveCommands_UploadVariants(t *testing.T) {
	fake := newFakeDriveCommands(t)
	run := fake.run

	tmp := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(tmp, []byte("data"), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	out := run("--json", "--account", "a@b.com", "drive", "upload", tmp)
	if !strings.Contains(out, "\"file\"") {
		t.Fatalf("unexpected upload json: %q", out)
	}
	baseMeta := latestUploadMeta(t, fake.uploadMetas())
	if got := toString(baseMeta["name"]); got != "upload.txt" {
		t.Fatalf("upload name = %q, want upload.txt", got)
	}
	if _, ok := baseMeta["mimeType"]; ok {
		t.Fatalf("unexpected mimeType on plain upload metadata: %#v", baseMeta)
	}

	docxTmp := filepath.Join(t.TempDir(), "report.docx")
	if err := os.WriteFile(docxTmp, []byte("docx-data"), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	out = run("--json", "--account", "a@b.com", "drive", "upload", docxTmp, "--convert")
	if !strings.Contains(out, "\"file\"") {
		t.Fatalf("unexpected upload --convert json: %q", out)
	}
	convertMeta := latestUploadMeta(t, fake.uploadMetas())
	if got := toString(convertMeta["mimeType"]); got != driveMimeGoogleDoc {
		t.Fatalf("upload --convert mimeType = %q, want %q", got, driveMimeGoogleDoc)
	}
	if got := toString(convertMeta["name"]); got != "report" {
		t.Fatalf("upload --convert name = %q, want report", got)
	}

	out = run("--json", "--account", "a@b.com", "drive", "upload", docxTmp, "--convert", "--name", "custom.docx")
	if !strings.Contains(out, "\"file\"") {
		t.Fatalf("unexpected upload --convert --name json: %q", out)
	}
	nameMeta := latestUploadMeta(t, fake.uploadMetas())
	if got := toString(nameMeta["name"]); got != "custom.docx" {
		t.Fatalf("upload --convert --name kept name = %q, want custom.docx", got)
	}

	pngTmp := filepath.Join(t.TempDir(), "chart.png")
	if err := os.WriteFile(pngTmp, []byte("png-data"), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	out = run("--json", "--account", "a@b.com", "drive", "upload", pngTmp, "--convert-to", "sheet")
	if !strings.Contains(out, "\"file\"") {
		t.Fatalf("unexpected upload --convert-to json: %q", out)
	}
	explicitMeta := latestUploadMeta(t, fake.uploadMetas())
	if got := toString(explicitMeta["mimeType"]); got != driveMimeGoogleSheet {
		t.Fatalf("upload --convert-to mimeType = %q, want %q", got, driveMimeGoogleSheet)
	}
	if got := toString(explicitMeta["name"]); got != "chart.png" {
		t.Fatalf("upload --convert-to name = %q, want chart.png", got)
	}

	mdTmp := filepath.Join(t.TempDir(), "notes.md")
	mdContent := "---\ntitle: Secret\n---\n\n# Public\n"
	if err := os.WriteFile(mdTmp, []byte(mdContent), 0o600); err != nil {
		t.Fatalf("write temp markdown: %v", err)
	}
	out = run("--json", "--account", "a@b.com", "drive", "upload", mdTmp, "--convert")
	if !strings.Contains(out, "\"file\"") {
		t.Fatalf("unexpected upload markdown --convert json: %q", out)
	}
	mdMeta := latestUploadMeta(t, fake.uploadMetas())
	if got := toString(mdMeta["mimeType"]); got != driveMimeGoogleDoc {
		t.Fatalf("upload markdown --convert mimeType = %q, want %q", got, driveMimeGoogleDoc)
	}
	if got := toString(mdMeta["name"]); got != "notes" {
		t.Fatalf("upload markdown --convert name = %q, want notes", got)
	}
	mdMedia := latestUploadMedia(t, fake.uploadMedia())
	if strings.Contains(mdMedia, "title: Secret") || strings.HasPrefix(mdMedia, "---") {
		t.Fatalf("expected frontmatter stripped from markdown media, got %q", mdMedia)
	}
	if mdMedia != "\n# Public\n" {
		t.Fatalf("markdown media = %q, want stripped body", mdMedia)
	}

	out = run("--json", "--account", "a@b.com", "drive", "upload", mdTmp, "--convert", "--keep-frontmatter")
	if !strings.Contains(out, "\"file\"") {
		t.Fatalf("unexpected upload markdown --keep-frontmatter json: %q", out)
	}
	keptMedia := latestUploadMedia(t, fake.uploadMedia())
	if keptMedia != mdContent {
		t.Fatalf("markdown media with --keep-frontmatter = %q, want %q", keptMedia, mdContent)
	}
}

func TestDriveCommands_MutateShareAndPermissions(t *testing.T) {
	fake := newFakeDriveCommands(t)
	run := fake.run

	out := run("--account", "a@b.com", "drive", "mkdir", "Folder")
	if !strings.Contains(out, "id") {
		t.Fatalf("unexpected mkdir output: %q", out)
	}

	out = run("--json", "--account", "a@b.com", "drive", "move", "file1", "--parent", "p2")
	if !strings.Contains(out, "\"file\"") {
		t.Fatalf("unexpected move json: %q", out)
	}

	out = run("--account", "a@b.com", "drive", "rename", "file1", "Renamed")
	if !strings.Contains(out, "name") {
		t.Fatalf("unexpected rename output: %q", out)
	}

	out = run("--json", "--account", "a@b.com", "drive", "share", "file1", "--to", "user", "--email", "share@example.com")
	if !strings.Contains(out, "\"permissionId\"") {
		t.Fatalf("unexpected share json: %q", out)
	}

	out = run("--json", "--account", "a@b.com", "drive", "share", "file1", "--to", "domain", "--domain", "example.com", "--role", "writer")
	if !strings.Contains(out, "\"permissionId\"") {
		t.Fatalf("unexpected domain share json: %q", out)
	}

	out = run("--force", "--account", "a@b.com", "drive", "unshare", "file1", "perm1")
	if !strings.Contains(out, "removed") {
		t.Fatalf("unexpected unshare output: %q", out)
	}

	out = run("--json", "--account", "a@b.com", "drive", "permissions", "file1")
	if !strings.Contains(out, "\"permissions\"") {
		t.Fatalf("unexpected permissions json: %q", out)
	}

	_ = run("--account", "a@b.com", "drive", "permissions", "file1", "--page", "empty")

	out = run("--json", "--account", "a@b.com", "drive", "url", "file1", "file2")
	if !strings.Contains(out, "\"urls\"") {
		t.Fatalf("unexpected url json: %q", out)
	}

	out = run("--json", "--force", "--account", "a@b.com", "drive", "delete", "file1")
	if !strings.Contains(out, "\"deleted\"") {
		t.Fatalf("unexpected delete json: %q", out)
	}
}

func readUploadRequest(r *http.Request) (map[string]any, string, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", fmt.Errorf("parse content-type: %w", err)
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, "", fmt.Errorf("unexpected content-type: %q", mediaType)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, "", fmt.Errorf("missing multipart boundary")
	}

	reader := multipart.NewReader(r.Body, boundary)
	var meta map[string]any
	var media []byte
	for {
		part, partErr := reader.NextPart()
		if partErr == io.EOF {
			break
		}
		if partErr != nil {
			return nil, "", fmt.Errorf("read multipart: %w", partErr)
		}

		contentType := part.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "application/json") {
			body, readErr := io.ReadAll(part)
			if readErr != nil {
				return nil, "", fmt.Errorf("read media part: %w", readErr)
			}
			media = body
			continue
		}

		if err := json.NewDecoder(part).Decode(&meta); err != nil {
			return nil, "", fmt.Errorf("decode metadata json: %w", err)
		}
	}

	if meta == nil {
		return nil, "", fmt.Errorf("metadata part not found")
	}
	return meta, string(media), nil
}

func latestUploadMeta(t *testing.T, uploadMetas []map[string]any) map[string]any {
	t.Helper()
	if len(uploadMetas) == 0 {
		t.Fatalf("expected at least one upload metadata entry")
	}
	return uploadMetas[len(uploadMetas)-1]
}

func latestUploadMedia(t *testing.T, uploadMedia []string) string {
	t.Helper()
	if len(uploadMedia) == 0 {
		t.Fatalf("expected at least one upload media entry")
	}
	return uploadMedia[len(uploadMedia)-1]
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}
