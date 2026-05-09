package googleapi

import (
	"context"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewDocs(ctx context.Context, email string) (*docs.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceDocs, "docs", docs.NewService)
}
