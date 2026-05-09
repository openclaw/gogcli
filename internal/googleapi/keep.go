package googleapi

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/keep/v1"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewKeep(ctx context.Context, email string) (*keep.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceKeep, "keep", keep.NewService)
}

func NewKeepWithServiceAccount(ctx context.Context, serviceAccountPath, impersonateEmail string) (*keep.Service, error) {
	data, err := os.ReadFile(serviceAccountPath) //nolint:gosec // user-provided path (or stored config file)
	if err != nil {
		return nil, fmt.Errorf("read service account file: %w", err)
	}

	scopes, err := googleauth.Scopes(googleauth.ServiceKeep)
	if err != nil {
		return nil, fmt.Errorf("keep scopes: %w", err)
	}

	config, err := google.JWTConfigFromJSON(data, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}

	config.Subject = impersonateEmail

	svc, err := keep.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx)))
	if err != nil {
		return nil, fmt.Errorf("create keep service: %w", err)
	}

	return svc, nil
}
