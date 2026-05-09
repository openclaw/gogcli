package googleapi

import (
	"context"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewGmail(ctx context.Context, email string) (*gmail.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceGmail, "gmail", gmail.NewService)
}
