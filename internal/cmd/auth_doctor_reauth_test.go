package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
)

func TestAuthDoctor_ReauthHintPreservesRecordedGrant(t *testing.T) {
	for _, refreshError := range []string{"invalid_grant", "invalid_rapt"} {
		t.Run(refreshError, func(t *testing.T) {
			setTestConfigHome(t)
			t.Setenv("GOG_KEYRING_BACKEND", "keychain")
			store := newMemSecretsStore()
			if err := store.SetToken(config.DefaultClientName, "reader@example.com", secrets.Token{
				RefreshToken: "test-refresh-token",
				Services:     []string{"gmail"},
				Scopes:       []string{"https://www.googleapis.com/auth/gmail.readonly"},
			}); err != nil {
				t.Fatal(err)
			}
			result := executeWithTestRuntime(t, []string{"--json", "auth", "doctor", "--check"}, &app.Runtime{Auth: app.AuthOperations{
				OpenSecretsStore:  func() (secrets.Store, error) { return store, nil },
				CheckRefreshToken: func(context.Context, string, string, []string, time.Duration) error { return errors.New(refreshError) },
			}})
			if result.err != nil {
				t.Fatal(result.err)
			}
			var report struct {
				Checks []authDoctorCheck `json:"checks"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &report); err != nil {
				t.Fatal(err)
			}
			for _, check := range report.Checks {
				if check.Name != "refresh.default.reader@example.com" {
					continue
				}
				for _, required := range []string{"gog auth list --json", "same account", "client", "--services", "--gmail-scope", "--drive-scope", "--photos-scope", "--extra-scopes", "--force-consent"} {
					if !strings.Contains(check.Hint, required) {
						t.Errorf("hint omits %q: %s", required, check.Hint)
					}
				}
				if strings.Contains(check.Hint, "gog auth add <email> --force-consent") {
					t.Error("hint still recommends default scopes")
				}
				return
			}
			t.Fatal("missing refresh check")
		})
	}
}
