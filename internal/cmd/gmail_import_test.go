package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
)

const testImportEML = "From: Sender <sender@example.com>\r\n" +
	"To: Receiver <receiver@example.com>\r\n" +
	"Subject: Import test\r\n" +
	"Date: Wed, 06 Aug 2026 12:34:56 +0000\r\n" +
	"Message-ID: <import-test@example.com>\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"synthetic body\r\n"

func TestGmailImportDryRunParsesFileWithoutAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "message.eml")
	if err := os.WriteFile(path, []byte(testImportEML), 0o600); err != nil {
		t.Fatalf("write EML: %v", err)
	}

	result := executeWithTestRuntime(t, []string{
		"--dry-run", "--json", "gmail", "import", path,
		"--label", "Inbox/Test", "--internal-date-source", "receivedTime",
		"--never-mark-spam", "--process-for-calendar",
	}, &app.Runtime{})
	if result.err != nil {
		t.Fatalf("dry-run import: %v\nstderr=%s", result.err, result.stderr)
	}

	var got struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			Bytes              int      `json:"bytes"`
			Subject            string   `json:"subject"`
			MessageID          string   `json:"message_id"`
			Labels             []string `json:"labels"`
			InternalDateSource string   `json:"internal_date_source"`
			NeverMarkSpam      bool     `json:"never_mark_spam"`
			ProcessForCalendar bool     `json:"process_for_calendar"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, result.stdout)
	}
	if !got.DryRun || got.Op != "gmail.import" || got.Request.Bytes != len(testImportEML) {
		t.Fatalf("unexpected dry-run envelope: %#v", got)
	}
	if got.Request.Subject != "Import test" || got.Request.MessageID != "<import-test@example.com>" {
		t.Fatalf("unexpected parsed headers: %#v", got.Request)
	}
	if len(got.Request.Labels) != 1 || got.Request.Labels[0] != "Inbox/Test" || got.Request.InternalDateSource != "receivedTime" || !got.Request.NeverMarkSpam || !got.Request.ProcessForCalendar {
		t.Fatalf("unexpected options: %#v", got.Request)
	}
}

func TestGmailImportDryRunReadsStdin(t *testing.T) {
	var out bytes.Buffer
	ctx := newCmdRuntimeIOContext(t, strings.NewReader(testImportEML), &out, io.Discard)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	err := (&GmailImportCmd{File: "-", InternalDateSource: "dateHeader"}).Run(ctx, &RootFlags{DryRun: true})
	if ExitCode(err) != 0 {
		t.Fatalf("stdin dry-run: %v", err)
	}
	var got struct {
		Request struct {
			Source  string `json:"source"`
			Subject string `json:"subject"`
		} `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.Request.Source != "-" || got.Request.Subject != "Import test" {
		t.Fatalf("unexpected stdin dry-run: err=%v value=%#v output=%s", err, got, out.String())
	}
}

func TestGmailImportRejectsInvalidRFC822(t *testing.T) {
	ctx := newCmdRuntimeIOContext(t, strings.NewReader("not a header\r\n\r\nbody"), io.Discard, io.Discard)
	err := (&GmailImportCmd{File: "-", InternalDateSource: "dateHeader"}).Run(ctx, &RootFlags{DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "invalid RFC822/EML input") {
		t.Fatalf("error = %v, want invalid RFC822/EML input", err)
	}
}

func TestGmailImportUploadsMessageAndOptions(t *testing.T) {
	var imported []byte
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/labels":
			_ = json.NewEncoder(w).Encode(map[string]any{"labels": []map[string]string{{"id": "Label_42", "name": "Imported"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/upload/gmail/v1/users/me/messages/import":
			if got := r.URL.Query().Get("internalDateSource"); got != "receivedTime" {
				t.Errorf("internalDateSource = %q", got)
			}
			if r.URL.Query().Get("neverMarkSpam") != "true" || r.URL.Query().Get("processForCalendar") != "true" {
				t.Errorf("missing import boolean options: %s", r.URL.RawQuery)
			}
			mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "multipart/related" {
				t.Errorf("content type = %q, err=%v", mediaType, err)
				http.Error(w, "bad multipart", http.StatusBadRequest)
				return
			}
			reader := multipart.NewReader(r.Body, params["boundary"])
			metadataPart, err := reader.NextPart()
			if err != nil {
				t.Errorf("metadata part: %v", err)
				return
			}
			var metadata gmail.Message
			if decodeErr := json.NewDecoder(metadataPart).Decode(&metadata); decodeErr != nil {
				t.Errorf("decode metadata: %v", decodeErr)
				return
			}
			if len(metadata.LabelIds) != 1 || metadata.LabelIds[0] != "Label_42" {
				t.Errorf("label IDs = %v", metadata.LabelIds)
			}
			messagePart, err := reader.NextPart()
			if err != nil {
				t.Errorf("message part: %v", err)
				return
			}
			imported, err = io.ReadAll(messagePart)
			if err != nil {
				t.Errorf("read message part: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "m-imported", "threadId": "t-imported", "labelIds": []string{"Label_42"}, "internalDate": "1786019696000",
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	var out bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &out, io.Discard)
	ctx = withGmailTestService(ctx, svc)
	err := (&GmailImportCmd{
		File: testWriteImportEML(t), Labels: []string{"Imported"}, InternalDateSource: "receivedTime",
		NeverMarkSpam: true, ProcessForCalendar: true,
	}).Run(ctx, &RootFlags{Account: "test@example.com"})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if string(imported) != testImportEML {
		t.Fatalf("imported bytes changed:\n%q", imported)
	}
	var got struct {
		MessageID string `json:"messageId"`
		ThreadID  string `json:"threadId"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.MessageID != "m-imported" || got.ThreadID != "t-imported" {
		t.Fatalf("unexpected output: err=%v value=%#v output=%s", err, got, out.String())
	}
}

func TestGmailImportReadOnlyBlocksBeforeAuth(t *testing.T) {
	ctx := googleapi.WithReadOnly(context.Background(), true)
	err := (&GmailImportCmd{File: testWriteImportEML(t), InternalDateSource: "dateHeader"}).Run(ctx, &RootFlags{Account: "test@example.com"})
	if !errors.Is(err, googleapi.ErrReadOnly) {
		t.Fatalf("error = %v, want ErrReadOnly", err)
	}
}

func TestGmailImportHonorsExactCommandAllowlist(t *testing.T) {
	path := testWriteImportEML(t)
	allowed := executeWithTestRuntime(t, []string{
		"--enable-commands-exact", "gmail.import", "--dry-run", "gmail", "import", path,
	}, &app.Runtime{})
	if allowed.err != nil {
		t.Fatalf("allowed import: %v", allowed.err)
	}
	blocked := executeWithTestRuntime(t, []string{
		"--enable-commands-exact", "gmail.search", "--dry-run", "gmail", "import", path,
	}, &app.Runtime{})
	if blocked.err == nil || !strings.Contains(blocked.err.Error(), "not enabled") {
		t.Fatalf("blocked import error = %v", blocked.err)
	}
}

func testWriteImportEML(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "message.eml")
	if err := os.WriteFile(path, []byte(testImportEML), 0o600); err != nil {
		t.Fatalf("write EML: %v", err)
	}
	return path
}
