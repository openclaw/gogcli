package googleapi

import (
	"context"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleauth"
)

const scopeGmailFullAccess = "https://mail.google.com/"

func NewGmail(ctx context.Context, email string, options ...option.ClientOption) (*gmail.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceGmail, "gmail", gmail.NewService, options...)
}

func NewGmailBatchDelete(ctx context.Context, email string, options ...option.ClientOption) (*gmail.Service, error) {
	return newGoogleServiceForRequiredScopes(
		ctx,
		email,
		string(googleauth.ServiceGmail),
		"gmail batch delete",
		[]string{scopeGmailFullAccess},
		gmail.NewService,
		options...,
	)
}
