package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestExecute_GmailSettingsMoreCommands_JSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/gmail/v1/users/me/labels") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX"},
					{"id": "Label_1", "name": "Custom"},
					{"id": "SPAM", "name": "SPAM"},
					{"id": "IMPORTANT", "name": "IMPORTANT"},
				},
			})
			return

		// Delegates
		case strings.Contains(path, "/gmail/v1/users/me/settings/delegates") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(path, "/delegates/") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"delegateEmail":       "d@b.com",
					"verificationStatus":  "accepted",
					"delegationEnabled":   true,
					"verificationStatus2": "ignored",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"delegates": []map[string]any{
					{"delegateEmail": "d@b.com", "verificationStatus": "accepted"},
				},
			})
			return
		case strings.Contains(path, "/gmail/v1/users/me/settings/delegates") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"delegateEmail": "d@b.com", "verificationStatus": "pending"})
			return
		case strings.Contains(path, "/gmail/v1/users/me/settings/delegates/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return

		// Forwarding addresses
		case strings.Contains(path, "/gmail/v1/users/me/settings/forwardingAddresses") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(path, "/forwardingAddresses/") {
				_ = json.NewEncoder(w).Encode(map[string]any{"forwardingEmail": "f@b.com", "verificationStatus": "accepted"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"forwardingAddresses": []map[string]any{
					{"forwardingEmail": "f@b.com", "verificationStatus": "accepted"},
				},
			})
			return
		case strings.Contains(path, "/gmail/v1/users/me/settings/forwardingAddresses") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"forwardingEmail": "f@b.com", "verificationStatus": "pending"})
			return
		case strings.Contains(path, "/gmail/v1/users/me/settings/forwardingAddresses/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return

		// Auto-forwarding
		case strings.Contains(path, "/gmail/v1/users/me/settings/autoForwarding") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":      false,
				"emailAddress": "f@b.com",
				"disposition":  "leaveInInbox",
			})
			return
		case strings.Contains(path, "/gmail/v1/users/me/settings/autoForwarding") && r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":      true,
				"emailAddress": "f@b.com",
				"disposition":  "archive",
			})
			return

		// Vacation settings
		case strings.Contains(path, "/gmail/v1/users/me/settings/vacation") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enableAutoReply":       false,
				"responseSubject":       "S",
				"responseBodyHtml":      "<b>hi</b>",
				"responseBodyPlainText": "hi",
			})
			return
		case strings.Contains(path, "/gmail/v1/users/me/settings/vacation") && r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enableAutoReply": true,
				"responseSubject": "S2",
			})
			return

		// Filters
		case strings.Contains(path, "/gmail/v1/users/me/settings/filters") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(path, "/filters/") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "f1",
					"criteria": map[string]any{
						"from": "a@example.com",
					},
					"action": map[string]any{
						"addLabelIds": []string{"Label_1"},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"filter": []map[string]any{
					{"id": "f1", "criteria": map[string]any{"from": "a@example.com"}, "action": map[string]any{"addLabelIds": []string{"Label_1"}}},
				},
			})
			return
		case strings.Contains(path, "/gmail/v1/users/me/settings/filters") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "f2",
				"criteria": map[string]any{
					"from": "a@example.com",
				},
				"action": map[string]any{
					"addLabelIds": []string{"Label_1"},
				},
			})
			return
		case strings.Contains(path, "/gmail/v1/users/me/settings/filters/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return

		// Send-as
		case strings.Contains(path, "/gmail/v1/users/me/settings/sendAs") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(path, "/sendAs/") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"sendAsEmail":        "alias@b.com",
					"displayName":        "Alias",
					"replyToAddress":     "r@b.com",
					"signature":          "<b>sig</b>",
					"isPrimary":          false,
					"isDefault":          false,
					"treatAsAlias":       true,
					"verificationStatus": "accepted",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAs": []map[string]any{
					{"sendAsEmail": "alias@b.com", "displayName": "Alias", "verificationStatus": "accepted", "isDefault": true},
				},
			})
			return
		case strings.Contains(path, "/gmail/v1/users/me/settings/sendAs") && r.Method == http.MethodPost && !strings.Contains(path, "/verify"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"sendAsEmail": "alias@b.com", "verificationStatus": "pending"})
			return
		case strings.Contains(path, "/verify") && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
			return
		case strings.Contains(path, "/gmail/v1/users/me/settings/sendAs/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		case strings.Contains(path, "/gmail/v1/users/me/settings/sendAs/") && r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"sendAsEmail": "alias@b.com", "verificationStatus": "accepted"})
			return

		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "delegates", "list"}); err != nil {
				t.Fatalf("delegates list: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "delegates", "get", "d@b.com"}); err != nil {
				t.Fatalf("delegates get: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "delegates", "add", "d@b.com"}); err != nil {
				t.Fatalf("delegates add: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--force", "--account", "a@b.com", "gmail", "delegates", "remove", "d@b.com"}); err != nil {
				t.Fatalf("delegates remove: %v", err)
			}
		})

		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "forwarding", "list"}); err != nil {
				t.Fatalf("forwarding list: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "forwarding", "get", "f@b.com"}); err != nil {
				t.Fatalf("forwarding get: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "forwarding", "create", "f@b.com"}); err != nil {
				t.Fatalf("forwarding create: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--force", "--account", "a@b.com", "gmail", "forwarding", "delete", "f@b.com"}); err != nil {
				t.Fatalf("forwarding delete: %v", err)
			}
		})

		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "autoforward", "get"}); err != nil {
				t.Fatalf("autoforward get: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "autoforward", "update", "--enable", "--email", "f@b.com", "--disposition", "archive"}); err != nil {
				t.Fatalf("autoforward update: %v", err)
			}
		})

		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "vacation", "get"}); err != nil {
				t.Fatalf("vacation get: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "vacation", "update", "--enable", "--subject", "S2", "--body", "<b>hi</b>"}); err != nil {
				t.Fatalf("vacation update: %v", err)
			}
		})

		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "filters", "list"}); err != nil {
				t.Fatalf("filters list: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "filters", "get", "f1"}); err != nil {
				t.Fatalf("filters get: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "filters", "create", "--from", "a@example.com", "--add-label", "Custom"}); err != nil {
				t.Fatalf("filters create: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--force", "--account", "a@b.com", "gmail", "filters", "delete", "f1"}); err != nil {
				t.Fatalf("filters delete: %v", err)
			}
		})

		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "sendas", "list"}); err != nil {
				t.Fatalf("sendas list: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "sendas", "get", "alias@b.com"}); err != nil {
				t.Fatalf("sendas get: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "sendas", "create", "alias@b.com", "--display-name", "Alias"}); err != nil {
				t.Fatalf("sendas create: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "sendas", "verify", "alias@b.com"}); err != nil {
				t.Fatalf("sendas verify: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "sendas", "update", "alias@b.com", "--make-default"}); err != nil {
				t.Fatalf("sendas update: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--force", "--account", "a@b.com", "gmail", "sendas", "delete", "alias@b.com"}); err != nil {
				t.Fatalf("sendas delete: %v", err)
			}
		})
	})
}
