package cmd

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
)

func TestDriveExportMimeTypeForFormat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		googleMime  string
		format      string
		wantMime    string
		wantErrFrag string
	}{
		{
			name:       "doc_default",
			googleMime: "application/vnd.google-apps.document",
			format:     "",
			wantMime:   "application/pdf",
		},
		{
			name:       "doc_pdf",
			googleMime: "application/vnd.google-apps.document",
			format:     "pdf",
			wantMime:   "application/pdf",
		},
		{
			name:       "doc_docx",
			googleMime: "application/vnd.google-apps.document",
			format:     "docx",
			wantMime:   "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			name:       "doc_txt",
			googleMime: "application/vnd.google-apps.document",
			format:     "txt",
			wantMime:   "text/plain",
		},
		{
			name:        "doc_invalid",
			googleMime:  "application/vnd.google-apps.document",
			format:      "xlsx",
			wantErrFrag: "Google Doc",
		},

		{
			name:       "sheet_default",
			googleMime: "application/vnd.google-apps.spreadsheet",
			format:     "",
			wantMime:   "text/csv",
		},
		{
			name:       "sheet_pdf",
			googleMime: "application/vnd.google-apps.spreadsheet",
			format:     "pdf",
			wantMime:   "application/pdf",
		},
		{
			name:       "sheet_csv",
			googleMime: "application/vnd.google-apps.spreadsheet",
			format:     "csv",
			wantMime:   "text/csv",
		},
		{
			name:       "sheet_xlsx",
			googleMime: "application/vnd.google-apps.spreadsheet",
			format:     "xlsx",
			wantMime:   "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			name:        "sheet_invalid",
			googleMime:  "application/vnd.google-apps.spreadsheet",
			format:      "pptx",
			wantErrFrag: "Google Sheet",
		},

		{
			name:       "slides_default",
			googleMime: "application/vnd.google-apps.presentation",
			format:     "",
			wantMime:   "application/pdf",
		},
		{
			name:       "slides_pdf",
			googleMime: "application/vnd.google-apps.presentation",
			format:     "pdf",
			wantMime:   "application/pdf",
		},
		{
			name:       "slides_pptx",
			googleMime: "application/vnd.google-apps.presentation",
			format:     "pptx",
			wantMime:   "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		},
		{
			name:        "slides_invalid",
			googleMime:  "application/vnd.google-apps.presentation",
			format:      "xlsx",
			wantErrFrag: "Google Slides",
		},

		{
			name:       "drawing_default",
			googleMime: "application/vnd.google-apps.drawing",
			format:     "",
			wantMime:   "image/png",
		},
		{
			name:       "drawing_png",
			googleMime: "application/vnd.google-apps.drawing",
			format:     "png",
			wantMime:   "image/png",
		},
		{
			name:       "drawing_pdf",
			googleMime: "application/vnd.google-apps.drawing",
			format:     "pdf",
			wantMime:   "application/pdf",
		},
		{
			name:        "drawing_invalid",
			googleMime:  "application/vnd.google-apps.drawing",
			format:      "txt",
			wantErrFrag: "Google Drawing",
		},

		{
			name:       "unknown_google_default",
			googleMime: "application/vnd.google-apps.form",
			format:     "",
			wantMime:   "application/pdf",
		},
		{
			name:       "unknown_google_pdf",
			googleMime: "application/vnd.google-apps.form",
			format:     "pdf",
			wantMime:   "application/pdf",
		},
		{
			name:        "unknown_google_invalid",
			googleMime:  "application/vnd.google-apps.form",
			format:      "xlsx",
			wantErrFrag: "file type",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := driveExportMimeTypeForFormat(tc.googleMime, tc.format)
			if tc.wantErrFrag != "" {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !strings.Contains(err.Error(), tc.wantErrFrag) {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantMime {
				t.Fatalf("mimeType=%q want %q", got, tc.wantMime)
			}
		})
	}
}

func TestDownloadDriveFile_InvalidExportFormat(t *testing.T) {
	t.Parallel()

	origExport := driveExportDownload
	t.Cleanup(func() { driveExportDownload = origExport })

	called := false
	driveExportDownload = func(context.Context, *drive.Service, string, string) (*http.Response, error) {
		called = true
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	}

	dest := filepath.Join(t.TempDir(), "out")
	_, _, err := downloadDriveFile(context.Background(), &drive.Service{}, &drive.File{
		Id:       "id1",
		MimeType: "application/vnd.google-apps.document",
	}, dest, "xlsx")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "Google Doc") {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatalf("export should not be called on validation error")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("expected no file written, stat=%v", statErr)
	}
}
