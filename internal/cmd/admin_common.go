package cmd

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/steipete/gogcli/internal/errfmt"
	"github.com/steipete/gogcli/internal/googleapi"
)

var (
	newAdminDirectoryService        = googleapi.NewAdminDirectory
	newAdminOrgUnitDirectoryService = googleapi.NewAdminDirectoryOrgUnit
)

const (
	adminCustomerID  = "my_customer"
	adminRoleMember  = "MEMBER"
	adminRoleOwner   = "OWNER"
	adminRoleManager = "MANAGER"
)

func generateAdminUserPassword(length int) (string, error) {
	if length < 8 {
		length = 8
	}
	const lower = "abcdefghijklmnopqrstuvwxyz"
	const upper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const digits = "0123456789"
	const special = "!@#$%^&*()_+-=[]{}|;:,.<>?"
	const all = lower + upper + digits + special

	sets := []string{lower, upper, digits, special}
	b := make([]byte, length)
	for i, set := range sets {
		ch, err := adminUserRandChar(set)
		if err != nil {
			return "", err
		}
		b[i] = ch
	}
	for i := len(sets); i < length; i++ {
		ch, err := adminUserRandChar(all)
		if err != nil {
			return "", err
		}
		b[i] = ch
	}
	for i := len(b) - 1; i > 0; i-- {
		j, err := adminUserRandInt(i + 1)
		if err != nil {
			return "", err
		}
		b[i], b[j] = b[j], b[i]
	}
	return string(b), nil
}

func adminUserRandChar(set string) (byte, error) {
	if len(set) == 0 {
		return 0, fmt.Errorf("empty character set")
	}
	idx, err := adminUserRandInt(len(set))
	if err != nil {
		return 0, err
	}
	return set[idx], nil
}

func adminUserRandInt(maxVal int) (int, error) {
	if maxVal <= 0 {
		return 0, fmt.Errorf("invalid max %d", maxVal)
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(maxVal)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

func normalizeAdminUserHashFunction(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "md5":
		return "MD5", nil
	case "sha-1", "sha1":
		return "SHA-1", nil
	case "crypt":
		return "crypt", nil
	default:
		return "", usage("invalid --hash-function (expected MD5, SHA-1, crypt)")
	}
}

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
	return wrapAdminDirectoryErrorWithScopes(err, account, "admin.directory.user, admin.directory.group, and admin.directory.group.member scopes")
}

func wrapAdminOrgUnitDirectoryError(err error, account string) error {
	return wrapAdminDirectoryErrorWithScopes(err, account, "admin.directory.orgunit scope")
}

func wrapAdminDirectoryErrorWithScopes(err error, account, scopes string) error {
	errStr := err.Error()
	if strings.Contains(errStr, "accessNotConfigured") ||
		strings.Contains(errStr, "Admin SDK API has not been used") {
		return errfmt.NewUserFacingError("Admin SDK API is not enabled; enable it at: https://console.developers.google.com/apis/api/admin.googleapis.com/overview", err)
	}
	if strings.Contains(errStr, "insufficientPermissions") ||
		strings.Contains(errStr, "insufficient authentication scopes") ||
		strings.Contains(errStr, "Not Authorized") {
		return errfmt.NewUserFacingError("Insufficient permissions for Admin SDK API; ensure your service account has domain-wide delegation enabled with "+scopes, err)
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
