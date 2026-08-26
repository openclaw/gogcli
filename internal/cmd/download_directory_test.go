package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestDownloadResolversHonorNewDirectoryIntent(t *testing.T) {
	home := setDownloadTestHome(t)

	for _, test := range []struct {
		name     string
		filename string
		resolve  func(string) (string, error)
	}{
		{
			name: "drive", filename: "file1_report.txt",
			resolve: func(out string) (string, error) {
				return resolveDriveDownloadDestPath(&drive.File{Id: "file1", Name: "../report.txt"}, out, "")
			},
		},
		{
			name: "photos", filename: "report.txt",
			resolve: func(out string) (string, error) {
				return resolvePhotosDownloadDestPathParts("photo1", "../report.txt", out, "")
			},
		},
		{
			name: "docs tab", filename: "doc1_Budget_2026.pdf",
			resolve: func(out string) (string, error) {
				return tabExportOutPath(out, "doc1", "Budget 2026", "pdf", "")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := "~/new/" + strings.ReplaceAll(test.name, " ", "-") + "/"
			got, err := test.resolve(out)
			if err != nil {
				t.Fatalf("resolve directory: %v", err)
			}
			want := filepath.Join(home, "new", strings.ReplaceAll(test.name, " ", "-"), test.filename)
			if got != want {
				t.Fatalf("directory destination = %q, want %q", got, want)
			}
			if _, statErr := os.Stat(filepath.Dir(want)); !os.IsNotExist(statErr) {
				t.Fatalf("resolver created destination directory: %v", statErr)
			}

			explicitFile := filepath.Join(home, "explicit", test.name)
			got, err = test.resolve(explicitFile)
			if err != nil || got != explicitFile {
				t.Fatalf("explicit file = %q, %v; want %q", got, err, explicitFile)
			}
		})
	}
}

func TestDriveDownloadAndExportsHonorHomeDirectoryIntent(t *testing.T) {
	home := setDownloadTestHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fileID := filepath.Base(r.URL.Path)
		mimeType := "text/plain"
		if fileID == "doc1" {
			mimeType = "application/vnd.google-apps.document"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": fileID, "name": "../report.txt", "mimeType": mimeType})
	}))
	defer srv.Close()

	svc, err := drive.NewService(context.Background(), option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	download := func(context.Context, *drive.Service, string) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("proof"))}, nil
	}
	export := func(context.Context, *drive.Service, string, string) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("proof"))}, nil
	}

	for _, test := range []struct {
		name     string
		args     []string
		filename string
	}{
		{name: "drive", args: []string{"drive", "download", "file1"}, filename: "file1_report.txt"},
		{name: "docs", args: []string{"docs", "export", "doc1", "--format", "pdf"}, filename: "doc1_report.pdf"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--json", "--account", "a@example.com"}, test.args...)
			args = append(args, "--out", "~/nested/"+test.name+"/")
			result := executeWithDriveTestOperations(t, args, svc, download, export)
			if result.err != nil {
				t.Fatalf("download: %v", result.err)
			}
			var got struct {
				Path string `json:"path"`
				Size int64  `json:"size"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			want := filepath.Join(home, "nested", test.name, test.filename)
			if got.Path != want || got.Size != 5 {
				t.Fatalf("download result = %#v, want path %q and size 5", got, want)
			}
			data, err := os.ReadFile(want)
			if err != nil || string(data) != "proof" {
				t.Fatalf("downloaded data = %q, %v", data, err)
			}
			if duplicate := executeWithDriveTestOperations(t, args, svc, download, export); !errors.Is(duplicate.err, os.ErrExist) {
				t.Fatalf("duplicate download error = %v, want existing-file protection", duplicate.err)
			}
			if overwrite := executeWithDriveTestOperations(t, append(args, "--overwrite"), svc, download, export); overwrite.err != nil {
				t.Fatalf("overwrite download: %v", overwrite.err)
			}
		})
	}
}

func TestPhotosDownloadHonorsHomeDirectoryIntent(t *testing.T) {
	home := setDownloadTestHome(t)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mediaItems/photo1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "photo1", "filename": "../photo.jpg", "baseUrl": srv.URL + "/media/photo",
			})
		case "/media/photo=d":
			_, _ = io.WriteString(w, "photo-proof")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := googleapi.NewPhotosClient(srv.Client(), googleapi.WithPhotosBaseURL(srv.URL))
	result := executeWithPhotosTestServices(t, []string{
		"--json", "--account", "a@example.com", "photos", "download", "photo1", "--out", "~/photos/new/",
	}, photosTestServices{Photos: fixedPhotosTestService(client)})
	if result.err != nil {
		t.Fatalf("download: %v", result.err)
	}
	want := filepath.Join(home, "photos", "new", "photo.jpg")
	var got struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil || got.Path != want {
		t.Fatalf("download output = %q, %v; want path %q", result.stdout, err, want)
	}
	if data, err := os.ReadFile(want); err != nil || string(data) != "photo-proof" {
		t.Fatalf("downloaded photo = %q, %v", data, err)
	}
}

func setDownloadTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}
