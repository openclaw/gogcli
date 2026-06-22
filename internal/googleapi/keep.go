package googleapi

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/api/keep/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewKeep(ctx context.Context, email string) (*keep.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceKeep, "keep", keep.NewService)
}

func NewKeepWithServiceAccount(ctx context.Context, serviceAccountPath, impersonateEmail string) (*keep.Service, error) {
	return newKeepWithServiceAccount(ctx, serviceAccountPath, impersonateEmail, DefaultServiceAccountTokenSource)
}

func newKeepWithServiceAccount(
	ctx context.Context,
	serviceAccountPath string,
	impersonateEmail string,
	tokenSourceFactory ServiceAccountTokenSourceFunc,
) (*keep.Service, error) {
	data, err := os.ReadFile(serviceAccountPath) //nolint:gosec // user-provided path (or stored config file)
	if err != nil {
		return nil, fmt.Errorf("read service account file: %w", err)
	}

	scopes, err := googleauth.Scopes(googleauth.ServiceKeep)
	if err != nil {
		return nil, fmt.Errorf("keep scopes: %w", err)
	}

	if tokenSourceFactory == nil {
		return nil, errServiceAccountTokenSourceRequired
	}

	tokenSource, err := tokenSourceFactory(ctx, data, impersonateEmail, scopes)
	if err != nil {
		return nil, err
	}

	svc, err := keep.NewService(ctx, tokenSourceClientOptions(ctx, tokenSource)...)
	if err != nil {
		return nil, fmt.Errorf("create keep service: %w", err)
	}

	return svc, nil
}
