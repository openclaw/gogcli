package googleapi

import (
	"context"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

// NewAdminDirectory creates an Admin SDK Directory service for user and group management.
// This API requires domain-wide delegation with a service account to manage Workspace users.
func NewAdminDirectory(ctx context.Context, email string) (*admin.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceAdmin, "admin directory", admin.NewService)
}
