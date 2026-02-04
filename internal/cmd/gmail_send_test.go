package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestReplyHeaders(t *testing.T) {
	type hdr struct {
		Name  string
		Value string
	}
	type msg struct {
		ThreadID string
		Headers  []hdr
	}

	messages := map[string]msg{
		"m1": {ThreadID: "t1", Headers: []hdr{{Name: "Message-ID", Value: "<id1@example.com>"}}},
		"m2": {ThreadID: "t2", Headers: []hdr{
			{Name: "Message-Id", Value: "<id2@example.com>"},
			{Name: "References", Value: "<ref@example.com>"},
		}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/")
		m, ok := messages[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		hs := make([]map[string]any, 0, len(m.Headers))
		for _, h := range m.Headers {
			hs = append(hs, map[string]any{"name": h.Name, "value": h.Value})
		}
		resp := map[string]any{
			"id":       id,
			"threadId": m.ThreadID,
			"payload": map[string]any{
				"headers": hs,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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

	ctx := context.Background()

	inReplyTo, refs, threadID, err := replyHeaders(ctx, svc, "m1")
	if err != nil {
		t.Fatalf("replyHeaders: %v", err)
	}
	if inReplyTo != "<id1@example.com>" || refs != "<id1@example.com>" || threadID != "t1" {
		t.Fatalf("unexpected: inReplyTo=%q refs=%q thread=%q", inReplyTo, refs, threadID)
	}

	inReplyTo, refs, threadID, err = replyHeaders(ctx, svc, "m2")
	if err != nil {
		t.Fatalf("replyHeaders: %v", err)
	}
	if inReplyTo != "<id2@example.com>" {
		t.Fatalf("unexpected inReplyTo: %q", inReplyTo)
	}
	if !strings.Contains(refs, "<ref@example.com>") || !strings.Contains(refs, "<id2@example.com>") {
		t.Fatalf("unexpected refs: %q", refs)
	}
	if threadID != "t2" {
		t.Fatalf("unexpected thread: %q", threadID)
	}
}

func TestFetchReplyInfo_ThreadID(t *testing.T) {
	type hdr struct {
		Name  string
		Value string
	}
	type msg struct {
		ID           string
		ThreadID     string
		InternalDate string
		Headers      []hdr
	}

	thread := struct {
		ID       string
		Messages []msg
	}{
		ID: "t1",
		Messages: []msg{
			{
				ID:           "m1",
				ThreadID:     "t1",
				InternalDate: "1000",
				Headers: []hdr{
					{Name: "Message-ID", Value: "<id1@example.com>"},
					{Name: "From", Value: "sender@example.com"},
				},
			},
			{
				ID:           "m2",
				ThreadID:     "t1",
				InternalDate: "2000",
				Headers: []hdr{
					{Name: "Message-ID", Value: "<id2@example.com>"},
					{Name: "From", Value: "sender2@example.com"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/threads/") {
			http.NotFound(w, r)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/threads/")
		if id != thread.ID {
			http.NotFound(w, r)
			return
		}
		msgs := make([]map[string]any, 0, len(thread.Messages))
		for _, m := range thread.Messages {
			hs := make([]map[string]any, 0, len(m.Headers))
			for _, h := range m.Headers {
				hs = append(hs, map[string]any{"name": h.Name, "value": h.Value})
			}
			msgs = append(msgs, map[string]any{
				"id":           m.ID,
				"threadId":     m.ThreadID,
				"internalDate": m.InternalDate,
				"payload": map[string]any{
					"headers": hs,
				},
			})
		}
		resp := map[string]any{
			"id":       thread.ID,
			"messages": msgs,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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

	info, err := fetchReplyInfo(context.Background(), svc, "", "t1")
	if err != nil {
		t.Fatalf("fetchReplyInfo: %v", err)
	}
	if info.ThreadID != "t1" {
		t.Fatalf("unexpected thread: %q", info.ThreadID)
	}
	if info.InReplyTo != "<id2@example.com>" {
		t.Fatalf("unexpected inReplyTo: %q", info.InReplyTo)
	}
}

func TestSelectLatestThreadMessage(t *testing.T) {
	m1 := &gmail.Message{Id: "m1"}
	m2 := &gmail.Message{Id: "m2", InternalDate: 10}
	m3 := &gmail.Message{Id: "m3", InternalDate: 20}
	if got := selectLatestThreadMessage([]*gmail.Message{m1, m2, m3}); got == nil || got.Id != "m3" {
		t.Fatalf("expected m3, got %#v", got)
	}

	if got := selectLatestThreadMessage([]*gmail.Message{nil, m1}); got == nil || got.Id != "m1" {
		t.Fatalf("expected m1 fallback, got %#v", got)
	}
}

func TestGmailSendCmd_RunJSON(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		if r.Method == http.MethodPost && path == "/users/me/messages/send" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
			})
			return
		}
		http.NotFound(w, r)
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

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &GmailSendCmd{
		To:      "a@example.com",
		Subject: "Hello",
		Body:    "Body",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "\"messageId\"") || !strings.Contains(out, "\"threadId\"") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestGmailSendCmd_RunJSON_WithFrom(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs/alias@example.com":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "alias@example.com",
				"displayName":        "Alias",
				"verificationStatus": "accepted",
			})
			return
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m2",
				"threadId": "t2",
			})
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

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &GmailSendCmd{
		To:      "a@example.com",
		From:    "alias@example.com",
		Subject: "Hello",
		Body:    "Body",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "\"from\"") || !strings.Contains(out, "Alias <alias@example.com>") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestGmailSendCmd_RunJSON_WithFromDisplayNameFallbackToList(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs/alias@example.com":
			// Return send-as settings with empty display name but valid verification.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "alias@example.com",
				"displayName":        "",
				"verificationStatus": "accepted",
			})
			return
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs":
			// Fallback list endpoint returns the alias with a populated display name.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAs": []map[string]any{
					{
						"sendAsEmail":        "alias@example.com",
						"displayName":        "Alias From List",
						"verificationStatus": "accepted",
					},
				},
			})
			return
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m2b",
				"threadId": "t2b",
			})
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

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &GmailSendCmd{
		To:      "a@example.com",
		From:    "alias@example.com",
		Subject: "Hello",
		Body:    "Body",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "\"from\"") || !strings.Contains(out, "Alias From List <alias@example.com>") {
		t.Fatalf("expected from with display name from list fallback, got: %q", out)
	}
}

func TestGmailSendCmd_RunJSON_PrimaryAccountDisplayName(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs/a@b.com":
			// Return send-as settings with display name for primary account
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "a@b.com",
				"displayName":        "Primary User",
				"verificationStatus": "accepted",
			})
			return
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m3",
				"threadId": "t3",
			})
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

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &GmailSendCmd{
		To:      "recipient@example.com",
		Subject: "Hello",
		Body:    "Body",
		// Note: No From field set - testing primary account display name lookup
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	// Verify the From field in output includes display name
	if !strings.Contains(out, "\"from\"") || !strings.Contains(out, "Primary User <a@b.com>") {
		t.Fatalf("expected from with display name, got: %q", out)
	}
}

func TestGmailSendCmd_RunJSON_PrimaryAccountDisplayNameFallbackToList(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs/a@b.com":
			// Simulate missing display name in get response.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "a@b.com",
				"displayName":        "",
				"verificationStatus": "accepted",
			})
			return
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAs": []map[string]any{
					{
						"sendAsEmail":        "a@b.com",
						"displayName":        "Primary User",
						"verificationStatus": "accepted",
						"isPrimary":          true,
					},
				},
			})
			return
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m3b",
				"threadId": "t3b",
			})
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

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &GmailSendCmd{
		To:      "recipient@example.com",
		Subject: "Hello",
		Body:    "Body",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "\"from\"") || !strings.Contains(out, "Primary User <a@b.com>") {
		t.Fatalf("expected from with display name, got: %q", out)
	}
}

func TestGmailSendCmd_RunJSON_PrimaryAccountNoDisplayName(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs/a@b.com":
			// Return send-as settings WITHOUT display name
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "a@b.com",
				"displayName":        "", // No display name
				"verificationStatus": "accepted",
			})
			return
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m4",
				"threadId": "t4",
			})
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

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &GmailSendCmd{
		To:      "recipient@example.com",
		Subject: "Hello",
		Body:    "Body",
	}

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	// Verify the From field in output is just the email (no angle brackets)
	// JSON output has space after colon, e.g., "from": "a@b.com"
	if !strings.Contains(out, "\"from\": \"a@b.com\"") {
		t.Fatalf("expected from without display name, got: %q", out)
	}
}

func TestGmailSendCmd_RunJSON_PrimaryAccountLookupFails(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs/a@b.com":
			// Simulate send-as lookup failure (404)
			http.NotFound(w, r)
			return
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m5",
				"threadId": "t5",
			})
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

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &GmailSendCmd{
		To:      "recipient@example.com",
		Subject: "Hello",
		Body:    "Body",
	}

	// Should NOT fail even if send-as lookup fails - should gracefully fall back to plain email
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v (should not fail when send-as lookup fails for primary account)", err)
		}
	})
	// Verify the From field in output is just the email
	// JSON output has space after colon, e.g., "from": "a@b.com"
	if !strings.Contains(out, "\"from\": \"a@b.com\"") {
		t.Fatalf("expected from with plain email on lookup failure, got: %q", out)
	}
}

func TestGmailSendCmd_Run_FromUnverified(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		if r.Method == http.MethodGet && path == "/users/me/settings/sendAs/alias@example.com" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "alias@example.com",
				"verificationStatus": "pending",
			})
			return
		}
		http.NotFound(w, r)
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

	cmd := &GmailSendCmd{
		To:      "a@example.com",
		From:    "alias@example.com",
		Subject: "Hello",
		Body:    "Body",
	}

	if err := cmd.Run(context.Background(), &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected error for unverified send-as")
	}
}

func TestParseEmailAddresses(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{
			name:   "empty",
			input:  "",
			expect: nil,
		},
		{
			name:   "single plain email",
			input:  "alice@example.com",
			expect: []string{"alice@example.com"},
		},
		{
			name:   "single with display name",
			input:  "Alice Smith <alice@example.com>",
			expect: []string{"alice@example.com"},
		},
		{
			name:   "single with quoted display name",
			input:  `"Alice Smith" <alice@example.com>`,
			expect: []string{"alice@example.com"},
		},
		{
			name:   "multiple addresses",
			input:  "alice@example.com, bob@example.com",
			expect: []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:   "multiple with display names",
			input:  "Alice <alice@example.com>, Bob <bob@example.com>",
			expect: []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:   "mixed formats",
			input:  `"Alice Smith" <alice@example.com>, bob@example.com, Charlie <charlie@example.com>`,
			expect: []string{"alice@example.com", "bob@example.com", "charlie@example.com"},
		},
		{
			name:   "uppercase email",
			input:  "Alice@EXAMPLE.COM",
			expect: []string{"alice@example.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseEmailAddresses(tc.input)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("parseEmailAddresses(%q) = %v, want %v", tc.input, got, tc.expect)
			}
		})
	}
}

func TestFilterOutSelf(t *testing.T) {
	tests := []struct {
		name      string
		addresses []string
		selfEmail string
		expect    []string
	}{
		{
			name:      "empty list",
			addresses: nil,
			selfEmail: "me@example.com",
			expect:    []string{},
		},
		{
			name:      "no self present",
			addresses: []string{"alice@example.com", "bob@example.com"},
			selfEmail: "me@example.com",
			expect:    []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:      "self present exact case",
			addresses: []string{"alice@example.com", "me@example.com", "bob@example.com"},
			selfEmail: "me@example.com",
			expect:    []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:      "self present different case",
			addresses: []string{"alice@example.com", "ME@EXAMPLE.COM"},
			selfEmail: "me@example.com",
			expect:    []string{"alice@example.com"},
		},
		{
			name:      "only self",
			addresses: []string{"me@example.com"},
			selfEmail: "me@example.com",
			expect:    []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterOutSelf(tc.addresses, tc.selfEmail)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("filterOutSelf(%v, %q) = %v, want %v", tc.addresses, tc.selfEmail, got, tc.expect)
			}
		})
	}
}

func TestDeduplicateAddresses(t *testing.T) {
	tests := []struct {
		name      string
		addresses []string
		expect    []string
	}{
		{
			name:      "empty",
			addresses: nil,
			expect:    []string{},
		},
		{
			name:      "no duplicates",
			addresses: []string{"alice@example.com", "bob@example.com"},
			expect:    []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:      "exact duplicates",
			addresses: []string{"alice@example.com", "alice@example.com", "bob@example.com"},
			expect:    []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:      "case-insensitive duplicates",
			addresses: []string{"alice@example.com", "ALICE@EXAMPLE.COM", "bob@example.com"},
			expect:    []string{"alice@example.com", "bob@example.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deduplicateAddresses(tc.addresses)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("deduplicateAddresses(%v) = %v, want %v", tc.addresses, got, tc.expect)
			}
		})
	}
}

func TestBuildReplyAllRecipients(t *testing.T) {
	tests := []struct {
		name      string
		info      *replyInfo
		selfEmail string
		expectTo  []string
		expectCc  []string
	}{
		{
			name: "simple reply-all",
			info: &replyInfo{
				FromAddr: "sender@example.com",
				ToAddrs:  []string{"me@example.com", "alice@example.com"},
				CcAddrs:  []string{"bob@example.com"},
			},
			selfEmail: "me@example.com",
			expectTo:  []string{"sender@example.com", "alice@example.com"},
			expectCc:  []string{"bob@example.com"},
		},
		{
			name: "sender with display name",
			info: &replyInfo{
				FromAddr: "Sender Name <sender@example.com>",
				ToAddrs:  []string{"me@example.com"},
				CcAddrs:  nil,
			},
			selfEmail: "me@example.com",
			expectTo:  []string{"sender@example.com"},
			expectCc:  []string{},
		},
		{
			name: "deduplication across To",
			info: &replyInfo{
				FromAddr: "sender@example.com",
				ToAddrs:  []string{"sender@example.com", "alice@example.com"},
				CcAddrs:  nil,
			},
			selfEmail: "me@example.com",
			expectTo:  []string{"sender@example.com", "alice@example.com"},
			expectCc:  []string{},
		},
		{
			name: "Cc address already in To is excluded from Cc",
			info: &replyInfo{
				FromAddr: "sender@example.com",
				ToAddrs:  []string{"alice@example.com"},
				CcAddrs:  []string{"alice@example.com", "bob@example.com"},
			},
			selfEmail: "me@example.com",
			expectTo:  []string{"sender@example.com", "alice@example.com"},
			expectCc:  []string{"bob@example.com"},
		},
		{
			name: "self in Cc is filtered",
			info: &replyInfo{
				FromAddr: "sender@example.com",
				ToAddrs:  []string{"alice@example.com"},
				CcAddrs:  []string{"me@example.com", "bob@example.com"},
			},
			selfEmail: "me@example.com",
			expectTo:  []string{"sender@example.com", "alice@example.com"},
			expectCc:  []string{"bob@example.com"},
		},
		{
			name: "case insensitive self filtering",
			info: &replyInfo{
				FromAddr: "sender@example.com",
				ToAddrs:  []string{"ME@EXAMPLE.COM", "alice@example.com"},
				CcAddrs:  nil,
			},
			selfEmail: "me@example.com",
			expectTo:  []string{"sender@example.com", "alice@example.com"},
			expectCc:  []string{},
		},
		{
			name: "empty recipients",
			info: &replyInfo{
				FromAddr: "",
				ToAddrs:  nil,
				CcAddrs:  nil,
			},
			selfEmail: "me@example.com",
			expectTo:  []string{},
			expectCc:  []string{},
		},
		{
			name: "Reply-To header takes precedence over From (RFC 5322)",
			info: &replyInfo{
				FromAddr:    "original-sender@example.com",
				ReplyToAddr: "reply-here@example.com",
				ToAddrs:     []string{"me@example.com", "alice@example.com"},
				CcAddrs:     nil,
			},
			selfEmail: "me@example.com",
			expectTo:  []string{"reply-here@example.com", "alice@example.com"},
			expectCc:  []string{},
		},
		{
			name: "Reply-To with display name",
			info: &replyInfo{
				FromAddr:    "sender@example.com",
				ReplyToAddr: "Mailing List <list@example.com>",
				ToAddrs:     []string{"alice@example.com"},
				CcAddrs:     nil,
			},
			selfEmail: "me@example.com",
			expectTo:  []string{"list@example.com", "alice@example.com"},
			expectCc:  []string{},
		},
		{
			name: "Empty Reply-To falls back to From",
			info: &replyInfo{
				FromAddr:    "sender@example.com",
				ReplyToAddr: "",
				ToAddrs:     []string{"alice@example.com"},
				CcAddrs:     nil,
			},
			selfEmail: "me@example.com",
			expectTo:  []string{"sender@example.com", "alice@example.com"},
			expectCc:  []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotTo, gotCc := buildReplyAllRecipients(tc.info, tc.selfEmail)

			// Sort for comparison since order may vary
			sort.Strings(gotTo)
			sort.Strings(tc.expectTo)
			sort.Strings(gotCc)
			sort.Strings(tc.expectCc)

			if !reflect.DeepEqual(gotTo, tc.expectTo) {
				t.Errorf("To: got %v, want %v", gotTo, tc.expectTo)
			}
			if !reflect.DeepEqual(gotCc, tc.expectCc) {
				t.Errorf("Cc: got %v, want %v", gotCc, tc.expectCc)
			}
		})
	}
}

func TestFetchReplyInfo(t *testing.T) {
	type hdr struct {
		Name  string
		Value string
	}
	type msg struct {
		ThreadID string
		Headers  []hdr
	}

	messages := map[string]msg{
		"m1": {
			ThreadID: "t1",
			Headers: []hdr{
				{Name: "Message-ID", Value: "<id1@example.com>"},
				{Name: "From", Value: "sender@example.com"},
				{Name: "To", Value: "alice@example.com, bob@example.com"},
				{Name: "Cc", Value: "charlie@example.com"},
			},
		},
		"m2": {
			ThreadID: "t2",
			Headers: []hdr{
				{Name: "Message-ID", Value: "<id2@example.com>"},
				{Name: "From", Value: `"Sender Name" <sender@example.com>`},
				{Name: "To", Value: "recipient@example.com"},
			},
		},
		"m3": {
			ThreadID: "t3",
			Headers: []hdr{
				{Name: "Message-ID", Value: "<id3@example.com>"},
				{Name: "From", Value: "original-sender@example.com"},
				{Name: "Reply-To", Value: "Mailing List <list@example.com>"},
				{Name: "To", Value: "recipient@example.com"},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/messages/")
		m, ok := messages[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		hs := make([]map[string]any, 0, len(m.Headers))
		for _, h := range m.Headers {
			hs = append(hs, map[string]any{"name": h.Name, "value": h.Value})
		}
		resp := map[string]any{
			"id":       id,
			"threadId": m.ThreadID,
			"payload": map[string]any{
				"headers": hs,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
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

	ctx := context.Background()

	// Test m1: multiple recipients
	info, err := fetchReplyInfo(ctx, svc, "m1", "")
	if err != nil {
		t.Fatalf("fetchReplyInfo(m1): %v", err)
	}
	if info.ThreadID != "t1" {
		t.Errorf("ThreadID = %q, want %q", info.ThreadID, "t1")
	}
	if info.FromAddr != "sender@example.com" {
		t.Errorf("FromAddr = %q, want %q", info.FromAddr, "sender@example.com")
	}
	expectedTo := []string{"alice@example.com", "bob@example.com"}
	if !reflect.DeepEqual(info.ToAddrs, expectedTo) {
		t.Errorf("ToAddrs = %v, want %v", info.ToAddrs, expectedTo)
	}
	expectedCc := []string{"charlie@example.com"}
	if !reflect.DeepEqual(info.CcAddrs, expectedCc) {
		t.Errorf("CcAddrs = %v, want %v", info.CcAddrs, expectedCc)
	}

	// Test m2: sender with display name
	info, err = fetchReplyInfo(ctx, svc, "m2", "")
	if err != nil {
		t.Fatalf("fetchReplyInfo(m2): %v", err)
	}
	if info.FromAddr != `"Sender Name" <sender@example.com>` {
		t.Errorf("FromAddr = %q, want %q", info.FromAddr, `"Sender Name" <sender@example.com>`)
	}

	// Test empty message ID
	info, err = fetchReplyInfo(ctx, svc, "", "")
	if err != nil {
		t.Fatalf("fetchReplyInfo(''): %v", err)
	}
	if info.ThreadID != "" || info.FromAddr != "" {
		t.Errorf("Expected empty replyInfo for empty message ID")
	}

	// Test m3: message with Reply-To header
	info, err = fetchReplyInfo(ctx, svc, "m3", "")
	if err != nil {
		t.Fatalf("fetchReplyInfo(m3): %v", err)
	}
	if info.FromAddr != "original-sender@example.com" {
		t.Errorf("FromAddr = %q, want %q", info.FromAddr, "original-sender@example.com")
	}
	if info.ReplyToAddr != "Mailing List <list@example.com>" {
		t.Errorf("ReplyToAddr = %q, want %q", info.ReplyToAddr, "Mailing List <list@example.com>")
	}
}

func TestReplyAllValidation(t *testing.T) {
	// Test that --reply-all requires --reply-to-message-id
	cmd := &GmailSendCmd{
		ReplyAll: true,
	}

	// This would normally go through Run(), but we can test the validation logic
	if cmd.ReplyAll && strings.TrimSpace(cmd.ReplyToMessageID) == "" && strings.TrimSpace(cmd.ThreadID) == "" {
		// Expected: should require --reply-to-message-id
	} else {
		t.Error("Expected validation to require --reply-to-message-id when --reply-all is set")
	}

	// Test with --reply-to-message-id set
	cmd.ReplyToMessageID = "msg123"
	if cmd.ReplyAll && strings.TrimSpace(cmd.ReplyToMessageID) == "" {
		t.Error("Should not require --reply-to-message-id when it's already set")
	}

	cmd.ReplyToMessageID = ""
	cmd.ThreadID = "thread123"
	if cmd.ReplyAll && strings.TrimSpace(cmd.ThreadID) == "" {
		t.Error("Should not require --reply-to-message-id when --thread-id is set")
	}

	// Test --to is optional when --reply-all is used
	if strings.TrimSpace(cmd.To) == "" && !cmd.ReplyAll {
		t.Error("--to should be optional when --reply-all is used")
	}
}
