package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chatAttachmentFixtureMessage(name string, attachments ...map[string]any) map[string]any {
	return map[string]any{
		"name":       name,
		"text":       "See attached",
		"createTime": "2026-01-02T03:04:05Z",
		"sender":     map[string]any{"name": "users/1", "displayName": "Test User", "type": "HUMAN"},
		"thread":     map[string]any{"name": "spaces/aaa/threads/m1"},
		"attachment": attachments,
	}
}

func chatUploadedAttachmentFixture(messageName string) map[string]any {
	return map[string]any{
		"name":              messageName + "/attachments/att1",
		"contentName":       "Screenshot.png",
		"contentType":       "image/png",
		"source":            "UPLOADED_CONTENT",
		"attachmentDataRef": map[string]any{"resourceName": messageName + "/attachments/att1"},
		"thumbnailUri":      "https://chat.google.com/thumb",
		"downloadUri":       "https://chat.google.com/dl",
	}
}

func TestExecute_ChatMessagesList_JSON_CarriesAttachments(t *testing.T) {
	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		if !(r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces/aaa/messages")) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"messages": []map[string]any{
				chatAttachmentFixtureMessage("spaces/aaa/messages/m1", chatUploadedAttachmentFixture("spaces/aaa/messages/m1")),
				{"name": "spaces/aaa/messages/m2", "text": "No attachment here", "createTime": "2026-01-02T03:04:00Z"},
			},
		})
	})

	result := executeWithChatTestService(t, []string{"--account", "a@b.com", "--json", "chat", "messages", "list", "spaces/aaa"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	var out struct {
		Messages []struct {
			Resource    string                `json:"resource"`
			Attachments []*chatAttachmentItem `json:"attachments"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &out); err != nil {
		t.Fatalf("parse stdout: %v\n%s", err, result.stdout)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("messages = %d, want 2\n%s", len(out.Messages), result.stdout)
	}
	atts := out.Messages[0].Attachments
	if len(atts) != 1 {
		t.Fatalf("attachments = %d, want 1\n%s", len(atts), result.stdout)
	}
	a := atts[0]
	if a.Name != "spaces/aaa/messages/m1/attachments/att1" || a.ContentName != "Screenshot.png" || a.ContentType != "image/png" ||
		a.Source != "UPLOADED_CONTENT" || a.AttachmentDataRef == nil || a.AttachmentDataRef.ResourceName != "spaces/aaa/messages/m1/attachments/att1" ||
		a.DriveDataRef != nil {
		t.Fatalf("attachment = %+v (ref=%+v)", a, a.AttachmentDataRef)
	}
	if out.Messages[1].Attachments != nil {
		t.Fatalf("message without attachments must omit the field:\n%s", result.stdout)
	}
	for _, leaked := range []string{"thumbnailUri", "downloadUri"} {
		if strings.Contains(result.stdout, leaked) {
			t.Fatalf("%s must not be projected:\n%s", leaked, result.stdout)
		}
	}
}

func TestExecute_ChatAttachmentsDownload_JSON(t *testing.T) {
	payload := []byte("\x89PNG\r\n\x1a\nfake-bytes-for-the-test")
	mediaCalls := 0
	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces/aaa/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(chatAttachmentFixtureMessage("spaces/aaa/messages/m1", chatUploadedAttachmentFixture("spaces/aaa/messages/m1")))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/media/spaces/aaa/messages/m1/attachments/att1") && r.URL.Query().Get("alt") == "media":
			mediaCalls++
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	})

	dir := t.TempDir()
	result := executeWithChatTestService(t, []string{"--account", "a@b.com", "--json", "chat", "attachments", "download", "spaces/aaa/messages/m1", "--out", dir + "/"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v (stderr=%q)", result.err, result.stderr)
	}
	var out struct {
		Message     string `json:"message"`
		Attachments []struct {
			Name        string `json:"name"`
			ContentName string `json:"contentName"`
			ContentType string `json:"contentType"`
			Source      string `json:"source"`
			Path        string `json:"path"`
			Bytes       int64  `json:"bytes"`
			Sha256      string `json:"sha256"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &out); err != nil {
		t.Fatalf("parse stdout: %v\n%s", err, result.stdout)
	}
	if out.Message != "spaces/aaa/messages/m1" || len(out.Attachments) != 1 {
		t.Fatalf("unexpected output:\n%s", result.stdout)
	}
	got := out.Attachments[0]
	wantPath := filepath.Join(dir, "Screenshot.png")
	sum := sha256.Sum256(payload)
	if got.Path != wantPath || got.Bytes != int64(len(payload)) || got.Sha256 != hex.EncodeToString(sum[:]) ||
		got.ContentName != "Screenshot.png" || got.ContentType != "image/png" || got.Source != "UPLOADED_CONTENT" {
		t.Fatalf("attachment row = %+v, want path=%s bytes=%d sha256=%s", got, wantPath, len(payload), hex.EncodeToString(sum[:]))
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("downloaded bytes differ")
	}
	if mediaCalls != 1 {
		t.Fatalf("media.download calls = %d, want 1", mediaCalls)
	}

	// a second run refuses to overwrite unless asked
	result = executeWithChatTestService(t, []string{"--account", "a@b.com", "--json", "chat", "attachments", "download", "spaces/aaa/messages/m1", "--out", dir + "/"}, svc)
	if result.err == nil {
		t.Fatalf("expected an error on an existing file without --overwrite")
	}
	result = executeWithChatTestService(t, []string{"--account", "a@b.com", "--json", "chat", "attachments", "download", "spaces/aaa/messages/m1", "--out", dir + "/", "--overwrite"}, svc)
	if result.err != nil {
		t.Fatalf("Execute with --overwrite: %v", result.err)
	}
}

func TestExecute_ChatAttachmentsDownload_DriveFileIsNotFetched(t *testing.T) {
	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/spaces/aaa/messages/m1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(chatAttachmentFixtureMessage("spaces/aaa/messages/m1",
				chatUploadedAttachmentFixture("spaces/aaa/messages/m1"),
				map[string]any{
					"name":         "spaces/aaa/messages/m1/attachments/d1",
					"contentName":  "Shared sheet",
					"contentType":  "application/vnd.google-apps.spreadsheet",
					"source":       "DRIVE_FILE",
					"driveDataRef": map[string]any{"driveFileId": "1AbCdEf"},
				}))
		case strings.Contains(r.URL.Path, "/media/"):
			t.Errorf("media.download must not be called for a Drive attachment: %s", r.URL.Path)
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	dir := t.TempDir()
	result := executeWithChatTestService(t, []string{"--account", "a@b.com", "--json", "chat", "attachments", "download", "spaces/aaa/messages/m1/attachments/d1", "--out", dir + "/"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	var out struct {
		Attachments []map[string]any `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &out); err != nil {
		t.Fatalf("parse stdout: %v\n%s", err, result.stdout)
	}
	if len(out.Attachments) != 1 || out.Attachments[0]["driveFileId"] != "1AbCdEf" || out.Attachments[0]["path"] != nil ||
		!strings.Contains(out.Attachments[0]["skipped"].(string), "gog drive download 1AbCdEf") {
		t.Fatalf("unexpected output:\n%s", result.stdout)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("nothing may be written for a Drive attachment, got %d file(s)", len(entries))
	}
}

func TestSplitChatAttachmentName(t *testing.T) {
	msg, att, err := splitChatAttachmentName("spaces/aaa/messages/m1.m1")
	if err != nil || msg != "spaces/aaa/messages/m1.m1" || att != "" {
		t.Fatalf("message name: %q %q %v", msg, att, err)
	}
	msg, att, err = splitChatAttachmentName(" spaces/aaa/messages/m1/attachments/x ")
	if err != nil || msg != "spaces/aaa/messages/m1" || att != "spaces/aaa/messages/m1/attachments/x" {
		t.Fatalf("attachment name: %q %q %v", msg, att, err)
	}
	for _, bad := range []string{"", "spaces/aaa", "spaces/aaa/threads/t1", "spaces/aaa/messages/m1/reactions/r1", "spaces//messages/m1", "m1"} {
		if _, _, err := splitChatAttachmentName(bad); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}
