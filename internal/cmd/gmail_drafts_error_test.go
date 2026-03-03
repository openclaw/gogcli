package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestGmailDraftsCreate_ValidationErrors(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}

	cmd := &GmailDraftsCreateCmd{}
	if err := runKong(t, cmd, []string{}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "required: --subject") {
		t.Fatalf("expected required subject error, got %v", err)
	}

	cmd = &GmailDraftsCreateCmd{}
	if err := runKong(t, cmd, []string{"--to", "b@b.com", "--subject", "Hi"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "required: --body") {
		t.Fatalf("expected body error, got %v", err)
	}

	cmd = &GmailDraftsCreateCmd{}
	if err := runKong(t, cmd, []string{"--to", "b@b.com", "--subject", "Hi", "--body", "Hello", "--quote"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "--quote requires --reply-to-message-id") {
		t.Fatalf("expected quote/reply validation error, got %v", err)
	}
}
