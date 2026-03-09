package googleapi

import (
	"context"
	"fmt"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

// NewAdminDirectory creates an Admin SDK Directory service for user and group management.
// This API requires domain-wide delegation with a service account to manage Workspace users.
func NewAdminDirectory(ctx context.Context, email string) (*admin.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceAdmin, email); err != nil {
		return nil, fmt.Errorf("admin directory options: %w", err)
	} else if svc, err := admin.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create admin directory service: %w", err)
	} else {
		return svc, nil
	}
}
