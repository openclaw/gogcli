package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

func TestGmailURLCmd_TextAndJSON(t *testing.T) {
	textOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		if err := (&GmailURLCmd{ThreadIDs: []string{"t1"}}).Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("text run: %v", err)
		}
	})
	if !strings.Contains(textOut, "t1") || !strings.Contains(textOut, "mail.google.com") {
		t.Fatalf("unexpected text output: %q", textOut)
	}

	jsonOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		jsonCtx := outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
		if err := (&GmailURLCmd{ThreadIDs: []string{"t2"}}).Run(jsonCtx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("json run: %v", err)
		}
	})
	var payload struct {
		URLs []map[string]string `json:"urls"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(payload.URLs) != 1 || payload.URLs[0]["id"] != "t2" {
		t.Fatalf("unexpected json payload: %#v", payload)
	}
}

func TestGmailURLCmd_MissingAccount(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	if err := (&GmailURLCmd{ThreadIDs: []string{"t1"}}).Run(context.Background(), &RootFlags{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCollectAttachmentsNil(t *testing.T) {
	if collectAttachments(nil) != nil {
		t.Fatalf("expected nil attachments")
	}
}

func TestDownloadAttachment_ErrorsAndSafeFilename(t *testing.T) {
	if _, _, err := downloadAttachment(context.Background(), nil, "", attachmentInfo{AttachmentID: "a"}, ".", false); err == nil {
		t.Fatalf("expected missing messageID error")
	}

	dir := t.TempDir()
	att := attachmentInfo{
		Filename:     "..",
		Size:         4,
		AttachmentID: "attachment1234567",
	}
	shortID := att.AttachmentID[:8]
	expectedPath := filepath.Join(dir, "m1_"+shortID+"_attachment")
	if err := os.WriteFile(expectedPath, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	path, cached, err := downloadAttachment(context.Background(), nil, "m1", att, dir, false)
	if err != nil {
		t.Fatalf("downloadAttachment: %v", err)
	}
	if path != expectedPath || !cached {
		t.Fatalf("unexpected download result: path=%q cached=%v", path, cached)
	}
}

func TestDownloadAttachment_IndexedFilenameUsesIndex(t *testing.T) {
	dir := t.TempDir()
	att := attachmentInfo{
		Filename:        "report.pdf",
		Size:            4,
		AttachmentID:    "attachment1234567",
		AttachmentIndex: 2,
	}
	// In indexed mode the saved filename embeds the 0-based index, not the id.
	expectedPath := filepath.Join(dir, "m1_2_report.pdf")
	if err := os.WriteFile(expectedPath, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	path, cached, err := downloadAttachment(context.Background(), nil, "m1", att, dir, true)
	if err != nil {
		t.Fatalf("downloadAttachment: %v", err)
	}
	if path != expectedPath || !cached {
		t.Fatalf("unexpected download result: path=%q cached=%v", path, cached)
	}
}

func TestDownloadAttachment_ServiceError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	att := attachmentInfo{
		Filename:     "file.txt",
		Size:         1,
		AttachmentID: "att1",
	}
	if _, _, err := downloadAttachment(context.Background(), svc, "m1", att, t.TempDir(), false); err == nil {
		t.Fatalf("expected error")
	}
}
