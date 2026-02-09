package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestDriveCommand_ValidationErrors(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}

	moveCmd := &DriveMoveCmd{}
	if err := runKong(t, moveCmd, []string{"file1"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "missing --parent") {
		t.Fatalf("expected parent error, got %v", err)
	}

	shareCmd := &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "must specify --to") {
		t.Fatalf("expected share target error, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--to", "domain", "--domain", "example.com", "--role", "owner"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "invalid --role") {
		t.Fatalf("expected role error for domain share, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--to", "anyone", "--role", "owner"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "invalid --role") {
		t.Fatalf("expected role error, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--to", "user"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "missing --email") {
		t.Fatalf("expected missing email error, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--to", "domain"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "missing --domain") {
		t.Fatalf("expected missing domain error, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--to", "user", "--email", "a@b.com", "--discoverable"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "discoverable") {
		t.Fatalf("expected discoverable error, got %v", err)
	}

	shareCmd = &DriveShareCmd{}
	if err := runKong(t, shareCmd, []string{"file1", "--email", "a@b.com", "--domain", "example.com"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous target error, got %v", err)
	}
}

func TestDriveDeleteUnshare_NoInput(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com", NoInput: true}

	deleteCmd := &DriveDeleteCmd{}
	if err := runKong(t, deleteCmd, []string{"file1"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("expected refusing error, got %v", err)
	}

	unshareCmd := &DriveUnshareCmd{}
	if err := runKong(t, unshareCmd, []string{"file1", "perm1"}, context.Background(), flags); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("expected refusing error, got %v", err)
	}
}
