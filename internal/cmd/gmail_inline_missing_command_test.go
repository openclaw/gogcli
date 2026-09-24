package cmd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/gmail/v1"
)

type inlineDraftFixture struct {
	source           map[string]any
	raw              string
	rawStatus        int
	attachmentStatus int
}

type inlineDraftCalls struct{ raw, attachments, posts int32 }

func newInlineDraftFixture(body string) inlineDraftFixture {
	source := mockReplySourceMessageWithInlineImage()
	parts := source["payload"].(map[string]any)["parts"].([]map[string]any)
	alternatives := parts[0]["parts"].([]map[string]any)
	alternatives[1]["body"] = map[string]any{"data": base64.RawURLEncoding.EncodeToString([]byte(body)), "size": len(body)}
	parts[1]["body"] = map[string]any{"attachmentId": "image-bytes", "size": 8}
	return inlineDraftFixture{source: source, raw: inlineTestMIME(body)}
}

func executeInlineDraftFixture(t *testing.T, fixture inlineDraftFixture, args ...string) (executeTestResult, *gmail.Message, inlineDraftCalls) {
	t.Helper()
	setTestConfigHome(t)
	var rawCalls, attachmentCalls, posts atomic.Int32
	var posted atomic.Pointer[gmail.Message]
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			if r.URL.Query().Get("format") == gmailFormatRaw {
				rawCalls.Add(1)
				if fixture.rawStatus != 0 {
					w.WriteHeader(fixture.rawStatus)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg-1", "raw": base64.RawURLEncoding.EncodeToString([]byte(fixture.raw))})
			} else {
				_ = json.NewEncoder(w).Encode(fixture.source)
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/attachments/image-bytes"):
			attachmentCalls.Add(1)
			if fixture.attachmentStatus != 0 {
				w.WriteHeader(fixture.attachmentStatus)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": base64.RawURLEncoding.EncodeToString([]byte("png-data"))})
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/drafts":
			posts.Add(1)
			var draft gmail.Draft
			if err := json.NewDecoder(r.Body).Decode(&draft); err != nil || draft.Message == nil {
				t.Errorf("invalid draft payload: %v", err)
				http.Error(w, "invalid payload", http.StatusBadRequest)
				return
			}
			posted.Store(draft.Message)
			writeDraftCreatedResponse(w)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	})
	t.Cleanup(cleanup)
	command := append([]string{"--json", "--gmail-no-send", "--account", "me@example.com"}, args...)
	result := executeWithGmailTestService(t, command, svc)
	return result, posted.Load(), inlineDraftCalls{rawCalls.Load(), attachmentCalls.Load(), posts.Load()}
}

func TestMissingInlineDraftReportsDegradationAndPreservesMessage(t *testing.T) {
	for _, action := range []string{"reply", "reply-all"} {
		for _, resultsOnly := range []bool{false, true} {
			t.Run(action+"/"+map[bool]string{false: "json", true: "results-only"}[resultsOnly], func(t *testing.T) {
				fixture := newInlineDraftFixture(missingInlineTestHTML)
				args := []string{"gmail", "drafts", action, "msg-1", "--body", "Thanks", "--missing-inline-images", "placeholder"}
				if resultsOnly {
					args = append(args, "--results-only")
				}
				result, draft, calls := executeInlineDraftFixture(t, fixture, args...)
				if result.err != nil || draft == nil || calls.raw != 1 || calls.attachments != 1 || calls.posts != 1 {
					t.Fatalf("error=%v, calls=%+v, stderr=%q", result.err, calls, result.stderr)
				}
				var output struct {
					Degraded bool                 `json:"degraded"`
					Warnings []inlineImageWarning `json:"warnings"`
				}
				if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
					t.Fatal(err)
				}
				if !output.Degraded || len(output.Warnings) != 1 || output.Warnings[0].Code != "missing_inline_image" || output.Warnings[0].SourceMessageID != "msg-1" || output.Warnings[0].ContentID != "missing@example.com" || output.Warnings[0].Occurrences != 1 || output.Warnings[0].Replacement != "placeholder" {
					t.Fatalf("missing degradation report: %s", result.stdout)
				}
				if !strings.Contains(result.stderr, "review before sending") {
					t.Fatalf("missing stderr warning: %q", result.stderr)
				}
				raw, err := base64.RawURLEncoding.DecodeString(draft.Raw)
				if err != nil {
					t.Fatal(err)
				}
				message, err := parseBackupEmail(raw)
				if err != nil {
					t.Fatal(err)
				}
				if draft.ThreadId != "thread-1" || message.Subject != "Re: Project update" || rawMessageHeader(t, string(raw), "In-Reply-To") != "<original@example.com>" || !strings.Contains(rawMessageHeader(t, string(raw), "References"), "<root@example.com>") {
					t.Fatalf("reply threading or subject changed: %#v", draft)
				}
				if !strings.Contains(rawMessageHeader(t, string(raw), "To"), "alice@example.com") {
					t.Fatal("reply recipient changed")
				}
				if !strings.Contains(message.HTMLBody, `<p>Original HTML</p><img src="cid:image-1@example.com">`) || strings.Contains(message.HTMLBody, "cid:missing@example.com") || !strings.Contains(message.HTMLBody, "[Inline image unavailable:") || !strings.Contains(message.TextBody, "Original plain") || !strings.Contains(message.TextBody, "[Inline image unavailable:") {
					t.Fatalf("quote or markers missing: plain=%q, HTML=%q", message.TextBody, message.HTMLBody)
				}
				if len(message.Attachments) != 1 || string(message.Attachments[0].Data) != "png-data" || !strings.Contains(string(raw), "Content-ID: <image-1@example.com>") {
					t.Fatalf("valid inline resource changed: %#v", message.Attachments)
				}
			})
		}
	}
}

func TestMissingInlineDraftDefaultsStayStrict(t *testing.T) {
	result, draft, calls := executeInlineDraftFixture(t, newInlineDraftFixture(missingInlineTestHTML), "gmail", "drafts", "reply", "msg-1", "--body", "Thanks")
	if result.err == nil || !strings.Contains(result.stderr, "no matching MIME part") || draft != nil || calls.posts != 0 || calls.raw != 0 {
		t.Fatalf("error=%v, calls=%+v, stderr=%q", result.err, calls, result.stderr)
	}
}

func TestMissingInlineDraftNoDegradationSkipsRawRead(t *testing.T) {
	fixture := newInlineDraftFixture(`<p>Original HTML</p><img src="cid:image-1@example.com">`)
	result, draft, calls := executeInlineDraftFixture(t, fixture, "gmail", "drafts", "reply", "msg-1", "--body", "Thanks", "--missing-inline-images", "placeholder")
	if result.err != nil || draft == nil || calls.posts != 1 || calls.raw != 0 || strings.Contains(result.stdout, `"degraded"`) || strings.Contains(result.stdout, `"warnings"`) {
		t.Fatalf("error=%v, calls=%+v, output=%q, stderr=%q", result.err, calls, result.stdout, result.stderr)
	}
}

func TestMissingInlineDraftHTMLOnlyGetsPlainPlaceholder(t *testing.T) {
	fixture := newInlineDraftFixture(missingInlineTestHTML)
	parts := fixture.source["payload"].(map[string]any)["parts"].([]map[string]any)
	alternatives := parts[0]["parts"].([]map[string]any)
	parts[0]["parts"] = alternatives[1:]
	fixture.raw = strings.Replace(fixture.raw, "--alt\r\nContent-Type: text/plain\r\n\r\nOriginal plain\r\n", "", 1)
	result, draft, _ := executeInlineDraftFixture(t, fixture, "gmail", "drafts", "reply", "msg-1", "--body", "Thanks", "--missing-inline-images=placeholder")
	if result.err != nil || draft == nil {
		t.Fatalf("error=%v, stderr=%q", result.err, result.stderr)
	}
	raw, err := base64.RawURLEncoding.DecodeString(draft.Raw)
	if err != nil {
		t.Fatal(err)
	}
	message, err := parseBackupEmail(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message.TextBody, "Original HTML") || !strings.Contains(message.TextBody, "[Inline image unavailable:") {
		t.Fatalf("plain alternative missing derived text or placeholder: %q", message.TextBody)
	}
}

func TestMissingInlineDraftRejectsOtherFailures(t *testing.T) {
	for _, failure := range []string{"raw request", "raw absent", "malformed raw", "attachment request", "duplicate IDs", "missing body", "bad base64", "CSS"} {
		t.Run(failure, func(t *testing.T) {
			fixture := newInlineDraftFixture(missingInlineTestHTML)
			payload := fixture.source["payload"].(map[string]any)
			parts := payload["parts"].([]map[string]any)
			switch failure {
			case "raw request":
				fixture.rawStatus = http.StatusForbidden
			case "raw absent":
				fixture.raw = ""
			case "malformed raw":
				fixture.raw = strings.TrimSuffix(fixture.raw, "--outer--\r\n")
			case "attachment request":
				fixture.attachmentStatus = http.StatusNotFound
			case "duplicate IDs":
				payload["parts"] = append(parts, parts[1])
			case "missing body":
				parts[1]["body"] = nil
			case "bad base64":
				parts[0]["parts"].([]map[string]any)[1]["body"] = map[string]any{"data": "not base64!", "size": 10}
			case "CSS":
				fixture = newInlineDraftFixture(`<style>p { background:url(cid:missing@example.com); }</style>`)
			}
			result, draft, calls := executeInlineDraftFixture(t, fixture, "gmail", "drafts", "reply", "msg-1", "--body", "Thanks", "--missing-inline-images", "placeholder")
			if result.err == nil || draft != nil || calls.posts != 0 {
				t.Fatalf("failure was degraded: error=%v, calls=%+v, output=%q", result.err, calls, result.stdout)
			}
		})
	}
}

func TestMissingInlineDraftPolicyIsOfflineAndDraftOnly(t *testing.T) {
	for _, command := range [][]string{{"gmail", "reply"}, {"gmail", "reply-all"}, {"gmail", "drafts", "forward"}} {
		args := append([]string{"--dry-run"}, command...)
		args = append(args, "msg-1", "--missing-inline-images=placeholder")
		result := executeWithTestRuntime(t, args, nil)
		if ExitCode(result.err) != 2 || !strings.Contains(result.stderr, "unknown flag --missing-inline-images") {
			t.Fatalf("non-draft policy accepted: %v; stderr=%q", result.err, result.stderr)
		}
	}
	for _, action := range []string{"reply", "reply-all"} {
		args := make([]string, 0, 10)
		args = append(args, "--json", "--dry-run", "gmail", "drafts", action, "msg-1", "--body", "Thanks", "--missing-inline-images=placeholder")
		result := executeWithTestRuntime(t, args, nil)
		if result.err != nil || !strings.Contains(result.stdout, `"missing_inline_images": "placeholder"`) {
			t.Fatalf("dry-run failed: %v; stdout=%q; stderr=%q", result.err, result.stdout, result.stderr)
		}
		result = executeWithTestRuntime(t, append(args, "--no-quote"), nil)
		if ExitCode(result.err) != 2 || !strings.Contains(result.stderr, "cannot be combined with --no-quote") {
			t.Fatalf("no-quote policy accepted: %v; stderr=%q", result.err, result.stderr)
		}
	}
}
