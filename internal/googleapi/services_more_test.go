package googleapi

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

func TestNewServicesWithStoredToken(t *testing.T) {
	origRead := readClientCredentials
	origOpen := openSecretsStore

	t.Cleanup(func() {
		readClientCredentials = origRead
		openSecretsStore = origOpen
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

	store := &stubStore{tok: secrets.Token{RefreshToken: "rt"}}
	openSecretsStore = func() (secrets.Store, error) {
		return store, nil
	}

	ctx := testClientResolverContext()

	if _, err := NewGmail(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewGmail: %v", err)
	}

	if _, err := NewGmailBatchDelete(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewGmailBatchDelete: %v", err)
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

	if _, err := NewClassroom(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewClassroom: %v", err)
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

	if _, err := NewAnalyticsAdmin(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewAnalyticsAdmin: %v", err)
	}

	if _, err := NewAnalyticsData(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewAnalyticsData: %v", err)
	}

	if _, err := NewSearchConsole(ctx, "a@b.com"); err != nil {
		t.Fatalf("NewSearchConsole: %v", err)
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

func TestNewChatScopesAreGrantedByChatAuth(t *testing.T) {
	granted, err := googleauth.Scopes(googleauth.ServiceChat)
	if err != nil {
		t.Fatalf("Scopes(ServiceChat): %v", err)
	}

	seen := make(map[string]struct{}, len(granted))
	for _, scope := range granted {
		seen[scope] = struct{}{}
	}

	requested := []string{
		scopeChatSpaces,
		scopeChatMessages,
		scopeChatMemberships,
		scopeChatReadStateRO,
		scopeChatReactionsCreate,
		scopeChatReactionsRO,
	}
	for _, scope := range requested {
		if _, ok := seen[scope]; !ok {
			t.Fatalf("NewChat requests scope not granted by chat auth: %s", scope)
		}
	}
}
