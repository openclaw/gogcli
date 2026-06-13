package googleapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/authclient"
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

func TestNewCloudIdentityGroupsAuthErrorUsesGroupsLabel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	origRead := readClientCredentials
	origOpen := openSecretsStore

	t.Cleanup(func() {
		readClientCredentials = origRead
		openSecretsStore = origOpen
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		t.Fatal("OAuth client credentials must not be read for Groups")
		return config.ClientCredentials{}, errBoom
	}
	openSecretsStore = func() (secrets.Store, error) {
		t.Fatal("OAuth token store must not be opened for Groups")
		return nil, errBoom
	}

	_, err := NewCloudIdentityGroups(testClientResolverContext(), "admin@example.com")
	if err == nil {
		t.Fatal("expected error")
	}

	var authErr *AuthRequiredError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthRequiredError, got %T: %v", err, err)
	}

	if authErr.Service != "groups" {
		t.Fatalf("service = %q, want groups", authErr.Service)
	}
}

func TestNewCloudIdentityGroupsUsesDirectToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ctx := authclient.WithAccessToken(testClientResolverContext(), "direct-token")

	if _, err := NewCloudIdentityGroups(ctx, "admin@example.com"); err != nil {
		t.Fatalf("NewCloudIdentityGroups: %v", err)
	}
}

func TestNewCloudIdentityGroupsUsesADC(t *testing.T) {
	t.Setenv("GOG_AUTH_MODE", "adc")

	origADC := newADCTokenSource

	t.Cleanup(func() { newADCTokenSource = origADC })

	newADCTokenSource = func(_ context.Context, scopes ...string) (oauth2.TokenSource, error) {
		if len(scopes) != 1 || scopes[0] != scopeCloudIdentityGroupsRO {
			t.Fatalf("scopes = %#v", scopes)
		}

		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "adc-token"}), nil
	}

	if _, err := NewCloudIdentityGroups(testClientResolverContext(), "admin@example.com"); err != nil {
		t.Fatalf("NewCloudIdentityGroups: %v", err)
	}
}

func TestNewCloudIdentityGroupsADCPrecedesDirectToken(t *testing.T) {
	t.Setenv("GOG_AUTH_MODE", "adc")

	origADC := newADCTokenSource

	t.Cleanup(func() { newADCTokenSource = origADC })

	adcCalled := false
	newADCTokenSource = func(_ context.Context, scopes ...string) (oauth2.TokenSource, error) {
		adcCalled = true

		if len(scopes) != 1 || scopes[0] != scopeCloudIdentityGroupsRO {
			t.Fatalf("scopes = %#v", scopes)
		}

		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "adc-token"}), nil
	}

	ctx := authclient.WithAccessToken(testClientResolverContext(), "direct-token")
	if _, err := NewCloudIdentityGroups(ctx, "admin@example.com"); err != nil {
		t.Fatalf("NewCloudIdentityGroups: %v", err)
	}

	if !adcCalled {
		t.Fatal("expected ADC token source to take precedence")
	}
}

func TestNewCloudIdentityGroupsUsesRequiredServiceAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	saPath, err := config.ServiceAccountPath("admin@example.com")
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}

	if _, err := config.EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}

	if err := os.WriteFile(saPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write service account: %v", err)
	}

	origSA := newServiceAccountTokenSource

	t.Cleanup(func() { newServiceAccountTokenSource = origSA })

	newServiceAccountTokenSource = func(_ context.Context, _ []byte, subject string, scopes []string) (oauth2.TokenSource, error) {
		if subject != "admin@example.com" {
			t.Fatalf("subject = %q", subject)
		}

		if len(scopes) != 1 || scopes[0] != scopeCloudIdentityGroupsRO {
			t.Fatalf("scopes = %#v", scopes)
		}

		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "service-account-token"}), nil
	}

	if _, err := NewCloudIdentityGroups(testClientResolverContext(), "admin@example.com"); err != nil {
		t.Fatalf("NewCloudIdentityGroups: %v", err)
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
