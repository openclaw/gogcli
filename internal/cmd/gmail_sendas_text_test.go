package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
)

func TestGmailSendAsListCmd_Text(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/settings/sendAs") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAs": []map[string]any{
					{
						"sendAsEmail":        "primary@example.com",
						"displayName":        "Primary",
						"isDefault":          true,
						"treatAsAlias":       false,
						"verificationStatus": "accepted",
					},
					{
						"sendAsEmail":        "alias@example.com",
						"displayName":        "Alias",
						"isDefault":          false,
						"treatAsAlias":       true,
						"verificationStatus": "pending",
					},
				},
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

	flags := &RootFlags{Account: "a@b.com"}

	textOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: io.Discard, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})

		if err := runKong(t, &GmailSendAsListCmd{}, []string{}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(textOut, "EMAIL") || !strings.Contains(textOut, "primary@example.com") {
		t.Fatalf("unexpected output: %q", textOut)
	}
	if !strings.Contains(textOut, "alias@example.com") {
		t.Fatalf("missing alias line: %q", textOut)
	}
}

func TestGmailSendAsListCmd_TextEmpty(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/settings/sendAs") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"sendAs": []map[string]any{}})
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

	flags := &RootFlags{Account: "a@b.com"}

	errOut := captureStderr(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: os.Stderr, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})

		if err := runKong(t, &GmailSendAsListCmd{}, []string{}, ctx, flags); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(errOut, "No send-as aliases") {
		t.Fatalf("unexpected stderr: %q", errOut)
	}
}

func TestGmailSendAsGetCmd_Text(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/settings/sendAs/work@company.com") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "work@company.com",
				"displayName":        "Work Alias",
				"replyToAddress":     "replies@company.com",
				"signature":          "Signature",
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

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	flags := &RootFlags{Account: "a@b.com"}

	var outBuf bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &outBuf, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	if err := runKong(t, &GmailSendAsGetCmd{}, []string{"work@company.com"}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := outBuf.String()
	if !strings.Contains(out, "send_as_email\twork@company.com") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "verification_status\taccepted") {
		t.Fatalf("unexpected output: %q", out)
	}
}
