package googleapi

import (
	"context"

	"google.golang.org/api/cloudidentity/v1"
)

const (
	scopeCloudIdentityGroupsRO = "https://www.googleapis.com/auth/cloud-identity.groups.readonly"
)

// NewCloudIdentityGroups creates a Cloud Identity service for reading groups.
// This API allows non-admin users to list groups they belong to and view group members.
func NewCloudIdentityGroups(ctx context.Context, email string) (*cloudidentity.Service, error) {
	return newGoogleServiceForScopes(ctx, email, "cloudidentity", "cloudidentity", []string{scopeCloudIdentityGroupsRO}, cloudidentity.NewService)
}
