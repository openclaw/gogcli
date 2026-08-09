package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGmailThreadGetAndAttachments_JSON(t *testing.T) {
	attachmentData := base64.RawURLEncoding.EncodeToString([]byte("payload"))
	threadResp := map[string]any{
		"id": "t1",
		"messages": []map[string]any{
			{
				"id": "m1",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "a@example.com"},
						{"name": "To", "value": "b@example.com"},
						{"name": "Subject", "value": "Hi"},
						{"name": "Date", "value": "Mon, 1 Jan 2025 00:00:00 +0000"},
					},
					"mimeType": "multipart/mixed",
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body": map[string]any{
								"data": base64.RawURLEncoding.EncodeToString([]byte("hello")),
							},
						},
						{
							"filename": "note.txt",
							"mimeType": "text/plain",
							"body": map[string]any{
								"attachmentId": "att1",
								"size":         7,
							},
						},
					},
				},
			},
		},
	}
	emptyThreadResp := map[string]any{
		"id":       "empty",
		"messages": []map[string]any{},
	}
	noAttsThreadResp := map[string]any{
		"id": "noatts",
		"messages": []map[string]any{
			{
				"id": "m2",
				"payload": map[string]any{
					"mimeType": "text/plain",
					"body": map[string]any{
						"data": base64.RawURLEncoding.EncodeToString([]byte("hello")),
					},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/threads/t1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(threadResp)
			return
		case r.Method == http.MethodGet && path == "/users/me/threads/empty":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(emptyThreadResp)
			return
		case r.Method == http.MethodGet && path == "/users/me/threads/noatts":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(noAttsThreadResp)
			return
		case r.Method == http.MethodGet && path == "/users/me/messages/m1/attachments/att1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": attachmentData,
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	outDir := t.TempDir()
	getResult := executeWithGmailTestService(t, []string{"--json", "--account", "a@b.com", "gmail", "thread", "get", "t1", "--download", "--out-dir", outDir}, svc)
	if getResult.err != nil {
		t.Fatalf("Execute thread get: %v\nstderr=%q", getResult.err, getResult.stderr)
	}

	var payload struct {
		Thread     map[string]any   `json:"thread"`
		Downloaded []map[string]any `json:"downloaded"`
	}
	if err := json.Unmarshal([]byte(getResult.stdout), &payload); err != nil {
		t.Fatalf("decode thread json: %v", err)
	}
	if payload.Thread == nil || len(payload.Downloaded) != 1 {
		t.Fatalf("unexpected thread payload: %#v", payload)
	}
	path, ok := payload.Downloaded[0]["path"].(string)
	if !ok || path == "" {
		t.Fatalf("expected download path, got: %#v", payload.Downloaded)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected downloaded file: %v", statErr)
	}

	indexedResult := executeWithGmailTestService(t, []string{
		"--json", "--account", "a@b.com", "gmail", "thread", "get", "t1", "--use-indexed-attachment-ids",
	}, svc)
	if indexedResult.err != nil {
		t.Fatalf("Execute indexed thread get: %v\nstderr=%q", indexedResult.err, indexedResult.stderr)
	}
	var indexedPayload struct {
		Attachments []struct {
			MessageID       string `json:"messageId"`
			AttachmentIndex *int   `json:"attachmentIndex"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(indexedResult.stdout), &indexedPayload); err != nil {
		t.Fatalf("decode indexed thread json: %v", err)
	}
	if len(indexedPayload.Attachments) != 1 || indexedPayload.Attachments[0].MessageID != "m1" ||
		indexedPayload.Attachments[0].AttachmentIndex == nil || *indexedPayload.Attachments[0].AttachmentIndex != 0 {
		t.Fatalf("unexpected indexed thread attachments: %#v", indexedPayload.Attachments)
	}
	if strings.Contains(indexedResult.stdout, "att1") || strings.Contains(indexedResult.stdout, "attachmentId") {
		t.Fatalf("indexed thread output leaked opaque id: %q", indexedResult.stdout)
	}

	attachmentsResult := executeWithGmailTestService(t, []string{"--json", "--account", "a@b.com", "gmail", "thread", "attachments", "t1"}, svc)
	if attachmentsResult.err != nil {
		t.Fatalf("Execute attachments: %v\nstderr=%q", attachmentsResult.err, attachmentsResult.stderr)
	}
	var attachments struct {
		ThreadID    string           `json:"threadId"`
		Attachments []map[string]any `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(attachmentsResult.stdout), &attachments); err != nil {
		t.Fatalf("decode attachments json: %v", err)
	}
	if attachments.ThreadID != "t1" || len(attachments.Attachments) != 1 {
		t.Fatalf("unexpected attachments payload: %#v", attachments)
	}
	if attachments.Attachments[0]["filename"] != "note.txt" {
		t.Fatalf("unexpected attachment filename: %#v", attachments.Attachments[0])
	}

	attachmentsDownloadResult := executeWithGmailTestService(t, []string{"--json", "--account", "a@b.com", "gmail", "thread", "attachments", "t1", "--download", "--out-dir", outDir}, svc)
	if attachmentsDownloadResult.err != nil {
		t.Fatalf("Execute attachments download: %v\nstderr=%q", attachmentsDownloadResult.err, attachmentsDownloadResult.stderr)
	}
	var attachmentsDownloaded struct {
		Attachments []map[string]any `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(attachmentsDownloadResult.stdout), &attachmentsDownloaded); err != nil {
		t.Fatalf("decode attachments download: %v", err)
	}
	if len(attachmentsDownloaded.Attachments) != 1 {
		t.Fatalf("unexpected download attachments: %#v", attachmentsDownloaded.Attachments)
	}
	if _, ok := attachmentsDownloaded.Attachments[0]["path"]; !ok {
		t.Fatalf("expected download path in attachments: %#v", attachmentsDownloaded.Attachments[0])
	}

	plainOutDir := t.TempDir()
	plainDownloadResult := executeWithGmailTestService(t, []string{"--plain", "--account", "a@b.com", "gmail", "thread", "attachments", "t1", "--download", "--out-dir", plainOutDir}, svc)
	if plainDownloadResult.err != nil {
		t.Fatalf("Execute attachments download plain: %v\nstderr=%q", plainDownloadResult.err, plainDownloadResult.stderr)
	}
	if !strings.Contains(plainDownloadResult.stdout, "Saved") {
		t.Fatalf("unexpected download output: %q", plainDownloadResult.stdout)
	}

	cachedResult := executeWithGmailTestService(t, []string{"--plain", "--account", "a@b.com", "gmail", "thread", "attachments", "t1", "--download", "--out-dir", plainOutDir}, svc)
	if cachedResult.err != nil {
		t.Fatalf("Execute attachments cached: %v\nstderr=%q", cachedResult.err, cachedResult.stderr)
	}
	if !strings.Contains(cachedResult.stdout, "Cached") {
		t.Fatalf("unexpected cached output: %q", cachedResult.stdout)
	}

	// Ensure path is within the requested output dir when downloading attachments.
	if !strings.HasPrefix(path, filepath.Clean(outDir)+string(os.PathSeparator)) {
		t.Fatalf("unexpected download path: %s", path)
	}

	plainResult := executeWithGmailTestService(t, []string{"--plain", "--account", "a@b.com", "gmail", "thread", "get", "t1"}, svc)
	if plainResult.err != nil {
		t.Fatalf("Execute thread get plain: %v\nstderr=%q", plainResult.err, plainResult.stderr)
	}
	if !strings.Contains(plainResult.stdout, "Thread contains") {
		t.Fatalf("unexpected plain output: %q", plainResult.stdout)
	}

	emptyResult := executeWithGmailTestService(t, []string{"--plain", "--account", "a@b.com", "gmail", "thread", "get", "empty"}, svc)
	if emptyResult.err != nil {
		t.Fatalf("Execute empty thread: %v\nstderr=%q", emptyResult.err, emptyResult.stderr)
	}
	if !strings.Contains(emptyResult.stderr, "Empty thread") {
		t.Fatalf("unexpected empty thread stderr: %q", emptyResult.stderr)
	}

	noAttsResult := executeWithGmailTestService(t, []string{"--plain", "--account", "a@b.com", "gmail", "thread", "attachments", "noatts"}, svc)
	if noAttsResult.err != nil {
		t.Fatalf("Execute no attachments: %v\nstderr=%q", noAttsResult.err, noAttsResult.stderr)
	}
	if !strings.Contains(noAttsResult.stdout, "No attachments found") {
		t.Fatalf("unexpected no attachments output: %q", noAttsResult.stdout)
	}

	emptyAttachResult := executeWithGmailTestService(t, []string{"--json", "--account", "a@b.com", "gmail", "thread", "attachments", "empty"}, svc)
	if emptyAttachResult.err != nil {
		t.Fatalf("Execute empty attachments json: %v\nstderr=%q", emptyAttachResult.err, emptyAttachResult.stderr)
	}
	if !strings.Contains(emptyAttachResult.stdout, "\"attachments\"") {
		t.Fatalf("unexpected empty attachments output: %q", emptyAttachResult.stdout)
	}

	noAttsJSONResult := executeWithGmailTestService(t, []string{"--json", "--account", "a@b.com", "gmail", "thread", "attachments", "noatts"}, svc)
	if noAttsJSONResult.err != nil {
		t.Fatalf("Execute no attachments json: %v\nstderr=%q", noAttsJSONResult.err, noAttsJSONResult.stderr)
	}
	var noAttsJSON struct {
		Attachments []map[string]any `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(noAttsJSONResult.stdout), &noAttsJSON); err != nil {
		t.Fatalf("decode no attachments json: %v", err)
	}
	if noAttsJSON.Attachments == nil {
		t.Fatalf("attachments is nil in output: %s", noAttsJSONResult.stdout)
	}
}
