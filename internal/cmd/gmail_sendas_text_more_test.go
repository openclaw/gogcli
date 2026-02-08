package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestGmailSendAsCreateVerifyDeleteUpdate_Text(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/settings/sendAs") && !strings.HasSuffix(r.URL.Path, "/verify"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail":        "alias@example.com",
				"verificationStatus": "pending",
			})
			return
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/verify"):
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/settings/sendAs/alias@example.com"):
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/settings/sendAs/alias@example.com"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail": "alias@example.com",
				"displayName": "Old Name",
			})
			return
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/settings/sendAs/alias@example.com"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAsEmail": "alias@example.com",
				"displayName": "New Name",
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

	flags := &RootFlags{Account: "a@b.com", Force: true}

	createOut := captureStdout(t, func() {
		errOut := captureStderr(t, func() {
			u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
			if uiErr != nil {
				t.Fatalf("ui.New: %v", uiErr)
			}
			ctx := ui.WithUI(context.Background(), u)
			ctx = outfmt.WithMode(ctx, outfmt.Mode{})
			if err := runKong(t, &GmailSendAsCreateCmd{}, []string{"alias@example.com", "--display-name", "Alias"}, ctx, flags); err != nil {
				t.Fatalf("create: %v", err)
			}
		})
		if !strings.Contains(errOut, "Verification email sent") {
			t.Fatalf("unexpected stderr: %q", errOut)
		}
	})
	if !strings.Contains(createOut, "send_as_email\talias@example.com") || !strings.Contains(createOut, "verification_status\tpending") {
		t.Fatalf("unexpected create output: %q", createOut)
	}

	verifyOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})
		if err := runKong(t, &GmailSendAsVerifyCmd{}, []string{"alias@example.com"}, ctx, flags); err != nil {
			t.Fatalf("verify: %v", err)
		}
	})
	if !strings.Contains(verifyOut, "Verification email sent to alias@example.com") {
		t.Fatalf("unexpected verify output: %q", verifyOut)
	}

	updateOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})
		if err := runKong(t, &GmailSendAsUpdateCmd{}, []string{"alias@example.com", "--display-name", "New Name"}, ctx, flags); err != nil {
			t.Fatalf("update: %v", err)
		}
	})
	if !strings.Contains(updateOut, "Updated send-as alias: alias@example.com") {
		t.Fatalf("unexpected update output: %q", updateOut)
	}

	deleteOut := captureStdout(t, func() {
		u, uiErr := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
		if uiErr != nil {
			t.Fatalf("ui.New: %v", uiErr)
		}
		ctx := ui.WithUI(context.Background(), u)
		ctx = outfmt.WithMode(ctx, outfmt.Mode{})
		if err := runKong(t, &GmailSendAsDeleteCmd{}, []string{"alias@example.com"}, ctx, flags); err != nil {
			t.Fatalf("delete: %v", err)
		}
	})
	if !strings.Contains(deleteOut, "Deleted send-as alias: alias@example.com") {
		t.Fatalf("unexpected delete output: %q", deleteOut)
	}
}
