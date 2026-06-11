package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

func TestDocsInsertImageResolveSourceURL(t *testing.T) {
	tests := []struct {
		name string
		cmd  DocsInsertImageCmd
		want string
	}{
		{name: "missing", cmd: DocsInsertImageCmd{}, want: "required: --file or --url"},
		{name: "both", cmd: DocsInsertImageCmd{File: "image.png", URL: "https://example.com/image.png"}, want: "mutually exclusive"},
		{name: "http", cmd: DocsInsertImageCmd{URL: "http://example.com/image.png"}, want: "public HTTPS"},
		{name: "relative", cmd: DocsInsertImageCmd{URL: "image.png"}, want: "public HTTPS"},
		{name: "credentials", cmd: DocsInsertImageCmd{URL: "https://user:pass@example.com/image.png"}, want: "without embedded credentials"},
		{name: "parent", cmd: DocsInsertImageCmd{URL: "https://example.com/image.png", Parent: "folder1"}, want: "require --file"},
		{name: "name", cmd: DocsInsertImageCmd{URL: "https://example.com/image.png", Name: "image.png"}, want: "require --file"},
		{name: "restricted fallback", cmd: DocsInsertImageCmd{URL: "https://example.com/image.png", OnRestricted: "link"}, want: "require --file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.cmd.resolveSource()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("resolveSource() error = %v, want %q", err, tt.want)
			}
		})
	}

	source, err := (&DocsInsertImageCmd{URL: "https://example.com/image.png?sig=abc"}).resolveSource()
	if err != nil {
		t.Fatalf("resolveSource valid URL: %v", err)
	}
	if source.imageURL != "https://example.com/image.png?sig=abc" || source.localPath != "" {
		t.Fatalf("unexpected URL source: %#v", source)
	}
}

func TestDocsInsertImageURLRunSkipsDrive(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})

	var got docs.BatchUpdateDocumentRequest
	docSvc, cleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/documents/"):
			_ = json.NewEncoder(w).Encode(docBodyWithText("before\n"))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer cleanup()
	newDocsService = func(context.Context, string) (*docs.Service, error) { return docSvc, nil }
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		t.Fatal("URL insertion must not create a Drive service")
		return nil, errors.New("unexpected Drive service call")
	}

	var runErr error
	out := captureStdout(t, func() {
		runErr = runKong(t, &DocsInsertImageCmd{}, []string{
			"doc1",
			"--url", "https://example.com/image.png?sig=abc",
			"--width", "320",
			"--height", "180",
		}, newDocsJSONContext(t), &RootFlags{Account: "a@b.com"})
	})
	if runErr != nil {
		t.Fatalf("docs insert-image --url: %v", runErr)
	}
	if len(got.Requests) != 1 || got.Requests[0].InsertInlineImage == nil {
		t.Fatalf("unexpected batch requests: %#v", got.Requests)
	}
	insert := got.Requests[0].InsertInlineImage
	if insert.Uri != "https://example.com/image.png?sig=abc" || insert.Location.Index != 7 {
		t.Fatalf("unexpected inline image request: %#v", insert)
	}
	if insert.ObjectSize.Width.Magnitude != 320 || insert.ObjectSize.Height.Magnitude != 180 {
		t.Fatalf("unexpected inline image size: %#v", insert.ObjectSize)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if payload["documentId"] != "doc1" || payload["url"] != "https://example.com/image.png?sig=abc" {
		t.Fatalf("unexpected output: %#v", payload)
	}
	if _, ok := payload["uploadedFileId"]; ok {
		t.Fatalf("URL output must not contain upload metadata: %#v", payload)
	}
}

func TestDocsInsertImageURLDryRunSkipsServices(t *testing.T) {
	origDocs := newDocsService
	origDrive := newDriveService
	t.Cleanup(func() {
		newDocsService = origDocs
		newDriveService = origDrive
	})
	newDocsService = func(context.Context, string) (*docs.Service, error) {
		t.Fatal("dry-run must not create a Docs service")
		return nil, errors.New("unexpected Docs service call")
	}
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		t.Fatal("dry-run must not create a Drive service")
		return nil, errors.New("unexpected Drive service call")
	}

	out := captureStdout(t, func() {
		err := runKong(t, &DocsInsertImageCmd{}, []string{
			"doc1",
			"--url", "https://example.com/image.png",
		}, newDocsJSONContext(t), &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 0 {
			t.Fatalf("dry-run error = %v", err)
		}
	})
	var payload struct {
		Op      string `json:"op"`
		Request struct {
			URL string `json:"url"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, out)
	}
	if payload.Op != "docs.insert-image" || payload.Request.URL != "https://example.com/image.png" {
		t.Fatalf("unexpected dry-run output: %#v", payload)
	}
}
