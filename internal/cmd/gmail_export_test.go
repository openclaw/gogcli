package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func TestGmailExportPrivacyBeforeWrite(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(dest, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("privacy unavailable")
	err := writeFileAtomicWithPrivacy(dest, []byte("private bytes"), func(path string) error {
		data, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if len(data) != 0 {
			t.Fatal("data written before privacy established")
		}
		return denied
	})
	if !errors.Is(err, denied) {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "original" {
		t.Fatal(string(data), err)
	}
}

func TestGmailExportRawExactBytes(t *testing.T) {
	want := []byte("From: fixture@example.invalid\r\nSubject: exact\r\n\r\n\x00\xff\r\n\n")
	svc, closeServer := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gmail/v1/users/me/messages/m1" || r.URL.Query().Get("format") != "raw" {
			t.Errorf("wrong raw request: %s", r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "m1", "threadId": "t1", "raw": base64.RawURLEncoding.EncodeToString(want)})
	}), gmail.NewService)
	defer closeServer()
	got, info, err := fetchGmailExport(context.Background(), svc, gmailExportKey{Kind: "raw", MessageID: "m1"}, 1024)
	if err != nil || !bytes.Equal(got, want) || info.ThreadID != "t1" || info.MessageID != "m1" || info.Size != int64(len(want)) {
		t.Fatalf("raw export=%q info=%+v err=%v", got, info, err)
	}
}

func TestGmailExportAttachmentExactBytes(t *testing.T) {
	want := []byte{0, 255, 13, 10, 10}
	for _, padded := range []bool{false, true} {
		enc := base64.RawURLEncoding
		if padded {
			enc = base64.URLEncoding
		}
		svc, closeServer := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/gmail/v1/users/me/messages/m1/attachments/a1" {
				t.Errorf("wrong attachment request: %s", r.URL)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": enc.EncodeToString(want), "size": 5})
		}), gmail.NewService)
		got, info, err := fetchGmailExport(context.Background(), svc, gmailExportKey{Kind: "attachment", MessageID: "m1", AttachmentID: "a1"}, 5)
		closeServer()
		if err != nil || !bytes.Equal(got, want) || info.Size != 5 || info.AttachmentID != "a1" {
			t.Fatalf("attachment=%q info=%+v err=%v", got, info, err)
		}
	}
}

func TestGmailExportRejectsOversize(t *testing.T) {
	for _, size := range []int{8, 9} {
		encoded := base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{255}, size))
		got, err := decodeGmailExport(encoded, 8)
		if size == 8 && (err != nil || !bytes.Equal(got, bytes.Repeat([]byte{255}, 8))) {
			t.Fatalf("exact limit: %v", err)
		}
		if size == 9 && (err == nil || got != nil) {
			t.Fatal("oversized decode published bytes")
		}
	}
}

func TestGmailExportHardLimit(t *testing.T) {
	const limit = 52428800
	data := bytes.Repeat([]byte{255}, limit)
	got, err := decodeGmailExport(base64.RawURLEncoding.EncodeToString(data), limit)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("50 MiB boundary rejected: %v", err)
	}
	if got, err := decodeGmailExport(base64.RawURLEncoding.EncodeToString(append(data, 0)), limit); err == nil || got != nil {
		t.Fatal("50 MiB + 1 accepted")
	}
	if _, err := decodeGmailExport("Zg==", limit+1); err == nil {
		t.Fatal("configured hard limit widened")
	}
}

func TestGmailExportMalformedBase64(t *testing.T) {
	for _, encoded := range []string{"Zg=", "Zg===", "Zg==junk", "Zh", "Zg\n", "Zg\r", "Zg ", "+/8=", "a", "=Zg=", "Zg==\x00"} {
		if _, err := decodeGmailExport(encoded, 8); err == nil {
			t.Errorf("accepted invalid encoding %q", encoded)
		}
	}
}

func TestGmailExportAtomicFailure(t *testing.T) {
	setTestConfigHome(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "mail.bin")
	if err := os.WriteFile(dest, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, closeServer := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": "AP8NCgo=", "size": 6})
	}), gmail.NewService)
	defer closeServer()
	runtime := &app.Runtime{Services: app.Services{Gmail: func(context.Context, string) (*gmail.Service, error) { return svc, nil }}}
	result := executeWithTestRuntime(t, []string{"--json", "--account", "fixture@example.invalid", "gmail", "export", "attachment", "m1", "a1", "--out", dest}, runtime)
	if result.err == nil || !strings.Contains(result.err.Error(), "size") {
		t.Fatalf("expected provider size mismatch, got %v", result.err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "original" {
		t.Fatalf("existing output changed: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("partial file left behind: %v, %v", entries, err)
	}
}

func TestGmailExportCLIFileAndMetadata(t *testing.T) {
	setTestConfigHome(t)
	want := []byte{0, 255, 13, 10, 10}
	svc, closeServer := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "m1", "threadId": "t1", "raw": "AP8NCgo"})
	}), gmail.NewService)
	defer closeServer()
	runtime := &app.Runtime{Services: app.Services{Gmail: func(context.Context, string) (*gmail.Service, error) { return svc, nil }}}
	dest := filepath.Join(t.TempDir(), "mail.bin")
	result := executeWithTestRuntime(t, []string{"--json", "--account", "fixture@example.invalid", "--wrap-untrusted", "gmail", "export", "raw", "m1", "--out", dest}, runtime)
	if result.err != nil {
		t.Fatal(result.err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("file bytes=%q, %v", got, err)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["size"] != float64(5) || meta["message_id"] != "m1" || meta["thread_id"] != "t1" || len(meta) != 3 {
		t.Fatalf("wrong bounded metadata: %s", result.stdout)
	}
}

func FuzzGmailExportBase64RoundTrip(f *testing.F) {
	f.Add([]byte{0, 255, 13, 10, 10}, uint16(8), false)
	f.Add([]byte{}, uint16(1), true)
	f.Add([]byte("mail\r\n"), uint16(4), true)
	f.Fuzz(func(t *testing.T, data []byte, configured uint16, padded bool) {
		limit := int64(configured) + 1
		enc := base64.RawURLEncoding
		if padded {
			enc = base64.URLEncoding
		}
		got, err := decodeGmailExport(enc.EncodeToString(data), limit)
		if int64(len(data)) > limit {
			if err == nil || got != nil {
				t.Fatal("size limit bypassed")
			}
		} else if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("exact round trip failed: %v", err)
		}
	})
}
