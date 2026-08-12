package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// assertGmailComposeFailsFast runs a gmail compose command with args against a
// counting mock server and asserts the command fails with an error mentioning
// flag (and wantErr, when non-empty) before any Gmail API request is made. The
// config home is isolated so a developer's real no-send config cannot shadow
// the expected validation error on send-side commands.
func assertGmailComposeFailsFast(t *testing.T, args []string, flag, wantErr string) {
	t.Helper()

	setTestConfigHome(t)
	requests := 0
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	})
	defer cleanup()

	result := executeWithGmailTestService(t, args, svc)
	if result.err == nil || !strings.Contains(result.err.Error(), flag) {
		t.Fatalf("expected %s validation error, got: %v", flag, result.err)
	}
	if wantErr != "" && !strings.Contains(result.err.Error(), wantErr) {
		t.Fatalf("expected error containing %q, got: %v", wantErr, result.err)
	}
	if requests != 0 {
		t.Fatalf("expected no Gmail API requests, got %d", requests)
	}
}

// TestExecute_GmailSend_CommaInDisplayName proves that a gmail send recipient
// whose display name contains a comma ("Smith, John" <john@example.com>) is
// parsed as a single recipient, not naively split on the comma — across --to,
// --cc, and --bcc.
func TestExecute_GmailSend_CommaInDisplayName(t *testing.T) {
	raw, _ := captureComposeRaw(t, []string{
		"--json", "--account", "me@example.com",
		"gmail", "send",
		"--to", `"Smith, John" <john@example.com>, other@example.com`,
		"--cc", `"Day, Ada" <ada@example.com>, c2@y.com`,
		"--bcc", `"Poe, Edgar" <edgar@example.com>, b2@y.com`,
		"--subject", "Hi",
		"--body", "Hello",
	}, "/gmail/v1/users/me/messages/send", mockReplySourceMessage)

	assertHeaderRecipients(t, raw, "To", []wantAddr{
		{name: "Smith, John", address: "john@example.com"},
		{address: "other@example.com"},
	})
	assertHeaderRecipients(t, raw, "Cc", []wantAddr{
		{name: "Day, Ada", address: "ada@example.com"},
		{address: "c2@y.com"},
	})
	assertHeaderRecipients(t, raw, "Bcc", []wantAddr{
		{name: "Poe, Edgar", address: "edgar@example.com"},
		{address: "b2@y.com"},
	})
}

// TestExecute_GmailSend_OrdinaryMultiRecipient guards the regression case: a
// plain comma-separated --to still yields the expected recipients.
func TestExecute_GmailSend_OrdinaryMultiRecipient(t *testing.T) {
	raw, _ := captureComposeRaw(t, []string{
		"--json", "--account", "me@example.com",
		"gmail", "send",
		"--to", "a@x.com, b@y.com",
		"--subject", "Hi",
		"--body", "Hello",
	}, "/gmail/v1/users/me/messages/send", mockReplySourceMessage)

	assertHeaderRecipients(t, raw, "To", []wantAddr{{address: "a@x.com"}, {address: "b@y.com"}})
}

// TestExecute_GmailSend_QuotedLocalPartRoundTrips proves an RFC-valid quoted
// local part ("john smith"@example.com) survives to the wire in re-parseable
// form: mail.ParseAddressList strips the quotes internally, and emitting the
// stripped form bare would produce an invalid To header.
func TestExecute_GmailSend_QuotedLocalPartRoundTrips(t *testing.T) {
	raw, _ := captureComposeRaw(t, []string{
		"--json", "--account", "me@example.com",
		"gmail", "send",
		"--to", `"john smith"@example.com`,
		"--subject", "Hi",
		"--body", "Hello",
	}, "/gmail/v1/users/me/messages/send", mockReplySourceMessage)

	// assertHeaderRecipients re-parses the built header, so it fails if the
	// quoting was lost.
	assertHeaderRecipients(t, raw, "To", []wantAddr{{address: "john smith@example.com"}})
}

// TestExecute_GmailCompose_GroupSyntaxRejectedEverywhere proves the zero-parse
// guard is shared by every recipient-flag consumer, not just gmail send: group
// syntax on forward (send and draft verbs), drafts create/update, and reply
// (--to and --remove) errors with the same flag-named message and makes no API
// call, instead of silently dropping the value.
func TestExecute_GmailCompose_GroupSyntaxRejectedEverywhere(t *testing.T) {
	const group = "undisclosed-recipients:;"
	cases := []struct {
		name string
		args []string
		flag string
	}{
		{
			name: "forward to",
			args: []string{"gmail", "forward", "orig-msg-1", "--to", group},
			flag: "--to",
		},
		{
			name: "forward cc",
			args: []string{"gmail", "forward", "orig-msg-1", "--to", "r@example.com", "--cc", group},
			flag: "--cc",
		},
		{
			name: "forward bcc",
			args: []string{"gmail", "forward", "orig-msg-1", "--to", "r@example.com", "--bcc", group},
			flag: "--bcc",
		},
		{
			name: "drafts forward to",
			args: []string{"gmail", "drafts", "forward", "orig-msg-1", "--to", group},
			flag: "--to",
		},
		{
			name: "drafts create to",
			args: []string{"gmail", "drafts", "create", "--subject", "S", "--body", "B", "--to", group},
			flag: "--to",
		},
		{
			name: "drafts create cc",
			args: []string{"gmail", "drafts", "create", "--subject", "S", "--body", "B", "--cc", group},
			flag: "--cc",
		},
		{
			name: "drafts update to",
			args: []string{"gmail", "drafts", "update", "d1", "--subject", "S", "--body", "B", "--to", group},
			flag: "--to",
		},
		{
			name: "drafts update cc",
			args: []string{"gmail", "drafts", "update", "d1", "--subject", "S", "--body", "B", "--cc", group},
			flag: "--cc",
		},
		{
			name: "reply to",
			args: []string{"gmail", "reply", "msg-1", "--body", "B", "--to", group},
			flag: "--to",
		},
		{
			name: "reply remove",
			args: []string{"gmail", "reply", "msg-1", "--body", "B", "--remove", group},
			flag: "--remove",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--account", "me@example.com"}, tc.args...)
			assertGmailComposeFailsFast(t, args, tc.flag, "contains no recipients")
		})
	}
}

// TestExecute_GmailSend_MalformedRecipientNoAPICall proves a malformed
// recipient on any of --to/--cc/--bcc surfaces a clear, flag-named error
// before any Gmail API request is made (no sender resolution, no send).
func TestExecute_GmailSend_MalformedRecipientNoAPICall(t *testing.T) {
	cases := []struct {
		name  string
		flag  string
		value string
	}{
		{name: "to", flag: "--to", value: "not an address <<>"},
		{name: "cc", flag: "--cc", value: "not an address <<>"},
		{name: "bcc", flag: "--bcc", value: "not an address <<>"},
		{name: "to commas only", flag: "--to", value: ", ,"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				"--account", "me@example.com",
				"gmail", "send",
				"--subject", "Hi",
				"--body", "Hello",
				tc.flag, tc.value,
			}
			if tc.flag != "--to" {
				// A valid --to keeps the failure attributable to the flag under
				// test (--to is required on the send path).
				args = append(args, "--to", "recipient@example.com")
			}
			assertGmailComposeFailsFast(t, args, tc.flag, "")
		})
	}
}

// TestExecute_GmailSend_GroupSyntaxRejectedBeforeDryRun proves RFC 5322 group
// syntax ("undisclosed-recipients:;"), which parses to zero addresses without
// a parse error, is rejected up front with a flag-named error in both plain
// and --reply-all modes. The --dry-run flag pins the ordering: without the
// check the dry-run would exit 0 reporting an empty list while the real send
// failed late, after service acquisition.
func TestExecute_GmailSend_GroupSyntaxRejectedBeforeDryRun(t *testing.T) {
	const group = "undisclosed-recipients:;"
	cases := []struct {
		name string
		args []string
		flag string
	}{
		{
			name: "plain to",
			args: []string{"--to", group},
			flag: "--to",
		},
		{
			name: "plain cc",
			args: []string{"--to", "recipient@example.com", "--cc", group},
			flag: "--cc",
		},
		{
			name: "reply-all to",
			args: []string{"--reply-all", "--reply-to-message-id", "msg-1", "--to", group},
			flag: "--to",
		},
		{
			name: "reply-all cc",
			args: []string{"--reply-all", "--reply-to-message-id", "msg-1", "--cc", group},
			flag: "--cc",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{
				"--dry-run",
				"--account", "me@example.com",
				"gmail", "send",
				"--subject", "Hi",
				"--body", "Hello",
			}, tc.args...)
			assertGmailComposeFailsFast(t, args, tc.flag, "contains no recipients")
		})
	}
}

// TestExecute_GmailSend_ReplyAllExplicitToOverrides proves an explicit --to
// still replaces (not merges with) the auto-populated reply-all To now that it
// is parsed address-aware, while the untouched Cc keeps the auto-populated Cc
// from the original message.
func TestExecute_GmailSend_ReplyAllExplicitToOverrides(t *testing.T) {
	raw, _ := captureComposeRaw(t, []string{
		"--json", "--account", "me@example.com",
		"gmail", "send",
		"--reply-all", "--reply-to-message-id", "msg-1",
		"--to", `"Smith, John" <john@example.com>`,
		"--body", "Hello",
	}, "/gmail/v1/users/me/messages/send", mockReplySourceMessage)

	assertHeaderRecipients(t, raw, "To", []wantAddr{{name: "Smith, John", address: "john@example.com"}})
	assertHeaderRecipients(t, raw, "Cc", []wantAddr{{name: "CC Person", address: "cc@example.com"}})
}

// TestExecute_GmailSend_ReplyAllExplicitCcOverrides proves an explicit --cc
// replaces (not merges with) the auto-populated reply-all Cc while the
// untouched To keeps the auto-populated reply-all recipients (original sender
// plus its To minus self).
func TestExecute_GmailSend_ReplyAllExplicitCcOverrides(t *testing.T) {
	raw, _ := captureComposeRaw(t, []string{
		"--json", "--account", "me@example.com",
		"gmail", "send",
		"--reply-all", "--reply-to-message-id", "msg-1",
		"--cc", "x@y.com",
		"--body", "Hello",
	}, "/gmail/v1/users/me/messages/send", mockReplySourceMessage)

	assertHeaderRecipients(t, raw, "To", []wantAddr{
		{name: "Alice Sender", address: "alice@example.com"},
		{name: "Other Person", address: "other@example.com"},
	})
	assertHeaderRecipients(t, raw, "Cc", []wantAddr{{address: "x@y.com"}})
}

// TestExecute_GmailSend_DryRunReportsParsedRecipients proves the send dry-run
// dict reports the recipient flags parsed address-aware (the display-name
// comma did not split into an extra element), that an omitted flag serializes
// as null (the pre-parse wire shape), and that the dry-run makes no API call —
// the reported lists are the same ones the built message would carry.
func TestExecute_GmailSend_DryRunReportsParsedRecipients(t *testing.T) {
	requests := 0
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--json", "--dry-run", "--account", "me@example.com",
		"gmail", "send",
		"--to", `"Smith, John" <john@example.com>, other@example.com`,
		"--bcc", `"Roe, Jane" <jane2@example.com>`,
		"--subject", "Hi",
		"--body", "Hello",
	}, svc)
	if code := ExitCode(result.err); code != 0 {
		t.Fatalf("expected clean dry-run exit (code 0), got code %d: %v", code, result.err)
	}

	assertDryRunRequestList(t, result.stdout, "to", []string{
		`"Smith, John" <john@example.com>`,
		"other@example.com",
	})
	assertDryRunRequestList(t, result.stdout, "bcc", []string{`"Roe, Jane" <jane2@example.com>`})

	// The omitted --cc must serialize as null (nil slice), not [] — the wire
	// shape the pre-parse splitCSV code produced.
	var payload struct {
		Request map[string]any `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, result.stdout)
	}
	if v, ok := payload.Request["cc"]; !ok || v != nil {
		t.Fatalf("expected omitted --cc to be null, got %#v", v)
	}

	if requests != 0 {
		t.Fatalf("expected no Gmail API requests, got %d", requests)
	}
}
