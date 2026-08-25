package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
)

const testRawSendEML = "From: ScoutWave <scoutwave@example.com>\r\n" +
	"To: Recipient <recipient@example.com>\r\n" +
	"Subject: Exact bytes\r\n" +
	"Message-ID: <stable@example.com>\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
	"body with trailing bytes\r\n\r\n"

func TestGmailSendRawDryRunIsOfflineAndSafe(t *testing.T) {
	path := writeRawSendEML(t, testRawSendEML)
	result := executeWithTestRuntime(t, []string{"--dry-run", "--json", "gmail", "send", "--raw-file", path, "--thread-id", "thread-1"}, &app.Runtime{})
	if result.err != nil {
		t.Fatalf("dry-run: %v\nstderr=%s", result.err, result.stderr)
	}
	var got struct {
		Op      string `json:"op"`
		Request struct {
			Source   string `json:"source"`
			Bytes    int    `json:"bytes"`
			SHA256   string `json:"sha256"`
			ThreadID string `json:"thread_id"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, result.stdout)
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte(testRawSendEML)))
	if got.Op != "gmail.send" || got.Request.Source != path || got.Request.Bytes != len(testRawSendEML) || got.Request.SHA256 != wantHash || got.Request.ThreadID != "thread-1" {
		t.Fatalf("unexpected plan: %#v", got)
	}
	for _, secret := range []string{"recipient@example.com", "Exact bytes", "body with trailing bytes", "stable@example.com"} {
		if strings.Contains(result.stdout, secret) {
			t.Fatalf("dry-run leaked message content %q: %s", secret, result.stdout)
		}
	}
}

func TestGmailSendRawReadsStdinAndPreservesBytes(t *testing.T) {
	var posted gmail.Message
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/gmail/v1/users/me/messages/send" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "msg-1", "threadId": "thread-1"})
	})
	defer cleanup()

	var out bytes.Buffer
	ctx := newCmdRuntimeIOContext(t, strings.NewReader(testRawSendEML), &out, io.Discard)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	ctx = withGmailTestService(ctx, svc)
	err := (&GmailSendCmd{RawFile: "-", ThreadID: "thread-1"}).Run(ctx, &RootFlags{Account: "scoutwave@example.com"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(posted.Raw)
	if err != nil || string(decoded) != testRawSendEML || posted.ThreadId != "thread-1" {
		t.Fatalf("raw bytes/thread changed: err=%v thread=%q raw=%q", err, posted.ThreadId, decoded)
	}
	if !strings.Contains(out.String(), `"messageId": "msg-1"`) || !strings.Contains(out.String(), `"threadId": "thread-1"`) {
		t.Fatalf("provider IDs missing: %s", out.String())
	}
}

func TestGmailSendRawRejectsInvalidBeforeAuth(t *testing.T) {
	for name, input := range map[string]string{
		"empty":             "",
		"no separator":      "From: a@example.com\r\nTo: b@example.com",
		"malformed header":  "not a header\r\n\r\nbody",
		"missing from":      "To: b@example.com\r\n\r\nbody",
		"missing recipient": "From: a@example.com\r\n\r\nbody",
		"invalid recipient": "From: a@example.com\r\nTo: not-an-address\r\n\r\nbody",
	} {
		t.Run(name, func(t *testing.T) {
			ctx := newCmdRuntimeIOContext(t, strings.NewReader(input), io.Discard, io.Discard)
			err := (&GmailSendCmd{RawFile: "-"}).Run(ctx, &RootFlags{})
			if err == nil || !strings.Contains(err.Error(), "RFC822") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestGmailSendRawAppearsInCommandSchema(t *testing.T) {
	doc := schemaForCommand(t, "gmail send")
	flag := schemaFlagByName(t, doc.Command, "raw-file")
	if flag.Type != "string" || !strings.Contains(flag.Help, "exact RFC822") {
		t.Fatalf("raw-file schema = %#v", flag)
	}
}

func TestGmailSendRawRejectsEveryComposeMode(t *testing.T) {
	cases := []GmailSendCmd{
		{To: "a@example.com"},
		{Cc: "a@example.com"},
		{Bcc: "a@example.com"},
		{Subject: "s"},
		{Body: "b"},
		{BodyFile: "body.txt"},
		{BodyHTML: "<b>x</b>"},
		{BodyHTMLFile: "body.html"},
		{ReplyToMessageID: "m"},
		{ReplyAll: true},
		{ReplyTo: "a@example.com"},
		{Attach: []string{"a"}},
		{From: "a@example.com"},
		{composeSignatureOptions: composeSignatureOptions{Signature: true}},
		{composeSignatureOptions: composeSignatureOptions{SignatureFrom: "a@example.com"}},
		{composeSignatureOptions: composeSignatureOptions{SignatureFile: "sig.txt"}},
		{Track: true},
		{TrackSplit: true},
		{Quote: true},
	}
	for i := range cases {
		cases[i].RawFile = "-"
		ctx := newCmdRuntimeIOContext(t, strings.NewReader(testRawSendEML), io.Discard, io.Discard)
		if err := cases[i].Run(ctx, &RootFlags{DryRun: true}); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
			t.Errorf("case %d error = %v", i, err)
		}
	}
}

func TestGmailSendRawSafetyAndAlias(t *testing.T) {
	path := writeRawSendEML(t, testRawSendEML)
	blocked := executeWithTestRuntime(t, []string{"--gmail-no-send", "--dry-run", "gmail", "send", "--raw-file", path}, &app.Runtime{})
	if blocked.err == nil || !strings.Contains(blocked.err.Error(), "no-send") {
		t.Fatalf("no-send error = %v", blocked.err)
	}
	alias := executeWithTestRuntime(t, []string{"--dry-run", "--json", "send", "--raw-file", path}, &app.Runtime{})
	if alias.err != nil || !strings.Contains(alias.stdout, `"op": "gmail.send"`) {
		t.Fatalf("alias: err=%v out=%s", alias.err, alias.stdout)
	}
	ctx := googleapi.WithReadOnly(newCmdRuntimeIOContext(t, strings.NewReader(testRawSendEML), io.Discard, io.Discard), true)
	err := (&GmailSendCmd{RawFile: "-"}).Run(ctx, &RootFlags{Account: "a@example.com"})
	if !errors.Is(err, googleapi.ErrReadOnly) {
		t.Fatalf("readonly error = %v", err)
	}
}

func writeRawSendEML(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "message.eml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
