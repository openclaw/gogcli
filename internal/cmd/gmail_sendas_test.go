package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/app"
)

func TestGmailSendAsListCmd_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/settings/sendAs") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAs": []map[string]any{
					{
						"sendAsEmail":        "primary@example.com",
						"displayName":        "Primary User",
						"isDefault":          true,
						"isPrimary":          true,
						"treatAsAlias":       false,
						"verificationStatus": "accepted",
					},
					{
						"sendAsEmail":        "work@company.com",
						"displayName":        "Work Alias",
						"isDefault":          false,
						"isPrimary":          false,
						"treatAsAlias":       true,
						"verificationStatus": "accepted",
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "sendas", "list"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		SendAs []struct {
			SendAsEmail        string `json:"sendAsEmail"`
			DisplayName        string `json:"displayName"`
			IsDefault          bool   `json:"isDefault"`
			VerificationStatus string `json:"verificationStatus"`
		} `json:"sendAs"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if len(parsed.SendAs) != 2 {
		t.Fatalf("unexpected sendAs count: %d", len(parsed.SendAs))
	}
	if parsed.SendAs[0].SendAsEmail != "primary@example.com" {
		t.Fatalf("unexpected first sendAs: %#v", parsed.SendAs[0])
	}
	if parsed.SendAs[1].SendAsEmail != "work@company.com" {
		t.Fatalf("unexpected second sendAs: %#v", parsed.SendAs[1])
	}
}

func TestGmailSendAsGetCmd_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/settings/sendAs/work@company.com") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "work@company.com",
				"displayName":        "Work Alias",
				"replyToAddress":     "replies@company.com",
				"signature":          "<b>Signature</b>",
				"isDefault":          false,
				"isPrimary":          false,
				"treatAsAlias":       true,
				"verificationStatus": "accepted",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "sendas", "get", "work@company.com"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		SendAs struct {
			SendAsEmail        string `json:"sendAsEmail"`
			DisplayName        string `json:"displayName"`
			ReplyToAddress     string `json:"replyToAddress"`
			VerificationStatus string `json:"verificationStatus"`
		} `json:"sendAs"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if parsed.SendAs.SendAsEmail != "work@company.com" {
		t.Fatalf("unexpected sendAs: %#v", parsed.SendAs)
	}
	if parsed.SendAs.DisplayName != "Work Alias" {
		t.Fatalf("unexpected displayName: %q", parsed.SendAs.DisplayName)
	}
}

func TestGmailBatchDeleteCmd_JSON(t *testing.T) {
	var receivedIDs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/messages/batchDelete") && r.Method == http.MethodPost {
			var body struct {
				IDs []string `json:"ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			receivedIDs = body.IDs
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--force", "--account", "a@b.com", "gmail", "batch", "delete", "msg1", "msg2", "msg3"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if len(receivedIDs) != 3 || receivedIDs[0] != "msg1" {
		t.Fatalf("unexpected IDs sent: %v", receivedIDs)
	}

	var parsed struct {
		Deleted []string `json:"deleted"`
		Count   int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if parsed.Count != 3 {
		t.Fatalf("unexpected count: %d", parsed.Count)
	}
}

func TestGmailBatchDeleteCmd_UsesDedicatedServiceFactory(t *testing.T) {
	var genericCalled bool
	var deleteCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/messages/batchDelete") && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc := newGmailServiceFromServer(t, srv)
	result := executeWithTestRuntime(
		t,
		[]string{"--json", "--force", "--account", "a@b.com", "gmail", "batch", "delete", "msg1"},
		&app.Runtime{Services: app.Services{
			Gmail: func(context.Context, string) (*gmail.Service, error) {
				genericCalled = true
				return nil, errors.New("generic Gmail factory called")
			},
			GmailDelete: func(context.Context, string) (*gmail.Service, error) {
				deleteCalled = true
				return svc, nil
			},
		}},
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if genericCalled {
		t.Fatal("generic Gmail factory was called")
	}
	if !deleteCalled {
		t.Fatal("dedicated Gmail delete factory was not called")
	}
}

func TestGmailBatchModifyCmd_JSON(t *testing.T) {
	var receivedRequest struct {
		IDs            []string `json:"ids"`
		AddLabelIds    []string `json:"addLabelIds"`
		RemoveLabelIds []string `json:"removeLabelIds"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
					{"id": "SPAM", "name": "SPAM", "type": "system"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/messages/batchModify") && r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&receivedRequest)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "batch", "modify", "msg1", "msg2", "--add", "INBOX", "--remove", "SPAM"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if len(receivedRequest.IDs) != 2 {
		t.Fatalf("unexpected IDs: %v", receivedRequest.IDs)
	}
	if len(receivedRequest.AddLabelIds) != 1 || receivedRequest.AddLabelIds[0] != "INBOX" {
		t.Fatalf("unexpected addLabelIds: %v", receivedRequest.AddLabelIds)
	}
	if len(receivedRequest.RemoveLabelIds) != 1 || receivedRequest.RemoveLabelIds[0] != "SPAM" {
		t.Fatalf("unexpected removeLabelIds: %v", receivedRequest.RemoveLabelIds)
	}

	var parsed struct {
		Modified []string `json:"modified"`
		Count    int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if parsed.Count != 2 {
		t.Fatalf("unexpected count: %d", parsed.Count)
	}
}

func TestGmailBatchModifyCmd_MissingLabelsIsUsageError(t *testing.T) {
	result := executeWithTestRuntime(t, []string{"--account", "a@b.com", "gmail", "batch", "modify", "msg1"}, nil)
	if result.err == nil {
		t.Fatal("expected error")
	}
	if got := ExitCode(result.err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, result.err)
	}
	if !strings.Contains(result.err.Error(), "must specify --add and/or --remove") {
		t.Fatalf("unexpected error: %v", result.err)
	}
}

func TestGmailSendAsCreateCmd_JSON(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/settings/sendAs") && r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "alias@example.com",
				"displayName":        "Test Alias",
				"verificationStatus": "pending",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "sendas", "create", "alias@example.com", "--display-name", "Test Alias"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		SendAs struct {
			SendAsEmail        string `json:"sendAsEmail"`
			VerificationStatus string `json:"verificationStatus"`
		} `json:"sendAs"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if parsed.SendAs.SendAsEmail != "alias@example.com" {
		t.Fatalf("unexpected sendAs: %#v", parsed.SendAs)
	}
	if parsed.SendAs.VerificationStatus != "pending" {
		t.Fatalf("unexpected status: %q", parsed.SendAs.VerificationStatus)
	}
}

func TestGmailSendAsDeleteCmd_JSON(t *testing.T) {
	var deletedEmail string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/settings/sendAs/") && r.Method == http.MethodDelete {
			parts := strings.Split(r.URL.Path, "/")
			deletedEmail = parts[len(parts)-1]
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--force", "--account", "a@b.com", "gmail", "sendas", "delete", "delete-me@example.com"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if deletedEmail != "delete-me@example.com" {
		t.Fatalf("unexpected deleted email: %q", deletedEmail)
	}

	var parsed struct {
		Email   string `json:"email"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if !parsed.Deleted {
		t.Fatalf("expected deleted=true")
	}
}

func TestGmailSendAsVerifyCmd_JSON(t *testing.T) {
	var verifiedEmail string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/settings/sendAs/") && strings.HasSuffix(r.URL.Path, "/verify") && r.Method == http.MethodPost {
			parts := strings.Split(r.URL.Path, "/")
			verifiedEmail = parts[len(parts)-2]
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "sendas", "verify", "verify-me@example.com"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if verifiedEmail != "verify-me@example.com" {
		t.Fatalf("unexpected verified email: %q", verifiedEmail)
	}

	var parsed struct {
		Email   string `json:"email"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if parsed.Email != "verify-me@example.com" {
		t.Fatalf("unexpected email: %q", parsed.Email)
	}
}

func TestGmailSendAsUpdateCmd_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/settings/sendAs/update@example.com") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "update@example.com",
				"displayName":        "Old Name",
				"verificationStatus": "accepted",
			})
			return
		case strings.Contains(r.URL.Path, "/settings/sendAs/update@example.com") && r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "update@example.com",
				"displayName":        "New Name",
				"verificationStatus": "accepted",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "sendas", "update", "update@example.com", "--display-name", "New Name"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		SendAs struct {
			SendAsEmail string `json:"sendAsEmail"`
			DisplayName string `json:"displayName"`
		} `json:"sendAs"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if parsed.SendAs.DisplayName != "New Name" {
		t.Fatalf("unexpected displayName: %q", parsed.SendAs.DisplayName)
	}
}
