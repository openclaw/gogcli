package cmd

import (
	"strings"

	"github.com/steipete/gogcli/internal/errfmt"
	"github.com/steipete/gogcli/internal/googleapi"
)

var newAdminDirectoryService = googleapi.NewAdminDirectory

const (
	adminRoleMember  = "MEMBER"
	adminRoleOwner   = "OWNER"
	adminRoleManager = "MANAGER"
)

func requireAdminAccount(flags *RootFlags) (string, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return "", err
	}
	if isConsumerAccount(account) {
		return "", errfmt.NewUserFacingError(
			"Admin SDK Directory API requires a Google Workspace account with domain-wide delegation; consumer accounts (gmail.com/googlemail.com) are not supported.",
			nil,
		)
	}
	return account, nil
}

// wrapAdminDirectoryError provides helpful error messages for common Admin SDK issues.
func wrapAdminDirectoryError(err error, account string) error {
	errStr := err.Error()
	if strings.Contains(errStr, "accessNotConfigured") ||
		strings.Contains(errStr, "Admin SDK API has not been used") {
		return errfmt.NewUserFacingError("Admin SDK API is not enabled; enable it at: https://console.developers.google.com/apis/api/admin.googleapis.com/overview", err)
	}
	if strings.Contains(errStr, "insufficientPermissions") ||
		strings.Contains(errStr, "insufficient authentication scopes") ||
		strings.Contains(errStr, "Not Authorized") {
		return errfmt.NewUserFacingError("Insufficient permissions for Admin SDK API; ensure your service account has domain-wide delegation enabled with admin.directory.user, admin.directory.group, and admin.directory.group.member scopes", err)
	}
	if strings.Contains(errStr, "domain_wide_delegation") ||
		strings.Contains(errStr, "invalid_grant") {
		return errfmt.NewUserFacingError("Domain-wide delegation not configured or invalid; ensure your service account has domain-wide delegation enabled in Google Workspace Admin Console", err)
	}
	if isConsumerAccount(account) {
		return errfmt.NewUserFacingError("Admin SDK Directory API requires a Google Workspace account with domain-wide delegation; consumer accounts (gmail.com/googlemail.com) are not supported.", err)
	}
	return err
}
