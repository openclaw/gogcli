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
	"github.com/steipete/gogcli/internal/secrets"
)

func integrationAccount(t *testing.T) string {
	t.Helper()

	if v := strings.TrimSpace(os.Getenv("GOG_IT_ACCOUNT")); v != "" {
		return v
	}

	store, err := secrets.OpenDefault()
	if err != nil {
		t.Skipf("open secrets store (set GOG_IT_ACCOUNT to avoid keyring prompts): %v", err)
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

func TestDriveSmoke(t *testing.T) {
	account := integrationAccount(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

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
	account := integrationAccount(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

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
	account := integrationAccount(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

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
	account := integrationAccount(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := secrets.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault: %v", err)
	}
	client, err := authclient.ResolveClientWithOverride(account, "")
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
	account := integrationAccount(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

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
	account := integrationAccount(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

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
	account := integrationAccount(t)
	attendee := strings.TrimSpace(os.Getenv("GOG_IT_ATTENDEE"))
	if attendee == "" {
		t.Skip("set GOG_IT_ATTENDEE to test --send-updates with attendees")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

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
