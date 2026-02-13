package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/degree-analytics/ratatosk/internal/googleauth"
)

func TestAuthManageCmd_ServicesAndOptions(t *testing.T) {
	orig := startManageServer
	t.Cleanup(func() { startManageServer = orig })

	var got googleauth.ManageServerOptions
	startManageServer = func(ctx context.Context, opts googleauth.ManageServerOptions) error {
		got = opts
		return nil
	}

	if err := runKong(t, &AuthManageCmd{}, []string{"--services", "gmail,drive,gmail", "--force-consent", "--timeout", "2m"}, context.Background(), nil); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !got.ForceConsent {
		t.Fatalf("expected force-consent")
	}
	if got.Timeout != 2*time.Minute {
		t.Fatalf("unexpected timeout: %v", got.Timeout)
	}
	if len(got.Services) != 2 {
		t.Fatalf("expected de-duped services, got %#v", got.Services)
	}
}

func TestAuthManageCmd_InvalidService(t *testing.T) {
	orig := startManageServer
	t.Cleanup(func() { startManageServer = orig })
	startManageServer = func(context.Context, googleauth.ManageServerOptions) error { return nil }

	if err := runKong(t, &AuthManageCmd{}, []string{"--services", "nope"}, context.Background(), nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAuthManageCmd_DefaultServices_UserPreset(t *testing.T) {
	orig := startManageServer
	t.Cleanup(func() { startManageServer = orig })

	var got googleauth.ManageServerOptions
	startManageServer = func(ctx context.Context, opts googleauth.ManageServerOptions) error {
		got = opts
		return nil
	}

	if err := runKong(t, &AuthManageCmd{}, nil, context.Background(), nil); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if len(got.Services) != len(googleauth.UserServices()) {
		t.Fatalf("unexpected services: %v", got.Services)
	}
	for _, s := range got.Services {
		if s == googleauth.ServiceKeep {
			t.Fatalf("unexpected keep in services: %v", got.Services)
		}
	}
}

func TestAuthManageCmd_KeepRejected(t *testing.T) {
	orig := startManageServer
	t.Cleanup(func() { startManageServer = orig })
	startManageServer = func(context.Context, googleauth.ManageServerOptions) error { return nil }

	if err := runKong(t, &AuthManageCmd{}, []string{"--services", "keep"}, context.Background(), nil); err == nil {
		t.Fatalf("expected error")
	}
}
