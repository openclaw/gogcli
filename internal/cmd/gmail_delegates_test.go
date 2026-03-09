package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestDelegatesCommandsExist(t *testing.T) {
	// Unit tests for the actual API calls live in integration; here we just ensure
	// the commands exist and are properly structured. (Compile-time coverage.)
	_ = GmailDelegatesCmd{}
	_ = GmailDelegatesListCmd{}
	_ = GmailDelegatesGetCmd{}
	_ = GmailDelegatesAddCmd{}
	_ = GmailDelegatesRemoveCmd{}
}

func TestGmailDelegatesAdd_NoInputRequiresForce(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", NoInput: true}
	cmd := &GmailDelegatesAddCmd{}
	err := runKong(t, cmd, []string{"delegate@example.com"}, context.Background(), flags)
	if err == nil || !strings.Contains(err.Error(), "refusing to add gmail delegate") {
		t.Fatalf("expected refusing error, got %v", err)
	}
}
