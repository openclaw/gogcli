package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDriveRevisionsListCmd_AllPagesJSON(t *testing.T) {
	var calls int
	svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.TrimPrefix(r.URL.Path, "/drive/v3") != "/files/file1/revisions" {
			http.NotFound(w, r)
			return
		}
		calls++
		requireQuery(t, r, "pageSize", "2")
		if fields := r.URL.Query().Get("fields"); !strings.Contains(fields, "exportLinks") {
			t.Fatalf("missing exportLinks field: %q", fields)
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			requireQuery(t, r, "pageToken", "")
			_, _ = io.WriteString(w, `{"revisions":[{"id":"r1"},{"id":"r2"}],"nextPageToken":"p2"}`)
			return
		}
		requireQuery(t, r, "pageToken", "p2")
		_, _ = io.WriteString(w, `{"revisions":[{"id":"r3"}]}`)
	}))
	t.Cleanup(closeSvc)
	stubDriveServiceForTest(t, svc)

	ctx := newDriveRevisionsTestContext(t, true, io.Discard, io.Discard)
	stdout := captureStdout(t, func() {
		cmd := &DriveRevisionsListCmd{}
		if err := runKong(t, cmd, []string{"file1", "--max", "2", "--all"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	var result struct {
		FileID        string            `json:"fileId"`
		Revisions     []*drive.Revision `json:"revisions"`
		NextPageToken string            `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	if result.FileID != "file1" || len(result.Revisions) != 3 || result.NextPageToken != "" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestDriveRevisionsListCmd_Text(t *testing.T) {
	svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"revisions":[{
				"id":"r1",
				"modifiedTime":"2026-06-10T12:34:56Z",
				"mimeType":"application/vnd.google-apps.document",
				"lastModifyingUser":{"displayName":"Alice"},
				"exportLinks":{"text/plain":"https://example.test/txt","application/pdf":"https://example.test/pdf"},
				"published":true
			}],
			"nextPageToken":"next"
		}`)
	}))
	t.Cleanup(closeSvc)
	stubDriveServiceForTest(t, svc)

	var stderr bytes.Buffer
	ctx := newDriveRevisionsTestContext(t, false, io.Discard, &stderr)
	stdout := captureStdout(t, func() {
		cmd := &DriveRevisionsListCmd{}
		if err := runKong(t, cmd, []string{"file1"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	for _, want := range []string{"ID  MODIFIED", "MIME", "r1", "Alice", "application/pdf,text/plain", "true"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("missing %q in %q", want, stdout)
		}
	}
	if !strings.Contains(stderr.String(), "--page next") {
		t.Fatalf("missing next-page hint: %q", stderr.String())
	}
}

func TestDriveRevisionsListCmd_EmptyJSON(t *testing.T) {
	svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(closeSvc)
	stubDriveServiceForTest(t, svc)

	ctx := newDriveRevisionsTestContext(t, true, io.Discard, io.Discard)
	stdout := captureStdout(t, func() {
		cmd := &DriveRevisionsListCmd{}
		if err := runKong(t, cmd, []string{"file1"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(stdout, `"revisions": []`) {
		t.Fatalf("expected empty array, got: %q", stdout)
	}
}

func TestDriveRevisionsGetCmd_JSONAndText(t *testing.T) {
	svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.TrimPrefix(r.URL.Path, "/drive/v3") != "/files/file1/revisions/r2" {
			http.NotFound(w, r)
			return
		}
		if fields := r.URL.Query().Get("fields"); !strings.Contains(fields, "exportLinks") {
			t.Fatalf("missing exportLinks field: %q", fields)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"r2",
			"modifiedTime":"2026-06-11T10:00:00Z",
			"mimeType":"application/vnd.google-apps.document",
			"lastModifyingUser":{"displayName":"Alice","emailAddress":"alice@example.com"},
			"exportLinks":{"text/plain":"https://example.test/txt","application/pdf":"https://example.test/pdf"}
		}`)
	}))
	t.Cleanup(closeSvc)
	stubDriveServiceForTest(t, svc)

	jsonCtx := newDriveRevisionsTestContext(t, true, io.Discard, io.Discard)
	jsonOut := captureStdout(t, func() {
		cmd := &DriveRevisionsGetCmd{}
		if err := runKong(t, cmd, []string{"file1", "r2"}, jsonCtx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("JSON Run: %v", err)
		}
	})
	var result struct {
		FileID   string          `json:"fileId"`
		Revision *drive.Revision `json:"revision"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("json: %v\n%s", err, jsonOut)
	}
	if result.FileID != "file1" || result.Revision == nil || result.Revision.Id != "r2" {
		t.Fatalf("unexpected result: %#v", result)
	}

	var textBuffer bytes.Buffer
	textCtx := newDriveRevisionsTestContext(t, false, &textBuffer, io.Discard)
	cmd := &DriveRevisionsGetCmd{}
	if err := runKong(t, cmd, []string{"file1", "r2"}, textCtx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("text Run: %v", err)
	}
	textOut := textBuffer.String()
	if strings.Index(textOut, "export.application/pdf") > strings.Index(textOut, "export.text/plain") {
		t.Fatalf("export links are not sorted: %q", textOut)
	}
	for _, want := range []string{"fileId\tfile1", "id\tr2", "userEmail\talice@example.com", "https://example.test/pdf"} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("missing %q in %q", want, textOut)
		}
	}
}

func TestDriveRevisionsValidationBeforeAuth(t *testing.T) {
	called := false
	original := newDriveService
	newDriveService = func(context.Context, string) (*drive.Service, error) {
		called = true
		return nil, errors.New("unexpected service call")
	}
	t.Cleanup(func() { newDriveService = original })

	ctx := newDriveRevisionsTestContext(t, false, io.Discard, io.Discard)
	if err := (&DriveRevisionsListCmd{FileID: "file1", Max: 0}).Run(ctx, &RootFlags{}); err == nil || !strings.Contains(err.Error(), "max must be > 0") {
		t.Fatalf("unexpected list error: %v", err)
	}
	if err := (&DriveRevisionsGetCmd{FileID: "file1"}).Run(ctx, &RootFlags{}); err == nil || !strings.Contains(err.Error(), "empty revisionId") {
		t.Fatalf("unexpected get error: %v", err)
	}
	if called {
		t.Fatal("validation should happen before auth")
	}
}

func newDriveRevisionsTestContext(t *testing.T, jsonMode bool, stdout, stderr io.Writer) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: stdout, Stderr: stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	return outfmt.WithMode(ctx, outfmt.Mode{JSON: jsonMode})
}
