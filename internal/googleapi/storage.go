package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/storage/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewStorage(ctx context.Context, email string) (*storage.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceVault, email)
	if err != nil {
		return nil, fmt.Errorf("storage options: %w", err)
	}
	svc, err := storage.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create storage service: %w", err)
	}
	return svc, nil
}
