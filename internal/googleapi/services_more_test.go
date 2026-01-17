package googleapi

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

func TestNewServicesWithStoredToken(t *testing.T) {
	origRead := readClientCredentials
	origOpen := openSecretsStore

	t.Cleanup(func() {
		readClientCredentials = origRead
		openSecretsStore = origOpen
	})

	readClientCredentials = func() (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

	store := &stubStore{tok: secrets.Token{RefreshToken: "rt"}}
	openSecretsStore = func() (secrets.Store, error) {
		return store, nil
	}

	ctx := context.Background()

	if _, err := NewGmail(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewGmail: %v", err)
	}

	if _, err := NewDrive(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewDrive: %v", err)
	}

	if _, err := NewDocs(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewDocs: %v", err)
	}

	if _, err := NewCalendar(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewCalendar: %v", err)
	}

	if _, err := NewChat(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewChat: %v", err)
	}

	if _, err := NewSheets(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewSheets: %v", err)
	}

	if _, err := NewTasks(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewTasks: %v", err)
	}

	if _, err := NewKeep(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewKeep: %v", err)
	}

	if _, err := NewCloudIdentityGroups(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewCloudIdentityGroups: %v", err)
	}

	if _, err := NewPeopleContacts(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewPeopleContacts: %v", err)
	}

	if _, err := NewPeopleOtherContacts(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewPeopleOtherContacts: %v", err)
	}

	if _, err := NewPeopleDirectory(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewPeopleDirectory: %v", err)
	}
}

func TestNewKeepWithServiceAccountErrors(t *testing.T) {
	_, err := NewKeepWithServiceAccount(context.Background(), filepath.Join(t.TempDir(), "missing.json"), "a@b.com")
	if err == nil {
		t.Fatalf("expected error")
	}

	if !strings.Contains(err.Error(), "read service account file") {
		t.Fatalf("unexpected error: %v", err)
	}
}
