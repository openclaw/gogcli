//go:build integration

package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

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

	if v, err := store.GetDefaultAccount(); err == nil && strings.TrimSpace(v) != "" {
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
	tok, err := store.GetToken(account)
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}

	scopes := tok.Scopes
	if len(scopes) == 0 {
		scopes = nil
	}
	if err := googleauth.CheckRefreshToken(ctx, tok.RefreshToken, scopes, 15*time.Second); err != nil {
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
