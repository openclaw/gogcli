//go:build integration

package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/oauthclient"
	"github.com/steipete/gogcli/internal/secrets"
)

func integrationAccount(t *testing.T, store secrets.Store) string {
	t.Helper()

	if v := strings.TrimSpace(os.Getenv("GOG_IT_ACCOUNT")); v != "" {
		return v
	}

	if v, err := store.GetDefaultAccount(config.DefaultClientName); err == nil && strings.TrimSpace(v) != "" {
		return v
	}

	tokens, err := store.ListTokens()
	if err != nil {
		t.Skipf("list tokens: %v", err)
	}
	if len(tokens) == 1 && strings.TrimSpace(tokens[0].Email) != "" {
		return tokens[0].Email
	}

	t.Skip("set GOG_IT_ACCOUNT (or set a default account via `gog auth manage`, or store exactly one token)")
	return ""
}

func withIntegrationAuth(t *testing.T, ctx context.Context) (context.Context, secrets.Repository) {
	t.Helper()

	layout, err := config.ResolveSystemLayoutFor("", config.PathKindConfig, config.PathKindData)
	if err != nil {
		t.Fatalf("resolve integration layout: %v", err)
	}
	configStore := config.NewConfigStore(layout)
	secretRepository, err := secrets.OpenWithConfig(layout, configStore)
	if err != nil {
		t.Skipf("open integration secrets repository: %v", err)
	}
	credentialFiles := config.NewClientCredentialsStore(layout)
	credentialStore, err := oauthclient.NewCredentialsStore(credentialFiles, secretRepository)
	if err != nil {
		t.Fatalf("create integration credential store: %v", err)
	}

	ctx = authclient.WithClientResolver(ctx, func(email string, override string) (string, error) {
		cfg, readErr := configStore.Read()
		if readErr != nil {
			return "", readErr
		}
		return config.ResolveClientForAccountWithCredentials(cfg, email, override, func(client string) (bool, error) {
			_, exists, pathErr := credentialFiles.ExistingPath(client)
			return exists, pathErr
		})
	})
	ctx = authclient.WithCredentialsReader(ctx, credentialStore.Read)
	ctx = authclient.WithSecretsStoreOpener(ctx, func() (secrets.Store, error) {
		return secretRepository, nil
	})

	return ctx, secretRepository
}

func integrationContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc, secrets.Repository) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx, store := withIntegrationAuth(t, ctx)

	return ctx, cancel, store
}

func TestDriveSmoke(t *testing.T) {
	ctx, cancel, store := integrationContext(t, 20*time.Second)
	defer cancel()
	account := integrationAccount(t, store)

	svc, err := googleapi.NewDrive(ctx, account)
	if err != nil {
		t.Fatalf("NewDrive: %v", err)
	}
	_, err = svc.Files.List().
		Q("trashed = false").
		PageSize(1).
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true).
		Fields("files(id)").
		Do()
	if err != nil {
		t.Fatalf("Drive list: %v", err)
	}
}

func TestCalendarSmoke(t *testing.T) {
	ctx, cancel, store := integrationContext(t, 20*time.Second)
	defer cancel()
	account := integrationAccount(t, store)

	svc, err := googleapi.NewCalendar(ctx, account)
	if err != nil {
		t.Fatalf("NewCalendar: %v", err)
	}
	_, err = svc.CalendarList.List().MaxResults(1).Do()
	if err != nil {
		t.Fatalf("Calendar list: %v", err)
	}
}

func TestGmailSmoke(t *testing.T) {
	ctx, cancel, store := integrationContext(t, 20*time.Second)
	defer cancel()
	account := integrationAccount(t, store)

	svc, err := googleapi.NewGmail(ctx, account)
	if err != nil {
		t.Fatalf("NewGmail: %v", err)
	}
	_, err = svc.Users.Labels.List("me").Do()
	if err != nil {
		t.Fatalf("Gmail labels: %v", err)
	}
}

func TestAuthRefreshTokenSmoke(t *testing.T) {
	ctx, cancel, store := integrationContext(t, 20*time.Second)
	defer cancel()
	account := integrationAccount(t, store)

	client, err := authclient.ResolveClientWithOverride(ctx, account, "")
	if err != nil {
		t.Fatalf("ResolveClient: %v", err)
	}
	tok, err := store.GetToken(client, account)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}

	scopes := tok.Scopes
	if len(scopes) == 0 {
		scopes = nil
	}
	if err := googleauth.CheckRefreshToken(ctx, client, tok.RefreshToken, scopes, 15*time.Second); err != nil {
		t.Fatalf("CheckRefreshToken: %v", err)
	}
}

func TestContactsSmoke(t *testing.T) {
	ctx, cancel, store := integrationContext(t, 20*time.Second)
	defer cancel()
	account := integrationAccount(t, store)

	svc, err := googleapi.NewPeopleContacts(ctx, account)
	if err != nil {
		t.Fatalf("NewPeople: %v", err)
	}
	_, err = svc.People.Connections.List("people/me").PersonFields("names").PageSize(1).Do()
	if err != nil {
		t.Fatalf("People connections: %v", err)
	}
}

func TestClassroomSmoke(t *testing.T) {
	ctx, cancel, store := integrationContext(t, 20*time.Second)
	defer cancel()
	account := integrationAccount(t, store)

	svc, err := googleapi.NewClassroom(ctx, account)
	if err != nil {
		t.Fatalf("NewClassroom: %v", err)
	}
	_, err = svc.Courses.List().PageSize(1).Do()
	if err != nil {
		t.Fatalf("Courses list: %v", err)
	}
}

func TestCalendarSendUpdates(t *testing.T) {
	attendee := strings.TrimSpace(os.Getenv("GOG_IT_ATTENDEE"))
	if attendee == "" {
		t.Skip("set GOG_IT_ATTENDEE to test --send-updates with attendees")
	}

	ctx, cancel, store := integrationContext(t, 60*time.Second)
	defer cancel()
	account := integrationAccount(t, store)

	svc, err := googleapi.NewCalendar(ctx, account)
	if err != nil {
		t.Fatalf("NewCalendar: %v", err)
	}

	// Create event with attendee
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	event := &calendar.Event{
		Summary:   "gogcli-send-updates-test",
		Start:     &calendar.EventDateTime{DateTime: start.Format(time.RFC3339)},
		End:       &calendar.EventDateTime{DateTime: start.Add(time.Hour).Format(time.RFC3339)},
		Attendees: []*calendar.EventAttendee{{Email: attendee}},
	}

	created, err := svc.Events.Insert("primary", event).SendUpdates("all").Do()
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	defer svc.Events.Delete("primary", created.Id).SendUpdates("all").Do()

	// Update with SendUpdates
	_, err = svc.Events.Patch("primary", created.Id, &calendar.Event{
		Summary: "gogcli-send-updates-test-UPDATED",
	}).SendUpdates("all").Do()
	if err != nil {
		t.Fatalf("Patch with SendUpdates: %v", err)
	}

	// Delete happens in defer with SendUpdates
	t.Logf("Created and updated event %s with attendee %s - check attendee email for notifications", created.Id, attendee)
}
