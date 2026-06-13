package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/ui"
)

func TestGroupsMembers_ValidationErrors(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := (&GroupsMembersCmd{}).Run(ctx, &RootFlags{}); err == nil {
		t.Fatalf("expected missing account error")
	}
	if err := (&GroupsMembersCmd{}).Run(ctx, &RootFlags{Account: "a@b.com"}); err == nil {
		t.Fatalf("expected missing group email error")
	}
}

func TestGroupsInvalidMaxFailsBeforeService(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := withCloudIdentityTestServiceFactory(
		ui.WithUI(context.Background(), u),
		unexpectedCloudIdentityTestService(t, "expected max validation to fail before creating Cloud Identity service"),
	)
	flags := &RootFlags{Account: "a@b.com"}

	testCases := []struct {
		name string
		run  func() error
	}{
		{name: "list zero", run: func() error { return (&GroupsListCmd{Max: 0}).Run(ctx, flags) }},
		{name: "list negative", run: func() error { return (&GroupsListCmd{Max: -1}).Run(ctx, flags) }},
		{name: "members zero", run: func() error { return (&GroupsMembersCmd{GroupEmail: "eng@example.com", Max: 0}).Run(ctx, flags) }},
		{name: "members negative", run: func() error { return (&GroupsMembersCmd{GroupEmail: "eng@example.com", Max: -1}).Run(ctx, flags) }},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "max must be > 0") {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestRequireGroupsAccount_ConsumerBlocked(t *testing.T) {
	account, err := requireGroupsAccount(&RootFlags{Account: "person@gmail.com"})
	if err == nil {
		t.Fatal("expected error")
	}
	if account != "" {
		t.Fatalf("account = %q, want empty", account)
	}
	if ExitCode(err) != exitCodePermissionDenied {
		t.Fatalf("exit code = %d, want %d: %v", ExitCode(err), exitCodePermissionDenied, err)
	}
	if !strings.Contains(err.Error(), groupsWorkspaceRequiredMessage) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireGroupsAccount_ExplicitIdentityRequiredForDirectToken(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	t.Setenv("GOG_AUTH_MODE", "")

	account, err := requireGroupsAccount(&RootFlags{
		AccessToken: "direct-token",
		diagnostics: io.Discard,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if account != "" {
		t.Fatalf("account = %q, want empty", account)
	}
	if ExitCode(err) != 2 || !strings.Contains(err.Error(), groupsExplicitAccountMessage) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireGroupsAccount_ExplicitIdentityRequiredForADC(t *testing.T) {
	t.Setenv("GOG_AUTH_MODE", "adc")
	t.Setenv("GOG_ACCOUNT", "")

	account, err := requireGroupsAccount(&RootFlags{})
	if err == nil {
		t.Fatal("expected error")
	}
	if account != "" {
		t.Fatalf("account = %q, want empty", account)
	}
	if ExitCode(err) != 2 || !strings.Contains(err.Error(), groupsExplicitAccountMessage) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireGroupsAccount_ExplicitIdentityRequiredForADCAutoEnv(t *testing.T) {
	t.Setenv("GOG_AUTH_MODE", "adc")

	for _, accountEnv := range []string{"auto", "default"} {
		t.Run(accountEnv, func(t *testing.T) {
			t.Setenv("GOG_ACCOUNT", accountEnv)

			account, err := requireGroupsAccount(&RootFlags{})
			if err == nil {
				t.Fatal("expected error")
			}
			if account != "" {
				t.Fatalf("account = %q, want empty", account)
			}
			if ExitCode(err) != 2 || !strings.Contains(err.Error(), groupsExplicitAccountMessage) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRequireGroupsAuthAccount_AllowsDirectTokenWithoutIdentity(t *testing.T) {
	t.Setenv("GOG_ACCOUNT", "")
	t.Setenv("GOG_AUTH_MODE", "")

	account, err := requireGroupsAuthAccount(&RootFlags{
		AccessToken: "direct-token",
		diagnostics: io.Discard,
	})
	if err != nil {
		t.Fatalf("requireGroupsAuthAccount: %v", err)
	}
	if account != accessTokenPlaceholderAccount {
		t.Fatalf("account = %q, want %q", account, accessTokenPlaceholderAccount)
	}
}

func TestRequireGroupsAuthAccount_AllowsADCWithoutIdentity(t *testing.T) {
	t.Setenv("GOG_AUTH_MODE", "adc")
	t.Setenv("GOG_ACCOUNT", "")

	account, err := requireGroupsAuthAccount(&RootFlags{})
	if err != nil {
		t.Fatalf("requireGroupsAuthAccount: %v", err)
	}
	if account != adcPlaceholderAccount {
		t.Fatalf("account = %q, want %q", account, adcPlaceholderAccount)
	}
}

func TestRequireGroupsAuthAccount_IgnoresIdentityHintForDirectAuth(t *testing.T) {
	tests := []struct {
		name        string
		authMode    string
		accessToken string
		want        string
	}{
		{name: "direct token", accessToken: "direct-token", want: accessTokenPlaceholderAccount},
		{name: "ADC", authMode: "adc", want: adcPlaceholderAccount},
		{name: "ADC over direct token", authMode: "adc", accessToken: "direct-token", want: adcPlaceholderAccount},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOG_AUTH_MODE", tc.authMode)
			t.Setenv("GOG_ACCOUNT", "person@gmail.com")

			account, err := requireGroupsAuthAccount(&RootFlags{
				AccessToken: tc.accessToken,
				diagnostics: io.Discard,
			})
			if err != nil {
				t.Fatalf("requireGroupsAuthAccount: %v", err)
			}
			if account != tc.want {
				t.Fatalf("account = %q, want %q", account, tc.want)
			}
		})
	}
}

func TestGroupsConsumerPreflightSkipsServices(t *testing.T) {
	ctx := withCloudIdentityTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		unexpectedCloudIdentityTestService(t, "consumer preflight must not create a Cloud Identity service"),
	)
	flags := &RootFlags{Account: "person@gmail.com"}

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "list", run: func() error { return (&GroupsListCmd{Max: 100}).Run(ctx, flags) }},
		{name: "members", run: func() error {
			return (&GroupsMembersCmd{GroupEmail: "engineering@example.com", Max: 100}).Run(ctx, flags)
		}},
		{name: "calendar team", run: func() error {
			return (&CalendarTeamCmd{GroupEmail: "engineering@example.com", Max: 100}).Run(ctx, flags)
		}},
		{name: "backup", run: func() error {
			_, err := buildGroupsBackupSnapshot(ctx, flags, 100)
			return err
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || ExitCode(err) != exitCodePermissionDenied {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(err.Error(), "Workspace/Cloud Identity account") {
				t.Fatalf("unexpected guidance: %v", err)
			}
		})
	}
}

func TestGroupsList_NoGroups_Text(t *testing.T) {
	svc := newCloudIdentityTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "groups/-/memberships:searchTransitiveGroups") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memberships": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
	}))

	var errBuf strings.Builder
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: &errBuf, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := withCloudIdentityTestService(ui.WithUI(context.Background(), u), svc)

	if err := (&GroupsListCmd{Max: 100}).Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(errBuf.String(), "No groups found") {
		t.Fatalf("unexpected stderr: %q", errBuf.String())
	}
}

func TestWrapCloudIdentityError_Messages(t *testing.T) {
	accessErr := errors.New("accessNotConfigured")
	if err := wrapCloudIdentityError(accessErr, "user@company.com"); err == nil || !strings.Contains(err.Error(), "Cloud Identity API is not enabled") {
		t.Fatalf("unexpected error: %v", err)
	}

	permErr := errors.New("insufficientPermissions")
	if err := wrapCloudIdentityError(permErr, "admin@company.com"); err == nil ||
		!strings.Contains(err.Error(), "Insufficient permissions") ||
		!strings.Contains(err.Error(), groupReadonlyScope) ||
		!strings.Contains(err.Error(), "gog auth service-account set admin@company.com") ||
		strings.Contains(err.Error(), "gog auth add") {
		t.Fatalf("unexpected error: %v", err)
	}

	other := errors.New("other")
	if err := wrapCloudIdentityError(other, "user@company.com"); err == nil || err.Error() != "other" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWrapCloudIdentityError_AuthModeGuidance(t *testing.T) {
	permissionErr := errors.New("insufficientPermissions")

	directErr := wrapCloudIdentityError(permissionErr, accessTokenPlaceholderAccount)
	if !strings.Contains(directErr.Error(), "direct access token") ||
		!strings.Contains(directErr.Error(), groupReadonlyScope) ||
		strings.Contains(directErr.Error(), "service-account set access-token-user") {
		t.Fatalf("unexpected direct-token guidance: %v", directErr)
	}

	adcErr := wrapCloudIdentityError(permissionErr, adcPlaceholderAccount)
	if !strings.Contains(adcErr.Error(), "Application Default Credentials principal") ||
		!strings.Contains(adcErr.Error(), "unset GOG_AUTH_MODE") ||
		strings.Contains(adcErr.Error(), "service-account set adc") {
		t.Fatalf("unexpected ADC guidance: %v", adcErr)
	}
}

func TestGetRelationType_More(t *testing.T) {
	if got := getRelationType("DIRECT"); got != "direct" {
		t.Fatalf("unexpected DIRECT: %q", got)
	}
	if got := getRelationType("INDIRECT"); got != "indirect" {
		t.Fatalf("unexpected INDIRECT: %q", got)
	}
	if got := getRelationType("OTHER"); got != "OTHER" {
		t.Fatalf("unexpected OTHER: %q", got)
	}
}
