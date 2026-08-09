package googleapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"

	"github.com/openclaw/gogcli/internal/authclient"
)

var errCapturedADC = errors.New("captured ADC")

func TestFactoryBuildsRepresentativeServices(t *testing.T) {
	t.Parallel()

	ctx := authclient.WithAccessToken(context.Background(), "test-token")
	factory := NewFactory(AuthDependencies{}, FactoryOptions{
		PhotosBaseURL:       "https://photos.example.test/v1",
		PhotosPickerBaseURL: "https://picker.example.test/v1",
	})

	if svc, err := factory.Drive(ctx, "user@example.com"); err != nil || svc == nil {
		t.Fatalf("Drive() = (%v, %v)", svc, err)
	}

	if svc, err := factory.Gmail(ctx, "user@example.com"); err != nil || svc == nil {
		t.Fatalf("Gmail() = (%v, %v)", svc, err)
	}

	if svc, err := factory.Docs(ctx, "user@example.com"); err != nil || svc == nil {
		t.Fatalf("Docs() = (%v, %v)", svc, err)
	}

	if svc, err := factory.Calendar(ctx, "user@example.com"); err != nil || svc == nil {
		t.Fatalf("Calendar() = (%v, %v)", svc, err)
	}

	if svc, err := factory.Sheets(ctx, "user@example.com"); err != nil || svc == nil {
		t.Fatalf("Sheets() = (%v, %v)", svc, err)
	}

	if svc, err := factory.YouTubeAccount(ctx, "user@example.com"); err != nil || svc == nil {
		t.Fatalf("YouTubeAccount() = (%v, %v)", svc, err)
	}

	photos, err := factory.Photos(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("Photos(): %v", err)
	}

	if photos.baseURL != "https://photos.example.test/v1" {
		t.Fatalf("Photos base URL = %q", photos.baseURL)
	}

	picker, err := factory.PhotosPicker(ctx, "user@example.com")
	if err != nil {
		t.Fatalf("PhotosPicker(): %v", err)
	}

	if picker.baseURL != "https://picker.example.test/v1" {
		t.Fatalf("Photos Picker base URL = %q", picker.baseURL)
	}
}

func TestFactoryUsesCapturedAuthDependencies(t *testing.T) {
	t.Parallel()

	factory := NewFactory(AuthDependencies{
		Mode: AuthModeADC,
		ADCTokenSource: func(context.Context, ...string) (oauth2.TokenSource, error) {
			return nil, errCapturedADC
		},
	}, FactoryOptions{})
	ctx := WithAuthDependencies(context.Background(), AuthDependencies{
		Mode: AuthModeADC,
		ADCTokenSource: func(context.Context, ...string) (oauth2.TokenSource, error) {
			return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "wrong"}), nil
		},
	})

	_, err := factory.Drive(ctx, "user@example.com")
	if !errors.Is(err, errCapturedADC) {
		t.Fatalf("Drive() error = %v, want %v", err, errCapturedADC)
	}
}

func TestFactoryMissingAuthDependenciesFailsClosed(t *testing.T) {
	t.Parallel()

	factory := NewFactory(AuthDependencies{}, FactoryOptions{})

	_, err := factory.Drive(context.Background(), "user@example.com")
	if !errors.Is(err, errServiceAccountStoreRequired) {
		t.Fatalf("Drive() error = %v, want %v", err, errServiceAccountStoreRequired)
	}
}

func TestFactoryKeepUsesCapturedTokenSource(t *testing.T) {
	t.Parallel()

	keyPath := filepath.Join(t.TempDir(), "service-account.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write service account: %v", err)
	}

	var gotSubject string
	factory := NewFactory(AuthDependencies{
		ServiceAccountTokenSource: func(_ context.Context, _ []byte, subject string, scopes []string) (oauth2.TokenSource, error) {
			gotSubject = subject

			if len(scopes) == 0 {
				t.Fatal("expected Keep scopes")
			}

			return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"}), nil
		},
	}, FactoryOptions{})

	svc, err := factory.Keep(context.Background(), keyPath, "user@example.com")
	if err != nil {
		t.Fatalf("Keep(): %v", err)
	}

	if svc == nil {
		t.Fatal("Keep() returned nil service")
	}

	if gotSubject != "user@example.com" {
		t.Fatalf("subject = %q", gotSubject)
	}
}
