package googleapi

import (
	"context"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

const scopeGmailFullAccess = "https://mail.google.com/"

func NewGmail(ctx context.Context, email string) (*gmail.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceGmail, "gmail", gmail.NewService)
}

func NewGmailBatchDelete(ctx context.Context, email string) (*gmail.Service, error) {
	return newGoogleServiceForRequiredScopes(
		ctx,
		email,
		string(googleauth.ServiceGmail),
		"gmail batch delete",
		[]string{scopeGmailFullAccess},
		gmail.NewService,
	)
}
