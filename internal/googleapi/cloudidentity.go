package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/cloudidentity/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

const (
	scopeCloudIdentityGroupsRO = "https://www.googleapis.com/auth/cloud-identity.groups.readonly"
)

// NewCloudIdentityGroups creates a Cloud Identity service for reading groups.
// This API allows non-admin users to list groups they belong to and view group members.
func NewCloudIdentityGroups(ctx context.Context, email string) (*cloudidentity.Service, error) {
	if opts, err := optionsForAccountScopes(ctx, "cloudidentity", email, []string{scopeCloudIdentityGroupsRO}); err != nil {
		return nil, fmt.Errorf("cloudidentity options: %w", err)
	} else if svc, err := cloudidentity.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create cloudidentity service: %w", err)
	} else {
		return svc, nil
	}
}

// NewCloudIdentityInboundSSO creates a Cloud Identity service for inbound SSO administration.
func NewCloudIdentityInboundSSO(ctx context.Context, email string) (*cloudidentity.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceInboundSSO, email)
	if err != nil {
		return nil, fmt.Errorf("cloudidentity inbound sso options: %w", err)
	}
	svc, err := cloudidentity.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create cloudidentity inbound sso service: %w", err)
	}
	return svc, nil
}

// NewCloudIdentity creates a Cloud Identity service for admin-level group and policy management.
func NewCloudIdentity(ctx context.Context, email string) (*cloudidentity.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceCloudIdentity, email)
	if err != nil {
		return nil, fmt.Errorf("cloud identity options: %w", err)
	}
	svc, err := cloudidentity.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create cloud identity service: %w", err)
	}
	return svc, nil
}
