package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/vault/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewVault(ctx context.Context, email string) (*vault.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceVault, email)
	if err != nil {
		return nil, fmt.Errorf("vault options: %w", err)
	}

	svc, err := vault.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create vault service: %w", err)
	}

	return svc, nil
}
