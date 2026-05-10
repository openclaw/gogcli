package googleapi

import (
	"context"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

var adminOrgUnitScopes = []string{"https://www.googleapis.com/auth/admin.directory.orgunit"}

// NewAdminDirectory creates an Admin SDK Directory service for user and group management.
// This API requires domain-wide delegation with a service account to manage Workspace users.
func NewAdminDirectory(ctx context.Context, email string) (*admin.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceAdmin, "admin directory", admin.NewService)
}

// NewAdminDirectoryOrgUnit creates an Admin SDK Directory service with only the
// organizational-unit scope so existing user/group DWD setups are not widened.
func NewAdminDirectoryOrgUnit(ctx context.Context, email string) (*admin.Service, error) {
	return newGoogleServiceForScopes(ctx, email, "admin orgunits", "admin orgunits", adminOrgUnitScopes, admin.NewService)
}
