package cmd

import (
	"context"
	"testing"
)

func TestGmailWatchServeCmd_ValidationErrors(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}

	t.Run("path missing slash", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--path", "hook", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("port missing", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--port", "0"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("non-loopback without auth", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--bind", "0.0.0.0", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("oidc email requires verify", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--oidc-email", "svc@example.com", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("oidc audience requires verify", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--oidc-audience", "aud", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("fetch delay must be non-negative", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--fetch-delay", "-1", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("fetch delay must parse as duration", func(t *testing.T) {
		if err := runKong(t, &GmailWatchServeCmd{}, []string{"--fetch-delay", "not-a-duration", "--port", "9999"}, context.Background(), flags); err == nil {
			t.Fatalf("expected error")
		}
	})
}
