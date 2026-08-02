package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func tempFilePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

// newGmailAttachmentTestServicePayload serves one attachment (id a1 on
// message m1) with the given messages.get payload.
func newGmailAttachmentTestServicePayload(t *testing.T, data []byte, payload map[string]any) *gmail.Service {
	t.Helper()
	encoded := base64.URLEncoding.EncodeToString(data)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1/attachments/a1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": encoded})
		case strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/m1") && !strings.Contains(r.URL.Path, "/attachments/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "m1", "payload": payload})
		default:
			http.NotFound(w, r)
		}
	})
	svc, closeServer := newGoogleTestService(t, handler, gmail.NewService)
	t.Cleanup(closeServer)
	return svc
}

// newGmailAttachmentTestService serves one attachment (id a1 on message m1)
// with the given payload metadata.
func newGmailAttachmentTestService(t *testing.T, data []byte, filename, mimeType string) *gmail.Service {
	t.Helper()
	return newGmailAttachmentTestServiceWithPayloadID(t, data, filename, mimeType, "a1")
}

// newGmailAttachmentTestServiceWithPayloadID lets the messages.get payload
// carry a different attachment ID than the one being downloaded, mimicking
// Gmail's unstable attachment IDs.
func newGmailAttachmentTestServiceWithPayloadID(t *testing.T, data []byte, filename, mimeType, payloadAttachmentID string) *gmail.Service {
	t.Helper()
	return newGmailAttachmentTestServicePayload(t, data, map[string]any{"parts": []map[string]any{{
		"filename": filename,
		"mimeType": mimeType,
		"body":     map[string]any{"attachmentId": payloadAttachmentID, "size": len(data)},
	}}})
}

func TestExecute_GmailAttachment_Inline_SmallAttachment_ReturnsBase64(t *testing.T) {
	data := []byte("hello inline attachment")
	svc := newGmailAttachmentTestService(t, data, "photo.png", "image/png")
	outPath := tempFilePath(t, "photo.png")

	parsed := executeGmailAttachmentJSON(t, svc,
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", outPath, "--inline",
	)

	got, ok := parsed["contentBase64"].(string)
	if !ok {
		t.Fatalf("contentBase64 missing or not string: %#v", parsed)
	}
	decoded, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("contentBase64 not decodable: %v", err)
	}
	if !bytes.Equal(decoded, data) {
		t.Fatalf("decoded=%q want=%q", decoded, data)
	}
	if parsed["mimeType"] != "image/png" {
		t.Fatalf("mimeType=%v", parsed["mimeType"])
	}
	if parsed["filename"] != "photo.png" {
		t.Fatalf("filename=%v", parsed["filename"])
	}
	if parsed["path"] != outPath {
		t.Fatalf("path=%v", parsed["path"])
	}
	if _, statErr := os.Stat(outPath); statErr != nil {
		t.Fatalf("file not written: %v", statErr)
	}
}

func TestExecute_GmailAttachment_Inline_Oversized_FallsBackToPathWithReason(t *testing.T) {
	// The limit is configured artificially low; the property under test is only
	// the boundary crossing (payload = limit+1), not any realistic size.
	const inlineLimit = 8
	data := bytes.Repeat([]byte("x"), inlineLimit+1)
	svc := newGmailAttachmentTestService(t, data, "big.bin", "application/octet-stream")
	outPath := tempFilePath(t, "big.bin")

	parsed := executeGmailAttachmentJSON(t, svc,
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", outPath, "--inline", "--inline-max-bytes", strconv.Itoa(inlineLimit),
	)

	if _, ok := parsed["contentBase64"]; ok {
		t.Fatalf("oversized attachment must not be inlined: %#v", parsed)
	}
	reason, _ := parsed["reason"].(string)
	if !strings.Contains(reason, "inline size limit") {
		t.Fatalf("reason=%q", reason)
	}
	if parsed["path"] != outPath {
		t.Fatalf("path=%v", parsed["path"])
	}
	st, statErr := os.Stat(outPath)
	if statErr != nil || st.Size() != int64(len(data)) {
		t.Fatalf("file not written: %v size=%v", statErr, st)
	}
}

func TestExecute_GmailAttachment_Inline_NegativeLimitRejected(t *testing.T) {
	result := executeWithTestRuntime(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", tempFilePath(t, "a.txt"), "--inline", "--inline-max-bytes=-1",
	}, nil)
	if result.err == nil {
		t.Fatalf("negative --inline-max-bytes must error, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "--inline-max-bytes must be non-negative") {
		t.Fatalf("err=%v", result.err)
	}
}

func TestExecute_GmailAttachment_Default_OutputUnchanged(t *testing.T) {
	data := []byte("plain old download")
	svc := newGmailAttachmentTestService(t, data, "a.txt", "text/plain")
	outPath := tempFilePath(t, "a.txt")

	parsed := executeGmailAttachmentJSON(t, svc,
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", outPath,
	)

	for _, key := range []string{"contentBase64", "reason", "mimeType", "filename"} {
		if _, ok := parsed[key]; ok {
			t.Fatalf("default output must not contain %q: %#v", key, parsed)
		}
	}
	if parsed["path"] != outPath || parsed["cached"] != false || parsed["bytes"] != float64(len(data)) {
		t.Fatalf("unexpected default output: %#v", parsed)
	}
}

func TestExecute_GmailAttachment_Inline_RefreshesSameSizeCache(t *testing.T) {
	fresh := []byte("fresh bytes")
	stale := []byte("stale bytes")
	if len(fresh) != len(stale) {
		t.Fatal("test fixture sizes must match")
	}
	var attachmentCalls int32
	svc := newGmailAttachmentCacheTestService(t, fresh, "a.txt", &attachmentCalls)
	outPath := tempFilePath(t, "a.txt")
	if err := os.WriteFile(outPath, stale, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	parsed := executeGmailAttachmentJSON(t, svc,
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", outPath, "--inline",
	)
	if parsed["cached"] != false {
		t.Fatalf("inline output must not use local cache: %#v", parsed)
	}
	got, err := base64.StdEncoding.DecodeString(parsed["contentBase64"].(string))
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	if !bytes.Equal(got, fresh) {
		t.Fatalf("content=%q want=%q", got, fresh)
	}
	written, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(written, fresh) {
		t.Fatalf("file=%q want=%q", written, fresh)
	}
	if attachmentCalls != 1 {
		t.Fatalf("attachment calls=%d want=1", attachmentCalls)
	}
}

func TestExecute_GmailAttachment_Inline_DryRunReportsMode(t *testing.T) {
	result := executeWithTestRuntime(t, []string{
		"--json", "--dry-run", "--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", tempFilePath(t, "a.txt"), "--inline", "--inline-max-bytes", "42",
	}, nil)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	var parsed struct {
		DryRun  bool           `json:"dry_run"`
		Request map[string]any `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("Unmarshal: %v\nstdout=%q", err, result.stdout)
	}
	if !parsed.DryRun || parsed.Request["inline"] != true || parsed.Request["inline_max_bytes"] != float64(42) {
		t.Fatalf("unexpected dry-run output: %#v", parsed)
	}
}

func TestExecute_GmailAttachment_Inline_PlainEscapesMetadata(t *testing.T) {
	svc := newGmailAttachmentTestService(t, []byte("x"), "unsafe\x1b[31m.txt", "text/plain")
	result := executeWithGmailTestService(t, []string{
		"--plain", "--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", tempFilePath(t, "a.txt"), "--inline",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	lines := strings.Split(strings.TrimSuffix(result.stdout, "\n"), "\n")
	if len(lines) != 6 {
		t.Fatalf("plain output lines=%d want=6\noutput=%q", len(lines), result.stdout)
	}
	if lines[3] != "filename\t\"unsafe\\x1b[31m.txt\"" {
		t.Fatalf("filename line=%q", lines[3])
	}
}

func TestExecute_GmailAttachment_Default_CacheRequiresExactAttachmentID(t *testing.T) {
	fresh := []byte("fresh bytes")
	stale := []byte("stale bytes")
	svc := newGmailAttachmentTestServiceWithPayloadID(t, fresh, "a.txt", "text/plain", "different-id")
	outPath := tempFilePath(t, "a.txt")
	if err := os.WriteFile(outPath, stale, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	parsed := executeGmailAttachmentJSON(t, svc,
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", outPath,
	)
	if parsed["cached"] != false {
		t.Fatalf("non-matching attachment ID must not use cache: %#v", parsed)
	}
	written, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(written, fresh) {
		t.Fatalf("file=%q want=%q", written, fresh)
	}
}

func TestExecute_GmailAttachment_Inline_MetadataAllowsSingleAttachmentFallback(t *testing.T) {
	svc := newGmailAttachmentTestServiceWithPayloadID(t, []byte("x"), "fallback.txt", "text/plain", "different-id")
	parsed := executeGmailAttachmentJSON(t, svc,
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
		"--out", tempFilePath(t, "a.txt"), "--inline",
	)
	if parsed["filename"] != "fallback.txt" || parsed["mimeType"] != "text/plain" {
		t.Fatalf("single-attachment metadata fallback missing: %#v", parsed)
	}
}
