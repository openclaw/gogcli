package googleapi

import (
	"context"
	"fmt"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewAdminDirectory(ctx context.Context, email string) (*admin.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceAdminDirectory, email)
	if err != nil {
		return nil, fmt.Errorf("admin directory options: %w", err)
	}
	svc, err := admin.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create admin directory service: %w", err)
	}
	return svc, nil
}
