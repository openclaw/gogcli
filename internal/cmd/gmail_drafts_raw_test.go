package cmd

import (
	"bytes"
	"context"
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
)

// Both alternatives, a custom display name, folded reply headers, and a CID
// image must survive without MIME reconstruction or header normalization.
const testRawDraftEML = "From: Tom from InpaintKit <owner@example.com>\r\n" +
	"To: Recipient <recipient@example.com>\r\n" +
	"Subject: Review this image\r\n" +
	"Message-ID: <review@example.com>\r\n" +
	"In-Reply-To: <parent@example.com>\r\n" +
	"References: <first@example.com>\r\n\t<parent@example.com>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/related; boundary=related\r\n\r\n" +
	"--related\r\nContent-Type: multipart/alternative; boundary=alternative\r\n\r\n" +
	"--alternative\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nSee the image.\r\n" +
	"--alternative\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
	"<p>See the image.</p><img src=\"cid:demo@example.com\">\r\n" +
	"--alternative--\r\n" +
	"--related\r\nContent-Type: image/png\r\nContent-ID: <demo@example.com>\r\n" +
	"Content-Disposition: inline; filename=demo.png\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=\r\n" +
	"--related--\r\n\r\n"

func rawDraftArgs(operation, source string) []string {
	args := []string{"gmail", "drafts", operation}
	if operation == "update" {
		args = append(args, "draft-1")
	}
	return append(args, "--raw-file", source)
}

func writeRawDraftFixture(t *testing.T, input string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "message.eml")
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGmailDraftsRawPreservesMIMEAndSafety(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for _, stdin := range []bool{false, true} {
			for _, jsonMode := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stdin=%v/json=%v", operation, stdin, jsonMode), func(t *testing.T) {
					calls := 0
					svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
						calls++
						wantMethod, wantPath := http.MethodPost, "/gmail/v1/users/me/drafts"
						if operation == "update" {
							wantMethod, wantPath = http.MethodPut, wantPath+"/draft-1"
						}
						if r.Method != wantMethod || r.URL.Path != wantPath {
							t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
							http.Error(w, "unexpected request", http.StatusBadRequest)
							return
						}
						var posted gmail.Draft
						if err := json.NewDecoder(r.Body).Decode(&posted); err != nil || posted.Message == nil {
							t.Errorf("decode draft: %v", err)
							return
						}
						decoded, err := base64.RawURLEncoding.DecodeString(posted.Message.Raw)
						if err != nil || string(decoded) != testRawDraftEML {
							t.Errorf("MIME bytes changed: %v", err)
						}
						if posted.Message.ThreadId != "18abcdef123" {
							t.Errorf("thread = %q", posted.Message.ThreadId)
						}
						_ = json.NewEncoder(w).Encode(&gmail.Draft{Id: "draft-1", Message: &gmail.Message{
							Id: "replacement-message", ThreadId: "actual-thread",
						}})
					})
					defer cleanup()
					source := "-"
					if !stdin {
						source = writeRawDraftFixture(t, testRawDraftEML)
					}
					store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
					if err := store.Write(config.File{GmailNoSend: true, NoSendAccounts: map[string]bool{"owner@example.com": true}}); err != nil {
						t.Fatal(err)
					}
					args := append(rawDraftArgs(operation, source),
						"--account", "owner@example.com", "--no-input", "--gmail-no-send",
						"--enable-commands-exact=gmail.drafts."+operation,
						"--thread-id", "https://mail.google.com/mail/u/0/#inbox/18abcdef123")
					if jsonMode {
						args = append(args, "--json", "--results-only")
					}
					var out, diagnostics bytes.Buffer
					err := executeWithRuntime(args, &app.Runtime{
						Config: store, KeyringOptions: testKeyringOptions(),
						IO:       app.IO{In: strings.NewReader(testRawDraftEML), Out: &out, Err: &diagnostics},
						Services: app.Services{Gmail: func(context.Context, string) (*gmail.Service, error) { return svc, nil }},
					})
					if err != nil || calls != 1 {
						t.Fatalf("draft: err=%v calls=%d stderr=%s", err, calls, diagnostics.String())
					}
					if jsonMode {
						var got map[string]any
						if err := json.Unmarshal(out.Bytes(), &got); err != nil {
							t.Fatal(err)
						}
						message, ok := got["message"].(map[string]any)
						if !ok || message["id"] != "replacement-message" || got["draftId"] != "draft-1" ||
							got["threadId"] != "actual-thread" || got["inReplyTo"] != "<parent@example.com>" ||
							got["references"] != "<first@example.com> <parent@example.com>" || got["replyContextSource"] != "caller" || len(got) != 6 {
							t.Fatalf("wrong draft output contract: %s", out.String())
						}
					} else {
						for _, line := range []string{"draft_id\tdraft-1", "message_id\treplacement-message", "thread_id\tactual-thread"} {
							if !strings.Contains(out.String(), line) {
								t.Fatalf("missing %q in %s", line, out.String())
							}
						}
					}
				})
			}
		}
	}
}

func TestGmailDraftsRawDryRunIsOfflineAndContentSafe(t *testing.T) {
	path := writeRawDraftFixture(t, testRawDraftEML)
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			args := append(rawDraftArgs(operation, path), "--dry-run", "--json", "--readonly", "--gmail-no-send", "--thread-id", "thread-1")
			result := executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
				Gmail: func(context.Context, string) (*gmail.Service, error) {
					t.Fatal("dry-run must not authenticate")
					return nil, errors.New("unexpected authentication")
				},
			}})
			if result.err != nil {
				t.Fatal(result.err)
			}
			var got struct {
				Op      string         `json:"op"`
				Request map[string]any `json:"request"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
				t.Fatal(err)
			}
			if got.Op != "gmail.drafts."+operation || got.Request["source"] != path ||
				got.Request["sha256"] != fmt.Sprintf("%x", sha256.Sum256([]byte(testRawDraftEML))) ||
				got.Request["bytes"] != float64(len(testRawDraftEML)) || got.Request["thread_id"] != "thread-1" {
				t.Fatalf("unexpected plan: %s", result.stdout)
			}
			wantFields := 4
			if operation == "update" {
				wantFields++
				if got.Request["draft_id"] != "draft-1" {
					t.Fatalf("missing update target: %s", result.stdout)
				}
			}
			if len(got.Request) != wantFields {
				t.Fatalf("extra dry-run fields: %s", result.stdout)
			}
			for _, secret := range []string{"owner@example.com", "Review this image", "demo.png", "parent@example.com", "iVBOR"} {
				if strings.Contains(result.stdout+result.stderr, secret) {
					t.Fatalf("dry-run leaked %q", secret)
				}
			}
		})
	}
}

func TestGmailDraftsRawRejectsComposeFlags(t *testing.T) {
	common := [][]string{
		{"--to", "r@example.com"},
		{"--to="},
		{"--cc", "r@example.com"},
		{"--bcc", "r@example.com"},
		{"--subject", "s"},
		{"--subject="},
		{"--body", "b"},
		{"--body="},
		{"--body-file", "-"},
		{"--body-html", "<p>b</p>"},
		{"--body-html-file", "-"},
		{"--attach", "not-a-file"},
		{"--from", "a@example.com"},
		{"--reply-to", "a@example.com"},
		{"--reply-to-message-id", "m"},
		{"--reply-all"},
		{"--quote"},
		{"--auto-from-addressed-alias"},
	}
	for _, operation := range []string{"create", "update"} {
		cases := append([][]string(nil), common...)
		if operation == "update" {
			cases = append(cases, []string{"--clear-attachments"}, []string{"--clear-reply-context"})
		}
		for _, flags := range cases {
			t.Run(operation+"/"+strings.Join(flags, " "), func(t *testing.T) {
				// Neither the missing raw file nor compose stdin should be read.
				args := append(rawDraftArgs(operation, "does-not-exist.eml"), "--dry-run")
				result := executeWithTestRuntime(t, append(args, flags...), &app.Runtime{})
				if result.err == nil || !strings.Contains(result.err.Error(), "cannot be combined") {
					t.Fatalf("conflict: %v", result.err)
				}
			})
		}
	}
}

func TestGmailDraftsRawInvalidBeforeAuth(t *testing.T) {
	inputs := map[string]string{
		"empty": "", "malformed": "not a header\r\n\r\nbody", "no separator": "From: a@example.com\r\n",
		"missing from":   "To: a@example.com\r\n\r\nbody",
		"duplicate from": "From: a@example.com\r\nFrom: b@example.com\r\n\r\nbody",
		"invalid from":   "From: invalid\r\n\r\nbody", "invalid recipient": "From: a@example.com\r\nTo: invalid\r\n\r\nbody",
	}
	for _, operation := range []string{"create", "update"} {
		for name, input := range inputs {
			t.Run(operation+"/"+name, func(t *testing.T) {
				path := writeRawDraftFixture(t, input)
				result := executeWithTestRuntime(t, rawDraftArgs(operation, path), &app.Runtime{})
				if result.err == nil || !strings.Contains(result.err.Error(), "RFC822") {
					t.Fatalf("invalid input: %v", result.err)
				}
			})
		}
		t.Run(operation+"/empty flag", func(t *testing.T) {
			result := executeWithTestRuntime(t, rawDraftArgs(operation, ""), &app.Runtime{})
			if result.err == nil || !strings.Contains(result.err.Error(), "--raw-file requires") {
				t.Fatalf("empty flag: %v", result.err)
			}
		})
	}
}

func TestGmailDraftsRawReadOnlyBeforeAuth(t *testing.T) {
	path := writeRawDraftFixture(t, testRawDraftEML)
	for _, operation := range []string{"create", "update"} {
		args := append(rawDraftArgs(operation, path), "--readonly", "--account", "owner@example.com")
		result := executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
			Gmail: func(context.Context, string) (*gmail.Service, error) {
				t.Fatal("readonly must block before authentication")
				return nil, errors.New("unexpected authentication")
			},
		}})
		if !errors.Is(result.err, googleapi.ErrReadOnly) {
			t.Fatalf("readonly: %v", result.err)
		}
	}
}

func TestGmailDraftsRawAllowsMissingRecipientsAndDoesNotMerge(t *testing.T) {
	input := "From: Custom <owner@example.com>\r\n\r\nbody\r\n"
	path := writeRawDraftFixture(t, input)
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			calls := 0
			svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method == http.MethodGet {
					t.Error("must not fetch the existing draft or infer reply headers")
				}
				var posted gmail.Draft
				if err := json.NewDecoder(r.Body).Decode(&posted); err != nil || posted.Message == nil {
					t.Errorf("decode: %v", err)
					return
				}
				raw, err := base64.RawURLEncoding.DecodeString(posted.Message.Raw)
				if err != nil || string(raw) != input || posted.Message.ThreadId != "" {
					t.Errorf("message unexpectedly modified: %v, %s", err, raw)
				}
				_ = json.NewEncoder(w).Encode(&gmail.Draft{Id: "draft-1", Message: &gmail.Message{Id: "message-2", ThreadId: "new-thread"}})
			})
			defer cleanup()
			args := append(rawDraftArgs(operation, path), "--account", "owner@example.com", "--json")
			result := executeWithGmailTestService(t, args, svc)
			if result.err != nil || calls != 1 {
				t.Fatalf("draft: %v calls=%d", result.err, calls)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
				t.Fatal(err)
			}
			if got["inReplyTo"] != nil || got["references"] != nil || got["replyContextSource"] != nil {
				t.Fatalf("unexpected reply context: %s", result.stdout)
			}
		})
	}
}

func TestGmailDraftsRawSenderValidation(t *testing.T) {
	path := writeRawDraftFixture(t, testRawDraftEML)
	for _, operation := range []string{"create", "update"} {
		for _, status := range []string{"accepted", "pending", "missing"} {
			t.Run(operation+"/"+status, func(t *testing.T) {
				writes := 0
				svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/gmail/v1/users/me/settings/sendAs" {
						aliases := []*gmail.SendAs{}
						if status != "missing" {
							aliases = append(aliases, &gmail.SendAs{SendAsEmail: "owner@example.com", VerificationStatus: status, DisplayName: "Do not substitute"})
						}
						_ = json.NewEncoder(w).Encode(&gmail.ListSendAsResponse{SendAs: aliases})
						return
					}
					writes++
					var posted gmail.Draft
					_ = json.NewDecoder(r.Body).Decode(&posted)
					if posted.Message == nil || posted.Message.Raw != base64.RawURLEncoding.EncodeToString([]byte(testRawDraftEML)) {
						t.Error("alias lookup changed the MIME")
					}
					_ = json.NewEncoder(w).Encode(&gmail.Draft{Id: "draft-1", Message: &gmail.Message{Id: "m1"}})
				})
				defer cleanup()
				args := append(rawDraftArgs(operation, path), "--account", "another@example.com")
				result := executeWithGmailTestService(t, args, svc)
				if status == "accepted" {
					if result.err != nil || writes != 1 {
						t.Fatalf("verified alias: %v writes=%d", result.err, writes)
					}
				} else if result.err == nil || writes != 0 {
					t.Fatalf("invalid alias: %v writes=%d", result.err, writes)
				}
			})
		}
	}
}

func TestGmailDraftsRawDirectTokenRequiresAccount(t *testing.T) {
	path := writeRawDraftFixture(t, testRawDraftEML)
	svc, cleanup := newGmailServiceForTest(t, func(http.ResponseWriter, *http.Request) {
		t.Error("direct token without account must not call Gmail")
	})
	defer cleanup()
	for _, operation := range []string{"create", "update"} {
		args := append(rawDraftArgs(operation, path), "--access-token", "invalid-token")
		result := executeWithGmailTestService(t, args, svc)
		if result.err == nil || !strings.Contains(result.err.Error(), "explicit --account") {
			t.Fatalf("direct token: %v", result.err)
		}
	}
}

func TestGmailDraftsRawSchema(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		doc := schemaForCommand(t, "gmail drafts "+operation)
		flag := schemaFlagByName(t, doc.Command, "raw-file")
		if flag.Type != "string" || !strings.Contains(flag.Help, "exact RFC822") {
			t.Fatalf("raw-file schema: %#v", flag)
		}
	}
	doc := schemaForCommand(t, "gmail drafts get")
	flag := schemaFlagByName(t, doc.Command, "format")
	if !strings.Contains(flag.Help, "full|raw") {
		t.Fatalf("format schema: %#v", flag)
	}
}

func TestGmailDraftsGetRaw(t *testing.T) {
	for _, jsonMode := range []bool{false, true} {
		t.Run(fmt.Sprintf("json=%v", jsonMode), func(t *testing.T) {
			svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/gmail/v1/users/me/drafts/draft-1" || r.URL.Query().Get("format") != "raw" {
					t.Errorf("unexpected raw get: %s %s", r.Method, r.URL)
				}
				_ = json.NewEncoder(w).Encode(&gmail.Draft{Id: "draft-1", Message: &gmail.Message{
					Id: "m1", ThreadId: "thread-1", Raw: base64.URLEncoding.EncodeToString([]byte(testRawDraftEML)),
				}})
			})
			defer cleanup()
			args := []string{"--readonly", "--account", "owner@example.com", "gmail", "drafts", "get", "draft-1", "--format", "raw"}
			if jsonMode {
				args = append(args, "--json")
			}
			result := executeWithGmailTestService(t, args, svc)
			if result.err != nil {
				t.Fatal(result.err)
			}
			if jsonMode {
				var got struct {
					Draft gmail.Draft `json:"draft"`
				}
				if err := json.Unmarshal([]byte(result.stdout), &got); err != nil || got.Draft.Message == nil {
					t.Fatalf("raw JSON: %v", err)
				}
				raw, err := decodeGmailRaw(got.Draft.Message.Raw)
				if err != nil || string(raw) != testRawDraftEML || got.Draft.Id != "draft-1" {
					t.Fatalf("raw JSON changed message: %v", err)
				}
			} else if result.stdout != testRawDraftEML {
				t.Fatal("raw text output must contain only exact RFC822 bytes, including trailing newlines")
			}
		})
	}
}

func TestGmailDraftsGetRawRejectsAttachmentFlags(t *testing.T) {
	for _, flag := range []string{"--download", "--use-indexed-attachment-ids"} {
		result := executeWithTestRuntime(t, []string{"gmail", "drafts", "get", "draft-1", "--format", "raw", flag}, &app.Runtime{})
		if result.err == nil || !strings.Contains(result.err.Error(), "cannot be combined") {
			t.Fatalf("raw get conflict: %v", result.err)
		}
	}
}

func TestGmailRawInputSizeBoundary(t *testing.T) {
	header := "From: owner@example.com\r\nTo: recipient@example.com\r\n\r\n"
	input := header + strings.Repeat("x", int(maxGmailRawMessageBytes)-len(header))
	ctx := newCmdRuntimeIOContext(t, strings.NewReader(input), io.Discard, io.Discard)
	raw, plan, err := readRawGmailInput(ctx, "-", "", false)
	if err != nil || len(raw) != int(maxGmailRawMessageBytes) || plan.Bytes != len(raw) {
		t.Fatalf("exact limit rejected: %v bytes=%d", err, len(raw))
	}
	input += "x"
	path := writeRawDraftFixture(t, input)
	for _, source := range []string{path, "-"} {
		for _, operation := range []string{"create", "update", "send"} {
			t.Run(operation+"/"+filepath.Base(source), func(t *testing.T) {
				var out, diagnostics bytes.Buffer
				args := rawDraftArgs(operation, source)
				if operation == "send" {
					args = []string{"gmail", "send", "--raw-file", source}
				}
				err := executeWithRuntime(args, &app.Runtime{KeyringOptions: testKeyringOptions(), IO: app.IO{
					In: strings.NewReader(input), Out: &out, Err: &diagnostics,
				}})
				if err == nil || !strings.Contains(err.Error(), "exceeds maximum size of 36700160 bytes") {
					t.Fatalf("oversize: %v", err)
				}
			})
		}
	}
}

func TestGmailRawInvalidDryRunDoesNotLeakContent(t *testing.T) {
	secret := "PRIVATE_CONTENT_WITHOUT_COLON"
	for _, operation := range []string{"create", "update", "send"} {
		for _, input := range []string{
			secret + "\r\n\r\nbody",
			"From: " + secret + "\r\n\r\nbody",
			"From: owner@example.com\r\nTo: " + secret + "\r\n\r\nbody",
		} {
			path := writeRawDraftFixture(t, input)
			args := rawDraftArgs(operation, path)
			if operation == "send" {
				args = []string{"gmail", "send", "--raw-file", path}
			}
			result := executeWithTestRuntime(t, append(args, "--dry-run", "--json"), &app.Runtime{})
			if result.err == nil || !strings.Contains(result.err.Error(), "RFC822") {
				t.Fatalf("invalid preview: %v", result.err)
			}
			if strings.Contains(result.stdout+result.stderr+result.err.Error(), secret) {
				t.Fatalf("%s leaked invalid message content", operation)
			}
		}
	}
}

func TestGmailDraftsRawAutomaticAliasEnvironment(t *testing.T) {
	t.Setenv("GOG_GMAIL_AUTO_FROM_ADDRESSED_ALIAS", "true")
	path := writeRawDraftFixture(t, testRawDraftEML)
	for _, operation := range []string{"create", "update"} {
		args := append(rawDraftArgs(operation, path), "--dry-run", "--json")
		result := executeWithTestRuntime(t, args, &app.Runtime{})
		if result.err == nil || !strings.Contains(result.err.Error(), "--auto-from-addressed-alias") {
			t.Fatalf("alias env should conflict: %v", result.err)
		}
		result = executeWithTestRuntime(t, append(args, "--auto-from-addressed-alias=false"), &app.Runtime{})
		if result.err != nil {
			t.Fatalf("explicit false should disable alias env: %v", result.err)
		}
	}
}

func TestGmailDraftsRawAPIFailureDoesNotReportSuccess(t *testing.T) {
	path := writeRawDraftFixture(t, testRawDraftEML)
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "draft rejected", http.StatusBadRequest)
	})
	defer cleanup()
	for _, operation := range []string{"create", "update"} {
		args := append(rawDraftArgs(operation, path), "--account", "owner@example.com", "--json")
		result := executeWithGmailTestService(t, args, svc)
		if result.err == nil || result.stdout != "" {
			t.Fatalf("API failure: err=%v stdout=%s", result.err, result.stdout)
		}
	}
}
