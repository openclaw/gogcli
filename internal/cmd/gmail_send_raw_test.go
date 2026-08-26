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
	"github.com/openclaw/gogcli/internal/config"
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
	path := writeRawSendEML(t)
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
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode send result: %v", err)
	}
	if len(result) != 3 || result["messageId"] != "msg-1" || result["threadId"] != "thread-1" ||
		result["from"] != "ScoutWave <scoutwave@example.com>" {
		t.Fatalf("raw send broke the normal send output contract: %#v", result)
	}
}

func TestGmailSendRawRejectsInvalidBeforeAuth(t *testing.T) {
	for name, input := range map[string]string{
		"empty":             "",
		"no separator":      "From: a@example.com\r\nTo: b@example.com",
		"malformed header":  "not a header\r\n\r\nbody",
		"missing from":      "To: b@example.com\r\n\r\nbody",
		"duplicate from":    "From: a@example.com\r\nFrom: b@example.com\r\nTo: c@example.com\r\n\r\nbody",
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

func TestGmailSendRawNormalizesThreadURL(t *testing.T) {
	path := writeRawSendEML(t)
	result := executeWithTestRuntime(t, []string{
		"--dry-run", "--json", "gmail", "send", "--raw-file", path,
		"--thread-id", "https://mail.google.com/mail/u/0/#inbox/18abcdef123",
	}, &app.Runtime{})
	if result.err != nil {
		t.Fatalf("dry-run: %v", result.err)
	}
	var got struct {
		Request struct {
			ThreadID string `json:"thread_id"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil || got.Request.ThreadID != "18abcdef123" {
		t.Fatalf("normalized thread = %q, error = %v", got.Request.ThreadID, err)
	}
}

func TestGmailSendRawValidatesAuthenticatedSender(t *testing.T) {
	for _, tc := range []struct {
		name        string
		account     string
		aliasStatus string
		wantError   string
		wantLookups int
		wantSends   int
	}{
		{name: "primary avoids settings reads", account: "SCOUTWAVE@example.com", wantSends: 1},
		{name: "verified alias", account: "owner@example.com", aliasStatus: "accepted", wantLookups: 1, wantSends: 1},
		{name: "unverified alias", account: "owner@example.com", aliasStatus: "pending", wantError: "not verified", wantLookups: 1},
		{name: "unknown alias", account: "owner@example.com", wantError: "not found", wantLookups: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookups, sends := 0, 0
			svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/gmail/v1/users/me/settings/sendAs":
					lookups++
					aliases := []map[string]any{}
					if tc.aliasStatus != "" {
						aliases = append(aliases, map[string]any{
							"sendAsEmail": "scoutwave@example.com", "verificationStatus": tc.aliasStatus,
						})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"sendAs": aliases})
				case "/gmail/v1/users/me/messages/send":
					sends++
					_ = json.NewEncoder(w).Encode(map[string]string{"id": "msg-1", "threadId": "thread-1"})
				default:
					http.NotFound(w, r)
				}
			})
			defer cleanup()

			ctx := withGmailTestService(newCmdRuntimeIOContext(t, strings.NewReader(testRawSendEML), io.Discard, io.Discard), svc)
			err := (&GmailSendCmd{RawFile: "-"}).Run(ctx, &RootFlags{Account: tc.account})
			if tc.wantError == "" && err != nil || tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("send error = %v, want %q", err, tc.wantError)
			}
			if lookups != tc.wantLookups || sends != tc.wantSends {
				t.Fatalf("provider calls: lookups=%d sends=%d, want %d/%d", lookups, sends, tc.wantLookups, tc.wantSends)
			}
		})
	}
}

func TestGmailSendRawDirectTokenRequiresConcreteAccount(t *testing.T) {
	sends := 0
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		sends++
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	})
	defer cleanup()
	ctx := withGmailTestService(newCmdRuntimeIOContext(t, strings.NewReader(testRawSendEML), io.Discard, io.Discard), svc)
	err := (&GmailSendCmd{RawFile: "-"}).Run(ctx, &RootFlags{AccessToken: "invalid-token"})
	if err == nil || !strings.Contains(err.Error(), "explicit --account") || sends != 0 {
		t.Fatalf("direct-token send: err=%v provider calls=%d", err, sends)
	}
}

func TestGmailSendRawNoSendUsesMessageSenderBeforeDryRun(t *testing.T) {
	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := store.Write(config.File{NoSendAccounts: map[string]bool{"scoutwave@example.com": true}}); err != nil {
		t.Fatalf("write account guard: %v", err)
	}
	path := writeRawSendEML(t)
	result := executeWithTestRuntime(t, []string{
		"--access-token", "invalid-token", "--dry-run", "gmail", "send", "--raw-file", path,
	}, &app.Runtime{Config: store})
	if result.err == nil || !strings.Contains(result.err.Error(), "no-send") {
		t.Fatalf("blocked sender bypassed account guard: %v", result.err)
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
	path := writeRawSendEML(t)
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

func writeRawSendEML(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "message.eml")
	if err := os.WriteFile(path, []byte(testRawSendEML), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
